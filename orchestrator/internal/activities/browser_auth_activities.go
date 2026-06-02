package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/gptaccount"
	"strings"

	"orchestrator/pb"
)

const (
	browserAuthModeRegister = "register"
	browserAuthModeLogin    = "login"
)

func (s *Server) BrowserAuthStartActivity(ctx context.Context, input *BrowserAuthStartInput) (*BrowserAuthStartOutput, error) {
	output := &BrowserAuthStartOutput{
		AccountId: input.GetAccountId(),
	}
	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return output, err
	}
	if err := rejectUserAlreadyExistsAccount(account); err != nil {
		return output, err
	}
	if strings.TrimSpace(gptaccount.Email(account)) == "" {
		return output, fmt.Errorf("email is required")
	}
	password, err := s.getAccountPassword(ctx, gptaccount.ID(account))
	if err != nil {
		return output, err
	}

	stepName, err := browserAuthStartStepName(input.GetMode())
	if err != nil {
		return output, err
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}

	data := &pb.ActivityBrowserAuthStartStepData{
		AccountId: gptaccount.ID(account),
		Email:     gptaccount.Email(account),
		Mode:      input.GetMode(),
	}
	step.progress("starting browser auth", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "starting browser auth", data)
	defer stopHeartbeat()

	startResp, otpKind, err := s.browserAuthStart(ctx, input.GetMode(), input.GetJobId(), account, password)
	data.BrowserStart = browserStartData(startResp)
	if otpKind != "" {
		data.OtpKind = otpKind
	}
	if err != nil {
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if startResp == nil {
		err := fmt.Errorf("browser %s start returned empty response", input.GetMode())
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if !startResp.GetSuccess() {
		err := fmt.Errorf("browser %s start failed: %s", input.GetMode(), startResp.GetErrorMessage())
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}

	output.BrowserSessionId = startResp.GetBrowserSessionId()
	output.Email = gptaccount.Email(account)
	output.OtpRequired = startResp.GetOtpRequired()
	output.OtpIssuedAfterUnix = startResp.GetOtpIssuedAfterUnix()
	output.OtpWaitStartedAtUnix = startResp.GetOtpWaitStartedAtUnix()
	output.OtpRequestActionStartedAtUnix = startResp.GetOtpRequestActionStartedAtUnix()
	output.OtpTimeoutSeconds = s.registrationOtpTimeout(ctx)
	if output.GetOtpRequired() && otpKind != "" {
		if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{browserAuthOTPKindParam: otpKind}); err != nil {
			output.Data = data
			return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
		}
	}
	step.progress("browser auth flow created", data)

	if startResp.GetResult() != nil {
		result := startResp.GetResult()
		data.BrowserComplete = registerResultData(result)
		if result == nil {
			err := fmt.Errorf("browser %s completed without result", input.GetMode())
			output.Data = data
			return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
		}
		if !result.GetSuccess() {
			err := fmt.Errorf("browser %s failed: %s", input.GetMode(), result.GetErrorMessage())
			output.Data = data
			return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
		}
		resultOutput := registerActivityOutputFromResponse(result, data)
		output.Result = resultOutput
		output.Data = data
		return output, step.complete(data, nil)
	}

	output.Data = data
	return output, step.complete(data, nil)
}
