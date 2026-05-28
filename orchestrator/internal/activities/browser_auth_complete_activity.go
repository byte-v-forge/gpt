package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) BrowserAuthCompleteActivity(ctx context.Context, input BrowserAuthCompleteInput) (RegisterActivityOutput, error) {
	stepName, err := browserAuthCompleteStepName(input.Mode)
	if err != nil {
		return RegisterActivityOutput{}, err
	}
	data := map[string]any{
		"account_id":         input.GetAccountId(),
		"browser_session_id": input.GetBrowserSessionId(),
		"mode":               input.GetMode(),
		"otp_source":         input.GetOtpSource(),
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, err
	}
	step.progress("completing browser auth", map[string]any{
		"mode":       input.GetMode(),
		"otp_source": input.GetOtpSource(),
	})
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "completing browser auth", data)
	defer stopHeartbeat()

	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	otpKind, _, err := s.getJobParam(ctx, input.GetJobId(), browserAuthOTPKindParam)
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if otpKind != "" {
		data["otp_kind"] = otpKind
	}
	otp, err := s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetOtpIssuedAfterUnix())
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	result, err := s.browserAuthComplete(ctx, input.GetMode(), input.GetJobId(), account, input.GetBrowserSessionId(), otp, otpKind)
	data["browser_complete"] = registerResultData(result)
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if result == nil {
		err := fmt.Errorf("browser %s complete returned empty response", input.GetMode())
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if !result.GetSuccess() {
		err := fmt.Errorf("browser %s complete failed: %s", input.GetMode(), result.GetErrorMessage())
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}

	output := registerActivityOutputFromResponse(result, data)
	return output, step.complete(data, nil)
}

func (s *Server) BrowserAuthCancelActivity(ctx context.Context, input BrowserAuthCancelInput) error {
	if strings.TrimSpace(input.GetBrowserSessionId()) == "" {
		return nil
	}
	resp, err := s.browserAuthCancel(ctx, input.GetMode(), input.GetBrowserSessionId())
	if err != nil {
		return err
	}
	if resp != nil && !resp.GetSuccess() {
		return fmt.Errorf("browser %s cancel failed: %s", input.Mode, resp.GetErrorMessage())
	}
	return nil
}

func (s *Server) completeBrowserAuthStep(ctx context.Context, jobID, stepName, accountID string, data map[string]any, err error) error {
	if isAccountAlreadyExistsError(err) {
		if data != nil {
			data["terminal_reason"] = "openai_user_already_exists"
		}
		if updateErr := s.updateAccount(ctx, &pb.Account{
			AccountId:    accountID,
			Status:       accountStatusUserAlreadyExists,
			ErrorMessage: err.Error(),
		}); updateErr != nil {
			err = fmt.Errorf("%w; additionally failed to mark account user already exists: %v", err, updateErr)
		}
		if updateErr := s.markAccountEmailUserAlreadyExists(ctx, accountID, err.Error()); updateErr != nil {
			err = fmt.Errorf("%w; additionally failed to mark mailbox user already exists: %v", err, updateErr)
		}
	}
	return s.completeActivityStep(ctx, jobID, stepName, false, true, data, err)
}

func (s *Server) markAccountEmailUserAlreadyExists(ctx context.Context, accountID string, lastError string) error {
	if s.accountClient == nil {
		return nil
	}
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return err
	}
	email := strings.TrimSpace(account.GetEmail())
	if email == "" {
		return nil
	}
	_, err = s.accountClient.MarkGPTEmailAllocationStatus(ctx, &pb.MarkGPTEmailAllocationStatusRequest{
		Email:     email,
		Status:    emailStatusUserAlreadyExists,
		LastError: lastError,
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "gpt email allocation not found") {
		return nil
	}
	return err
}

func registerActivityOutputFromResponse(resp *pb.RegisterResponse, data map[string]any) RegisterActivityOutput {
	if resp == nil {
		return RegisterActivityOutput{Data: protoData(data)}
	}
	return RegisterActivityOutput{
		SessionToken:      resp.GetSessionToken(),
		AccessToken:       resp.GetAccessToken(),
		DeviceId:          resp.GetDeviceId(),
		PlusTrialEligible: resp.GetPlusTrialEligible(),
		PlusTrialChecked:  resp.GetPlusTrialChecked(),
		CheckoutUrl:       resp.GetCheckoutUrl(),
		Data:              protoData(data),
	}
}
