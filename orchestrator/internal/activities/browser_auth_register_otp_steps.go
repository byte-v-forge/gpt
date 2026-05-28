package activities

import (
	"fmt"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"orchestrator/pb"
)

func (f *browserAuthFlow) resendRegisterEmailOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-resend-email-otp", []*browserautomationv1.BrowserCommand{
		clickCommand("click-resend-email", selectorGroup(5*time.Second, roleSelector("button", "Resend email", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-resend-email-request", "https://auth.openai.com/api/accounts/email-otp/resend", "POST", 200, 299, startedAfter, 45*time.Second, true),
		waitForSelectorCommand("wait-resend-email-code", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 10*time.Second, true),
		getPageStateCommand("resend-email-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "resend-email-state")
	if !browserAuthCommandSucceeded(results, "wait-resend-email-request") {
		return 0, browserAuthStepError(f.mode, "resend", "resend_email_request_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-resend-email-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	if !browserAuthCommandSucceeded(results, "wait-resend-email-code") {
		recoveredState, recovered, recoverErr := f.recoverRegisterEmailVerificationCode(client, cfg, "register-resend-email-verification-reload")
		if recoverErr != nil {
			return 0, recoverErr
		}
		if recovered {
			return startedAt, nil
		}
		if recoveredState != nil {
			state = recoveredState
		}
		return 0, browserAuthStepError(f.mode, "resend", "email_verification_input_missing", state)
	}
	return startedAt, nil
}

func (f *browserAuthFlow) resendEmailOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (*pb.BrowserAuthResendOTPOutput, error) {
	if f.cancelled() {
		return nil, fmt.Errorf("browser auth cancelled")
	}
	if f.mode != browserAuthModeRegister {
		return &pb.BrowserAuthResendOTPOutput{
			BrowserSessionId: f.sessionID,
			Email:            f.email,
			Success:          false,
			ErrorMessage:     fmt.Sprintf("browser %s email OTP resend is not supported", f.mode),
		}, nil
	}
	f.setStatus(browserAuthStageOTPRequestClick, "resending email OTP")
	startedAt, err := f.resendRegisterEmailOTP(client, cfg)
	if err != nil {
		return nil, err
	}
	issuedAfter := unixSecondsFromMillis(startedAt)
	f.markOTPRequestClickedAt(issuedAfter)
	data := map[string]any{
		"browser_session_id":             f.sessionID,
		"mode":                           f.mode,
		"email":                          f.email,
		"otp_issued_after_unix":          issuedAfter,
		"otp_request_started_at_unix_ms": startedAt,
	}
	return &pb.BrowserAuthResendOTPOutput{
		BrowserSessionId:          f.sessionID,
		Email:                     f.email,
		Success:                   true,
		OtpIssuedAfterUnix:        issuedAfter,
		OtpRequestStartedAtUnixMs: startedAt,
		OtpTimeoutSeconds:         0,
		Data:                      protoData(data),
	}, nil
}

func (f *browserAuthFlow) submitRegisterOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-submit-otp", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-email-code", browserAuthRegisterOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-code-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-email-code-validate", "https://auth.openai.com/api/accounts/email-otp/validate", "POST", 200, 299, startedAfter, 45*time.Second, true),
		waitForSelectorCommand("wait-about-you-name", browserAuthRegisterProfileNameSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("otp-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "otp-submit-state")
	if !browserAuthCommandSucceeded(results, "wait-email-code-validate") {
		return 0, browserAuthStepError(f.mode, "otp", "email_code_validate_request_missing", state)
	}
	if !browserAuthCommandSucceeded(results, "wait-about-you-name") {
		return 0, browserAuthStepError(f.mode, "otp", "profile_name_input_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-email-code-validate")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) completeRegisterProfile(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-complete-profile", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-full-name", browserAuthRegisterProfileNameSelector(), f.fullName, 10*time.Second, false),
		fillCommand("fill-age", browserAuthRegisterAgeSelector(), f.age, 10*time.Second, false),
		clickCommand("click-finish-creating-account", selectorGroup(5*time.Second, roleSelector("button", "Finish creating account", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-create-account-request", "https://auth.openai.com/api/accounts/create_account", "POST", 200, 299, startedAfter, 90*time.Second, true),
		waitTimeoutCommand("wait-after-create-account", 3*time.Second),
		getPageStateCommand("profile-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	if !browserAuthCommandSucceeded(results, "wait-create-account-request") {
		return 0, browserAuthStepError(f.mode, "profile", "create_account_request_missing", browserAuthPageStateData(results, "profile-submit-state"))
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-create-account-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}
