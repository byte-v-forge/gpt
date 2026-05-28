package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) openEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	if _, err := f.execute(client, cfg, "open-entry", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-chatgpt", "https://chatgpt.com/", cfg.CommandTimeout),
	}); err != nil {
		return "", err
	}
	if f.hasBrowserAuthEmailInput(client, cfg) {
		return "email_ready", nil
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", nil
	}
	if f.tryBrowserAuthEntryClick(client, cfg, "direct-entry") {
		return "clicked", nil
	}
	if f.tryBrowserAuthProfileMenuClick(client, cfg) {
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return "session_ready", nil
		}
		if f.tryBrowserAuthEntryClick(client, cfg, "profile-entry") {
			return "clicked", nil
		}
	}
	if f.hasBrowserAuthEmailInput(client, cfg) {
		return "email_ready", nil
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", nil
	}
	data := f.browserAuthPageState(client, cfg, "entry-state")
	return "entry_missing", browserAuthStepError(f.mode, "entry", "entry_missing", data)
}

func (f *browserAuthFlow) submitEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", nil
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) {
		_ = f.tryBrowserAuthEntryClick(client, cfg, "email-entry")
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) {
		_ = f.tryBrowserAuthEmailProviderClick(client, cfg)
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) && f.tryBrowserAuthProfileMenuClick(client, cfg) {
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return "session_ready", nil
		}
		_ = f.tryBrowserAuthEntryClick(client, cfg, "email-profile-entry")
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) {
		data := f.browserAuthPageState(client, cfg, "email-state")
		return "email_input_missing", browserAuthStepError(f.mode, "email", "email_input_missing", data)
	}
	results, err := f.execute(client, cfg, "submit-email", []*browserautomationv1.BrowserCommand{
		typeTextCommand("type-email", browserAuthEmailSelector(), f.email, 15*time.Millisecond, 5*time.Second, true, false),
	})
	if err != nil {
		return "", err
	}
	if !browserAuthCommandSucceeded(results, "type-email") {
		data := f.browserAuthPageState(client, cfg, "email-state")
		return "email_input_missing", browserAuthStepError(f.mode, "email", "email_input_missing", data)
	}
	if f.tryBrowserAuthEmailSubmitForm(client, cfg) && f.browserAuthEmailSubmitted(client, cfg) {
		return "submitted", nil
	}
	if f.tryBrowserAuthEmailSubmitPress(client, cfg) && f.browserAuthEmailSubmitted(client, cfg) {
		return "submitted", nil
	}
	if f.tryBrowserAuthEmailSubmitClick(client, cfg) && f.browserAuthEmailSubmitted(client, cfg) {
		return "submitted", nil
	}
	data := f.browserAuthPageState(client, cfg, "email-state")
	return "email_submit_missing", browserAuthStepError(f.mode, "email", "email_submit_missing", data)
}

func (f *browserAuthFlow) hasBrowserAuthEmailInput(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "check-email-input", []*browserautomationv1.BrowserCommand{
		countElementsCommand("count-email-input", browserAuthEmailSelector(), 2*time.Second, true),
	})
	return err == nil && browserAuthMatchedCount(results, "count-email-input") > 0
}

func (f *browserAuthFlow) hasBrowserAuthSessionCookie(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "check-session-cookie", []*browserautomationv1.BrowserCommand{
		getCookiesCommand("check-session-cookie", []string{"https://chatgpt.com/"}, 5*time.Second),
	})
	if err != nil {
		return false
	}
	return extractBrowserSessionToken(browserCookieMaps(commandResultMap(results, "check-session-cookie"))) != ""
}

func (f *browserAuthFlow) hasBrowserAuthPasswordInput(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "check-password-input", []*browserautomationv1.BrowserCommand{
		countElementsCommand("count-password-input", browserAuthPasswordSelector(), 2*time.Second, true),
	})
	return err == nil && browserAuthMatchedCount(results, "count-password-input") > 0
}

func (f *browserAuthFlow) browserAuthEmailSubmitted(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	if f.hasBrowserAuthSessionCookie(client, cfg) || f.hasBrowserAuthPasswordInput(client, cfg) {
		return true
	}
	return !f.hasBrowserAuthEmailInput(client, cfg)
}

func (f *browserAuthFlow) tryBrowserAuthEntryClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, key string) bool {
	results, err := f.execute(client, cfg, key, []*browserautomationv1.BrowserCommand{
		clickCommand("click-entry", browserAuthEntrySelector(f.mode), 3*time.Second, true),
		waitTimeoutCommand("wait-entry", 1500*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "click-entry")
}

func (f *browserAuthFlow) tryBrowserAuthProfileMenuClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "open-profile-menu", []*browserautomationv1.BrowserCommand{
		clickCommand("open-profile-menu", browserAuthProfileMenuSelector(), 2*time.Second, true),
		waitTimeoutCommand("wait-profile-menu", 700*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "open-profile-menu")
}

func (f *browserAuthFlow) tryBrowserAuthEmailProviderClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "choose-email-provider", []*browserautomationv1.BrowserCommand{
		clickCommand("click-email-provider", browserAuthEmailProviderSelector(), 2*time.Second, true),
		waitTimeoutCommand("wait-email-provider", 700*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "click-email-provider")
}

func (f *browserAuthFlow) tryBrowserAuthEmailSubmitClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "click-email-submit", []*browserautomationv1.BrowserCommand{
		clickCommand("click-email-submit", browserAuthEmailSubmitSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-email-submit", 1200*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "click-email-submit")
}

func (f *browserAuthFlow) tryBrowserAuthEmailSubmitForm(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "submit-email-form", []*browserautomationv1.BrowserCommand{
		submitFormCommand("submit-email-form", browserAuthEmailSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-email-form-submit", 1200*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "submit-email-form")
}

func (f *browserAuthFlow) tryBrowserAuthEmailSubmitPress(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "press-email-submit", []*browserautomationv1.BrowserCommand{
		pressCommand("press-email-enter", browserAuthEmailSelector(), "Enter", 2*time.Second, true),
		waitTimeoutCommand("wait-email-enter", 1200*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "press-email-enter")
}

func (f *browserAuthFlow) browserAuthPageState(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, commandID string) map[string]any {
	results, err := f.execute(client, cfg, commandID, []*browserautomationv1.BrowserCommand{
		getPageStateCommand(commandID, true, true, false, 5*time.Second),
	})
	if err != nil {
		return map[string]any{"state": "page_state_failed", "title": err.Error()}
	}
	return browserAuthPageStateData(results, commandID)
}
