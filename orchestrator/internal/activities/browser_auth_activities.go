package activities

import (
	"context"
	"fmt"
	"strings"
)

const (
	browserAuthModeRegister = "register"
	browserAuthModeLogin    = "login"
)

func (s *Server) BrowserAuthStartActivity(ctx context.Context, input BrowserAuthStartInput) (BrowserAuthStartOutput, error) {
	output := BrowserAuthStartOutput{
		AccountId: input.GetAccountId(),
	}
	data := map[string]any{}
	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return output, err
	}
	if err := rejectUserAlreadyExistsAccount(account); err != nil {
		return output, err
	}
	if input.GetMode() == browserAuthModeLogin {
		if strings.TrimSpace(account.GetEmail()) == "" {
			return output, fmt.Errorf("email is required")
		}
		if strings.TrimSpace(account.GetPassword()) == "" {
			return output, fmt.Errorf("password is required")
		}
	}

	stepName, err := browserAuthStartStepName(input.GetMode())
	if err != nil {
		return output, err
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}

	data["account_id"] = account.GetAccountId()
	data["email"] = account.GetEmail()
	step.progress("starting browser auth", map[string]any{
		"mode":  input.GetMode(),
		"email": account.GetEmail(),
	})
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "starting browser auth", data)
	defer stopHeartbeat()

	startResp, otpKind, err := s.browserAuthStart(ctx, input.GetMode(), input.GetJobId(), account)
	data["browser_start"] = browserStartData(startResp)
	if otpKind != "" {
		data["otp_kind"] = otpKind
	}
	if err != nil {
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if startResp == nil {
		err := fmt.Errorf("browser %s start returned empty response", input.GetMode())
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if !startResp.GetSuccess() {
		err := fmt.Errorf("browser %s start failed: %s", input.GetMode(), startResp.GetErrorMessage())
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}

	output.BrowserSessionId = startResp.GetBrowserSessionId()
	output.Email = account.GetEmail()
	output.OtpRequired = startResp.GetOtpRequired()
	output.OtpIssuedAfterUnix = startResp.GetOtpIssuedAfterUnix()
	output.OtpWaitStartedAtUnix = startResp.GetOtpWaitStartedAtUnix()
	output.OtpRequestActionStartedAtUnix = startResp.GetOtpRequestActionStartedAtUnix()
	output.OtpTimeoutSeconds = s.registrationOtpTimeout(ctx)
	if output.GetOtpRequired() && otpKind != "" {
		if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{browserAuthOTPKindParam: otpKind}); err != nil {
			output.Data = protoData(data)
			return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
		}
	}
	step.progress("browser auth flow created", map[string]any{
		"mode":               input.GetMode(),
		"browser_session_id": output.GetBrowserSessionId(),
		"otp_required":       output.GetOtpRequired(),
		"otp_kind":           otpKind,
	})

	if startResp.GetResult() != nil {
		result := startResp.GetResult()
		data["browser_complete"] = registerResultData(result)
		if result == nil {
			err := fmt.Errorf("browser %s completed without result", input.GetMode())
			output.Data = protoData(data)
			return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
		}
		if !result.GetSuccess() {
			err := fmt.Errorf("browser %s failed: %s", input.GetMode(), result.GetErrorMessage())
			output.Data = protoData(data)
			return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
		}
		resultOutput := registerActivityOutputFromResponse(result, data)
		output.Result = &resultOutput
		output.Data = protoData(data)
		return output, step.complete(data, nil)
	}

	output.Data = protoData(data)
	return output, step.complete(data, nil)
}
