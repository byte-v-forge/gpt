package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/gptaccount"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"orchestrator/pb"
)

func (f *browserAuthFlow) openCodexOAuthEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, authorizeURL string) error {
	results, err := f.execute(client, cfg, "codex-oauth-open", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-codex-oauth", authorizeURL, cfg.CommandTimeout),
		clickCommand("reject-cookies", browserAuthRejectCookiesSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-oauth-entry", 500*time.Millisecond),
		getPageStateCommand("oauth-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	state := browserAuthPageStateData(results, "oauth-entry-state")
	if browserAuthPageHasAny(state, "error", "blocked") {
		return browserAuthStepError(f.mode, "entry", "oauth_entry_error", state)
	}
	return nil
}

func (f *browserAuthFlow) ensureCodexOAuthLoggedIn(ctx context.Context, s *Server, account *pb.Account, jobID string, data *codexOAuthStepData) error {
	cfg := s.browserAuthConfig
	stage, err := f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
	if err != nil {
		return err
	}
	if stage != "email" {
		data.setStage(stage)
		return nil
	}
	if _, err := f.submitCodexOAuthEmail(s.browserAutomationClient, cfg, gptaccount.Email(account)); err != nil {
		return err
	}
	if _, err := f.submitCodexOAuthPassword(s.browserAutomationClient, cfg, f.password); err != nil {
		return err
	}
	stage, err = f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
	if err != nil {
		return err
	}
	if stage == "email_otp" {
		return fmt.Errorf("codex oauth email otp requires channel otp wait step")
	}
	data.setStage(stage)
	return nil
}

func (f *browserAuthFlow) submitCodexOAuthEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, email string) (int64, error) {
	networkFilterStartedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "codex-oauth-email", []*browserautomationv1.BrowserCommand{
		waitForLoadStateCommand("wait-email-dom-ready", browserautomationv1.BrowserLoadState_BROWSER_LOAD_STATE_DOM_CONTENT_LOADED, 10*time.Second, true),
		waitForLoadStateCommand("wait-email-network-idle", browserautomationv1.BrowserLoadState_BROWSER_LOAD_STATE_NETWORK_IDLE, 5*time.Second, true),
		waitTimeoutCommand("settle-email-page", 750*time.Millisecond),
		waitForSelectorCommand("wait-email-input", browserAuthEmailSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		typeTextCommand("type-email", browserAuthEmailSelector(), email, 20*time.Millisecond, 10*time.Second, true, false),
		waitTimeoutCommand("settle-email-value", 500*time.Millisecond),
		clickCommand("click-email-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-codex-oauth-email-request", "https://auth.openai.com/api/accounts/authorize/continue", "POST", 200, 399, networkFilterStartedAfter, 30*time.Second, false),
		waitForSelectorGroupCommand("wait-password-or-otp", selectorGroup(45*time.Second, browserAuthLoginPasswordSelector(), browserAuthLoginOTPSelector()), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 45*time.Second, true),
		getPageStateCommand("email-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-codex-oauth-email-request")
	if startedAt <= 0 {
		return 0, browserAuthStepError(f.mode, "email", "email_request_started_at_missing", browserAuthPageStateData(results, "email-state"))
	}
	issuedAfter := unixSecondsFromMillis(startedAt)
	if browserAuthAnyCommandSucceeded(results, "wait-password-or-otp") {
		return issuedAfter, nil
	}
	return issuedAfter, browserAuthStepError(f.mode, "email", "next_step_missing", browserAuthPageStateData(results, "email-state"))
}

func (f *browserAuthFlow) submitCodexOAuthPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, password string) (int64, error) {
	networkFilterStartedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "codex-oauth-password", []*browserautomationv1.BrowserCommand{
		waitForLoadStateCommand("wait-password-dom-ready", browserautomationv1.BrowserLoadState_BROWSER_LOAD_STATE_DOM_CONTENT_LOADED, 10*time.Second, true),
		waitTimeoutCommand("settle-password-page", 500*time.Millisecond),
		waitForSelectorCommand("wait-password-input", browserAuthLoginPasswordSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		typeTextCommand("type-password", browserAuthLoginPasswordSelector(), password, 20*time.Millisecond, 10*time.Second, true, false),
		waitTimeoutCommand("settle-password-value", 500*time.Millisecond),
		clickCommand("click-password-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-codex-oauth-password-request", "https://auth.openai.com/api/accounts/password/verify", "POST", 200, 399, networkFilterStartedAfter, 45*time.Second, false),
		waitTimeoutCommand("wait-after-password", time.Second),
	})
	if err != nil {
		return 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-codex-oauth-password-request")
	if startedAt <= 0 {
		return 0, fmt.Errorf("browser %s password step failed: password_request_started_at_missing", f.mode)
	}
	issuedAfter := unixSecondsFromMillis(startedAt)
	return issuedAfter, f.waitCodexOAuthPostLoginTransition(client, cfg, "codex-oauth-password", "password", "wait-post-password", codexOAuthPostPasswordSelectorGroup(60*time.Second))
}

func (f *browserAuthFlow) submitCodexOAuthOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) error {
	if _, err := f.execute(client, cfg, "codex-oauth-email-otp", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-otp", browserAuthLoginOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-email-otp", browserAuthLoginOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-email-otp-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitTimeoutCommand("wait-after-email-otp", time.Second),
	}); err != nil {
		return err
	}
	return f.waitCodexOAuthPostLoginTransition(client, cfg, "codex-oauth-email-otp", "email_otp", "wait-post-email-otp", codexOAuthPostEmailOTPSelectorGroup(60*time.Second))
}

func (f *browserAuthFlow) waitCodexOAuthPostLoginTransition(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, taskPrefix, stage, selectorCommandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup) error {
	if ok, state, err := f.waitCodexOAuthPostURL(client, cfg, taskPrefix+"-callback", "wait-"+stage+"-callback-url", "http://localhost:*/auth/callback*", "callback-state", 5*time.Second); err != nil {
		return err
	} else if ok || codexOAuthPostLoginStateReady(state) {
		return nil
	}
	if ok, state, err := f.waitCodexOAuthPostURL(client, cfg, taskPrefix+"-consent", "wait-"+stage+"-consent-url", "**/sign-in-with-chatgpt/**", "consent-state", 10*time.Second); err != nil {
		return err
	} else if ok || codexOAuthPostLoginStateReady(state) {
		return nil
	}
	results, err := f.execute(client, cfg, taskPrefix+"-stage", []*browserautomationv1.BrowserCommand{
		waitForSelectorGroupCommand(selectorCommandID, selectorGroup, browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand(stage+"-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	state := browserAuthPageStateData(results, stage+"-state")
	if browserAuthCommandSucceeded(results, selectorCommandID) || codexOAuthPostLoginStateReady(state) {
		return nil
	}
	return browserAuthStepError(f.mode, stage, "next_step_missing", state)
}

func (f *browserAuthFlow) waitCodexOAuthPostURL(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, taskKey, commandID, pattern, stateCommandID string, timeout time.Duration) (bool, map[string]any, error) {
	results, err := f.execute(client, cfg, taskKey, []*browserautomationv1.BrowserCommand{
		waitForURLCommand(commandID, pattern, false, timeout, true),
		getPageStateCommand(stateCommandID, true, true, false, 5*time.Second),
	})
	if err != nil {
		return false, nil, err
	}
	return browserAuthCommandSucceeded(results, commandID), browserAuthPageStateData(results, stateCommandID), nil
}

func codexOAuthPostLoginStateReady(state map[string]any) bool {
	return browserAuthPageHasAny(state, "/auth/callback", "/sign-in-with-chatgpt/", "Codex CLI", "Sign in with ChatGPT")
}
