package activities

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"orchestrator/internal/gptaccount"
	"strings"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/protowrap"
	"orchestrator/pb"
)

func (s *Server) BrowserAuthCompleteActivity(ctx context.Context, input *BrowserAuthCompleteInput) (*RegisterActivityOutput, error) {
	stepName, err := browserAuthCompleteStepName(input.Mode)
	if err != nil {
		return nil, err
	}
	data := &pb.ActivityBrowserAuthCompleteStepData{
		AccountId:        input.GetAccountId(),
		BrowserSessionId: input.GetBrowserSessionId(),
		Mode:             input.GetMode(),
		OtpSource:        input.GetOtpSource(),
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return &RegisterActivityOutput{Data: registerOutputData(data)}, err
	}
	step.progress("completing browser auth", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "completing browser auth", data)
	defer stopHeartbeat()

	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return &RegisterActivityOutput{Data: registerOutputData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	otpKind, _, err := s.getJobParam(ctx, input.GetJobId(), browserAuthOTPKindParam)
	if err != nil {
		return &RegisterActivityOutput{Data: registerOutputData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if otpKind != "" {
		data.OtpKind = otpKind
	}
	otp, err := s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetOtpIssuedAfterUnix())
	if err != nil {
		return &RegisterActivityOutput{Data: registerOutputData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	result, err := s.browserAuthComplete(ctx, input.GetMode(), input.GetJobId(), account, input.GetBrowserSessionId(), otp, otpKind)
	data.BrowserComplete = registerResultData(result)
	if err != nil {
		return &RegisterActivityOutput{Data: registerOutputData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if result == nil {
		err := fmt.Errorf("browser %s complete returned empty response", input.GetMode())
		return &RegisterActivityOutput{Data: registerOutputData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if !result.GetSuccess() {
		err := fmt.Errorf("browser %s complete failed: %s", input.GetMode(), result.GetErrorMessage())
		return &RegisterActivityOutput{Data: registerOutputData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}

	output := registerActivityOutputFromResponse(result, data)
	return output, step.complete(data, nil)
}

func (s *Server) BrowserAuthCancelActivity(ctx context.Context, input *BrowserAuthCancelInput) error {
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

func (s *Server) completeBrowserAuthStep(ctx context.Context, jobID, stepName, accountID string, data proto.Message, err error) error {
	if isAccountAlreadyExistsError(err) {
		setBrowserAuthTerminalReason(data, "openai_user_already_exists")
		account := gptaccount.Patch(accountID)
		gptaccount.SetStatus(account, gptplugin.AccountStatusUserAlreadyExists, err.Error())
		if updateErr := s.updateAccount(ctx, account); updateErr != nil {
			err = fmt.Errorf("%w; additionally failed to mark account user already exists: %v", err, updateErr)
		}
		if updateErr := s.markAccountEmailUserAlreadyExists(ctx, accountID, err.Error()); updateErr != nil {
			err = fmt.Errorf("%w; additionally failed to mark mailbox user already exists: %v", err, updateErr)
		}
	}
	return s.completeActivityStep(ctx, jobID, stepName, false, true, data, err)
}

func setBrowserAuthTerminalReason(data proto.Message, reason string) {
	protowrap.SetStringField(data, "terminal_reason", strings.TrimSpace(reason))
}

func (s *Server) markAccountEmailUserAlreadyExists(ctx context.Context, accountID string, lastError string) error {
	if s.accountClient == nil {
		return nil
	}
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return err
	}
	email := strings.TrimSpace(gptaccount.Email(account))
	if email == "" {
		return nil
	}
	_, err = s.accountClient.MarkGPTEmailAllocationStatus(ctx, &pb.MarkGPTEmailAllocationStatusRequest{
		Email:     email,
		Status:    gptplugin.EmailStatusUserAlreadyExists,
		LastError: lastError,
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "gpt email allocation not found") {
		return nil
	}
	return err
}

func registerActivityOutputFromResponse(resp *pb.RegisterResponse, data proto.Message) *RegisterActivityOutput {
	if resp == nil {
		return &RegisterActivityOutput{Data: registerOutputData(data)}
	}
	return &RegisterActivityOutput{
		SessionToken:      resp.GetSessionToken(),
		AccessToken:       resp.GetAccessToken(),
		DeviceId:          resp.GetDeviceId(),
		PlusTrialEligible: resp.GetPlusTrialEligible(),
		PlusTrialChecked:  resp.GetPlusTrialChecked(),
		CheckoutUrl:       resp.GetCheckoutUrl(),
		Data:              registerOutputData(data),
	}
}

func registerOutputData(data proto.Message) *pb.ActivityRegisterOutputData {
	out := &pb.ActivityRegisterOutputData{}
	if !protowrap.SetMessage(out, data) {
		return nil
	}
	return out
}
