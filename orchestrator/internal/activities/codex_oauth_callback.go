package activities

import (
	"fmt"
	"strings"
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) completeCodexOAuthConsentAndCallback(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	stage, err := f.detectCodexOAuthStage(client, cfg)
	if err != nil {
		return "", err
	}
	if stage == "consent" {
		if _, err := f.execute(client, cfg, "codex-oauth-consent", []*browserautomationv1.BrowserCommand{
			clickCommand("click-consent-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		}); err != nil {
			return "", err
		}
	} else if stage != "callback" {
		return "", fmt.Errorf("codex oauth callback stage not ready: %s", stage)
	}
	results, err := f.execute(client, cfg, "codex-oauth-callback", []*browserautomationv1.BrowserCommand{
		waitForURLCommand("wait-callback-url", "http://localhost:*/auth/callback*", false, 90*time.Second, true),
		getPageStateCommand("callback-state", true, false, false, 5*time.Second),
	})
	if err != nil {
		return "", err
	}
	rawURL := stringMapValue(commandResultMap(results, "callback-state"), "url")
	if rawURL == "" {
		rawURL = stringMapValue(commandResultMap(results, "wait-callback-url"), "current_url")
	}
	if rawURL == "" {
		return "", browserAuthStepError(f.mode, "callback", "callback_url_missing", browserAuthPageStateData(results, "callback-state"))
	}
	return rawURL, nil
}

func (f *browserAuthFlow) detectCodexOAuthStage(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	results, err := f.execute(client, cfg, "codex-oauth-detect-stage", []*browserautomationv1.BrowserCommand{
		countElementsCommand("count-email", browserAuthEmailSelector(), 2*time.Second, true),
		countElementsCommand("count-password", browserAuthLoginPasswordSelector(), 2*time.Second, true),
		countElementsCommand("count-email-otp", browserAuthLoginOTPSelector(), 2*time.Second, true),
		countElementsCommand("count-phone", codexOAuthPhoneInputSelector(), 2*time.Second, true),
		countElementsCommand("count-consent", codexOAuthConsentSignalSelector(), 2*time.Second, true),
		getPageStateCommand("stage-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return "", err
	}
	rawURL := stringMapValue(commandResultMap(results, "stage-state"), "url")
	if strings.Contains(rawURL, "/auth/callback") {
		return "callback", nil
	}
	if browserAuthMatchedCount(results, "count-phone") > 0 || browserAuthPageHasAny(browserAuthPageStateData(results, "stage-state"), "Add your phone", "Phone number") {
		return "add_phone", nil
	}
	if strings.Contains(rawURL, "/sign-in-with-chatgpt/") ||
		browserAuthMatchedCount(results, "count-consent") > 0 ||
		browserAuthPageHasAny(browserAuthPageStateData(results, "stage-state"), "Codex CLI", "Sign in with ChatGPT") {
		return "consent", nil
	}
	if browserAuthMatchedCount(results, "count-email-otp") > 0 {
		return "email_otp", nil
	}
	if browserAuthMatchedCount(results, "count-password") > 0 {
		return "password", nil
	}
	if browserAuthMatchedCount(results, "count-email") > 0 {
		return "email", nil
	}
	return "unknown", nil
}
