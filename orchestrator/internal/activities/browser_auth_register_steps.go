package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) openRegisterEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	return f.openAuthLoginEntry(client, cfg)
}

func (f *browserAuthFlow) openAuthLoginEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	results, err := f.execute(client, cfg, f.mode+"-open-auth-login", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-auth-login", "https://chatgpt.com/auth/login", cfg.CommandTimeout),
		clickCommand("reject-cookies", browserAuthRejectCookiesSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-cookies-dismissed", 500*time.Millisecond),
		getCookiesCommand("auth-entry-cookies", []string{"https://chatgpt.com/"}, 5*time.Second),
		countElementsCommand("count-email-input", browserAuthRegisterEmailSelector(), 2*time.Second, true),
		getPageStateCommand("auth-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if f.mode == browserAuthModeLogin && extractBrowserSessionToken(browserCookieMaps(commandResultMap(results, "auth-entry-cookies"))) != "" {
		return nil
	}
	if browserAuthMatchedCount(results, "count-email-input") > 0 {
		return nil
	}
	results, err = f.execute(client, cfg, f.mode+"-wait-auth-login-email", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-input", browserAuthRegisterEmailSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, true),
		getCookiesCommand("auth-wait-cookies", []string{"https://chatgpt.com/"}, 5*time.Second),
		getPageStateCommand("auth-wait-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if f.mode == browserAuthModeLogin && extractBrowserSessionToken(browserCookieMaps(commandResultMap(results, "auth-wait-cookies"))) != "" {
		return nil
	}
	if browserAuthCommandSucceeded(results, "wait-email-input") {
		return nil
	}
	state := browserAuthPageStateData(results, "auth-wait-state")
	return browserAuthStepError(f.mode, "entry", "email_input_missing", state)
}

func (f *browserAuthFlow) submitRegisterEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-submit-email", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-email", browserAuthRegisterEmailSelector(), f.email, 10*time.Second, false),
		clickCommand("click-email-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-email-submit-request", "https://chatgpt.com/api/auth/signin/openai", "POST", 200, 299, startedAfter, 30*time.Second, true),
		waitForSelectorCommand("wait-email-verification-code", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("email-verification-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "email-verification-state")
	if !browserAuthCommandSucceeded(results, "wait-email-submit-request") {
		return 0, browserAuthStepError(f.mode, "email", "email_submit_request_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-email-submit-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	if !browserAuthCommandSucceeded(results, "wait-email-verification-code") {
		recoveredState, recovered, recoverErr := f.recoverRegisterEmailVerificationCode(client, cfg, "register-email-verification-reload")
		if recoverErr != nil {
			return 0, recoverErr
		}
		if recovered {
			return startedAt, nil
		}
		if recoveredState != nil {
			state = recoveredState
		}
		return 0, browserAuthStepError(f.mode, "email", "email_verification_input_missing", state)
	}
	return startedAt, nil
}

func (f *browserAuthFlow) openRegisterPasswordEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	results, err := f.execute(client, cfg, "register-password-entry", []*browserautomationv1.BrowserCommand{
		clickCommand("click-continue-with-password", selectorGroup(5*time.Second, roleSelector("link", "Continue with password", true)), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-password-input", selectorGroup(20*time.Second, browserAuthRegisterPasswordSelector(), browserAuthLoginPasswordSelector()), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, true),
		countElementsCommand("count-new-password-input", browserAuthRegisterPasswordSelector(), 2*time.Second, true),
		countElementsCommand("count-current-password-input", browserAuthLoginPasswordSelector(), 2*time.Second, true),
		getPageStateCommand("password-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return "", err
	}
	state := browserAuthPageStateData(results, "password-entry-state")
	if browserAuthMatchedCount(results, "count-new-password-input") > 0 {
		return "new_password", nil
	}
	if browserAuthMatchedCount(results, "count-current-password-input") > 0 ||
		browserAuthPageHasAny(state, "log-in/password", "Enter your password", "Forgot password") {
		return "existing_login_password", nil
	}
	if !browserAuthCommandSucceeded(results, "wait-password-input") {
		return "", browserAuthStepError(f.mode, "password_entry", "password_input_missing", state)
	}
	return "", browserAuthStepError(f.mode, "password_entry", "password_input_unknown", state)
}

func (f *browserAuthFlow) submitRegisterPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-submit-password", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-password", browserAuthRegisterPasswordSelector(), f.password, 10*time.Second, false),
		clickCommand("click-password-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-password-register-request", "https://auth.openai.com/api/accounts/user/register", "POST", 200, 299, startedAfter, 45*time.Second, true),
		waitForSelectorCommand("wait-password-email-verification-code", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("password-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "password-submit-state")
	if !browserAuthCommandSucceeded(results, "wait-password-register-request") {
		return 0, browserAuthStepError(f.mode, "password", "password_register_request_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-password-register-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	if !browserAuthCommandSucceeded(results, "wait-password-email-verification-code") {
		recoveredState, recovered, recoverErr := f.recoverRegisterEmailVerificationCode(client, cfg, "register-password-email-verification-reload")
		if recoverErr != nil {
			return 0, recoverErr
		}
		if recovered {
			return startedAt, nil
		}
		if recoveredState != nil {
			state = recoveredState
		}
		return 0, browserAuthStepError(f.mode, "password", "email_verification_input_missing", state)
	}
	return startedAt, nil
}

func (f *browserAuthFlow) recoverRegisterEmailVerificationCode(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, taskKey string) (map[string]any, bool, error) {
	results, err := f.execute(client, cfg, taskKey, []*browserautomationv1.BrowserCommand{
		waitTimeoutCommand("wait-before-email-verification-reload", 2*time.Second),
		reloadCommand("reload-email-verification", 20*time.Second, true),
		waitForLoadStateCommand("wait-email-verification-dom", browserautomationv1.BrowserLoadState_BROWSER_LOAD_STATE_DOM_CONTENT_LOADED, 20*time.Second, true),
		waitForSelectorCommand("wait-email-verification-code-after-reload", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 45*time.Second, true),
		getPageStateCommand("email-verification-reload-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return nil, false, err
	}
	state := browserAuthPageStateData(results, "email-verification-reload-state")
	return state, browserAuthCommandSucceeded(results, "wait-email-verification-code-after-reload"), nil
}
