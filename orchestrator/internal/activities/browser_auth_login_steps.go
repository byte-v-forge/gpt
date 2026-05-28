package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) openLoginEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	return f.openAuthLoginEntry(client, cfg)
}

func (f *browserAuthFlow) submitLoginEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "login-submit-email", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-email", browserAuthRegisterEmailSelector(), f.email, 10*time.Second, false),
		clickCommand("click-email-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-email-submit-request", "https://chatgpt.com/api/auth/signin/openai", "POST", 200, 299, startedAfter, 30*time.Second, true),
		waitForSelectorGroupCommand("wait-email-verification-or-password", browserAuthLoginEmailAdvancedSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 35*time.Second, true),
		getPageStateCommand("email-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	if !browserAuthCommandSucceeded(results, "wait-email-verification-or-password") {
		return 0, browserAuthStepError(f.mode, "email", "email_advance_missing", browserAuthPageStateData(results, "email-submit-state"))
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-email-submit-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) openLoginPasswordEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	results, err := f.execute(client, cfg, "login-password-entry", []*browserautomationv1.BrowserCommand{
		clickCommand("click-continue-with-password", selectorGroup(5*time.Second, roleSelector("link", "Continue with password", true)), 10*time.Second, false),
		waitForSelectorCommand("wait-current-password-input", browserAuthLoginPasswordSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("password-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if !browserAuthCommandSucceeded(results, "wait-current-password-input") {
		return browserAuthStepError(f.mode, "password_entry", "password_input_missing", browserAuthPageStateData(results, "password-entry-state"))
	}
	return nil
}

func (f *browserAuthFlow) submitLoginPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "login-submit-password", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-password", browserAuthLoginPasswordSelector(), f.password, 10*time.Second, false),
		clickCommand("click-password-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-login-auth-request", "https://auth.openai.com", "POST", 200, 399, startedAfter, 45*time.Second, true),
		waitTimeoutCommand("wait-after-password", 5*time.Second),
		waitForSelectorCommand("wait-login-otp", browserAuthLoginOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 2*time.Second, true),
		getPageStateCommand("password-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return "", 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-login-auth-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	if browserAuthCommandSucceeded(results, "wait-login-otp") {
		return "otp_required", startedAt, nil
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", startedAt, nil
	}
	state := browserAuthPageStateData(results, "password-submit-state")
	if browserAuthPageHasAny(state, "Open profile menu", "Claim offer", "New chat", "auth/callback/openai") {
		return "password_submitted", startedAt, nil
	}
	return "", startedAt, browserAuthStepError(f.mode, "password", "session_or_otp_missing", state)
}

func (f *browserAuthFlow) submitLoginOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "login-submit-otp", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-login-code", browserAuthLoginOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-code-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-login-code-validate", "https://auth.openai.com", "POST", 200, 399, startedAfter, 45*time.Second, true),
		waitTimeoutCommand("wait-after-login-code", 5*time.Second),
		getPageStateCommand("otp-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-login-code-validate")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}
