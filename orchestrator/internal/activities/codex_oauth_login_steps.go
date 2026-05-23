package activities

import (
	"context"
	"fmt"
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
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

func (f *browserAuthFlow) ensureCodexOAuthLoggedIn(ctx context.Context, s *Server, account *pb.Account, jobID string, data map[string]any) error {
	cfg := s.browserAuthConfig
	stage, err := f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
	if err != nil {
		return err
	}
	if stage != "email" {
		data["login_stage"] = stage
		return nil
	}
	if err := f.submitCodexOAuthEmail(s.browserAutomationClient, cfg, account.GetEmail()); err != nil {
		return err
	}
	issuedAfter, err := f.submitCodexOAuthPassword(s.browserAutomationClient, cfg, account.GetPassword())
	if err != nil {
		return err
	}
	stage, err = f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
	if err != nil {
		return err
	}
	if stage == "email_otp" {
		otp, err := s.waitCodexOAuthEmailOTP(ctx, jobID, account.GetEmail(), issuedAfter)
		if err != nil {
			return err
		}
		if err := f.submitCodexOAuthOTP(s.browserAutomationClient, cfg, otp); err != nil {
			return err
		}
		stage, err = f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
		if err != nil {
			return err
		}
	}
	data["login_stage"] = stage
	return nil
}

func (f *browserAuthFlow) submitCodexOAuthEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, email string) error {
	results, err := f.execute(client, cfg, "codex-oauth-email", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-input", browserAuthEmailSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-email", browserAuthEmailSelector(), email, 10*time.Second, false),
		clickCommand("click-email-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-password-or-otp", selectorGroup(45*time.Second, browserAuthLoginPasswordSelector(), browserAuthLoginOTPSelector()), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 45*time.Second, true),
		getPageStateCommand("email-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if browserAuthAnyCommandSucceeded(results, "wait-password-or-otp") {
		return nil
	}
	return browserAuthStepError(f.mode, "email", "next_step_missing", browserAuthPageStateData(results, "email-state"))
}

func (f *browserAuthFlow) submitCodexOAuthPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, password string) (int64, error) {
	issuedAfter := time.Now().Add(-time.Second).Unix()
	results, err := f.execute(client, cfg, "codex-oauth-password", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-password-input", browserAuthLoginPasswordSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-password", browserAuthLoginPasswordSelector(), password, 10*time.Second, false),
		clickCommand("click-password-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitTimeoutCommand("wait-after-password", time.Second),
		waitForURLCommand("wait-password-callback-url", "http://localhost:*/auth/callback*", false, 5*time.Second, true),
		waitForSelectorGroupCommand("wait-post-password", codexOAuthPostPasswordSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("password-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return issuedAfter, err
	}
	state := browserAuthPageStateData(results, "password-state")
	if browserAuthAnyCommandSucceeded(results, "wait-password-callback-url", "wait-post-password") || browserAuthPageHasAny(state, "/auth/callback") {
		return issuedAfter, nil
	}
	return issuedAfter, browserAuthStepError(f.mode, "password", "next_step_missing", state)
}

func (s *Server) waitCodexOAuthEmailOTP(ctx context.Context, _ string, email string, issuedAfter int64) (string, error) {
	wait := s.regOTPTimeout
	if wait <= 0 {
		wait = defaultCodexOAuthPhoneWaitSeconds
	}
	if s.mailboxClient == nil {
		return "", fmt.Errorf("mailbox client not configured")
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(wait+5)*time.Second)
	defer cancel()
	resp, err := s.mailboxClient.WaitForMailboxEmail(reqCtx, &pb.WaitForEmailRequest{
		EmailAddress:    email,
		TimeoutSeconds:  wait,
		IssuedAfterUnix: issuedAfter,
		SignalKind:      pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP,
	})
	if err != nil {
		return "", err
	}
	code := extractOTPFromEmailMessage(resp.GetMessage())
	if !resp.GetFound() || code == "" {
		return "", fmt.Errorf("codex oauth email otp not found")
	}
	return code, nil
}

func (f *browserAuthFlow) submitCodexOAuthOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) error {
	results, err := f.execute(client, cfg, "codex-oauth-email-otp", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-otp", browserAuthLoginOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-email-otp", browserAuthLoginOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-email-otp-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitTimeoutCommand("wait-after-email-otp", time.Second),
		waitForURLCommand("wait-email-otp-callback-url", "http://localhost:*/auth/callback*", false, 5*time.Second, true),
		waitForSelectorGroupCommand("wait-post-email-otp", codexOAuthPostEmailOTPSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("email-otp-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	state := browserAuthPageStateData(results, "email-otp-state")
	if browserAuthAnyCommandSucceeded(results, "wait-email-otp-callback-url", "wait-post-email-otp") || browserAuthPageHasAny(state, "/auth/callback") {
		return nil
	}
	return browserAuthStepError(f.mode, "email_otp", "next_step_missing", state)
}
