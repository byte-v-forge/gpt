package activities

import (
	"fmt"
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) completePostOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	waitLimit := 2 * cfg.CommandTimeout
	if waitLimit < 90*time.Second {
		waitLimit = 90 * time.Second
	}
	deadline := time.Now().Add(waitLimit)
	var last map[string]any
	for time.Now().Before(deadline) {
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return nil
		}
		last = f.browserAuthPageState(client, cfg, "post-otp-state")
		if browserAuthPageHasAny(last, "age") {
			_ = f.tryBrowserAuthAgeProfile(client, cfg)
		} else {
			_ = f.tryBrowserAuthBirthdayProfile(client, cfg)
		}
		_ = f.tryBrowserAuthPostOTPContinue(client, cfg)
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return nil
		}
		_, _ = f.execute(client, cfg, "wait-post-otp", []*browserautomationv1.BrowserCommand{
			waitTimeoutCommand("wait-post-otp", 1500*time.Millisecond),
		})
	}
	if last == nil {
		last = f.browserAuthPageState(client, cfg, "post-otp-state")
	}
	return browserAuthStepError(f.mode, "post_otp", "session_cookie_missing", last)
}

func (f *browserAuthFlow) waitForBrowserAuthSessionCookie(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	f.setStatus(browserAuthStageSessionCapture, "waiting for browser session cookie")
	waitLimit := cfg.CommandTimeout
	if waitLimit < 60*time.Second {
		waitLimit = 60 * time.Second
	}
	deadline := time.Now().Add(waitLimit)
	var last map[string]any
	for time.Now().Before(deadline) {
		results, err := f.execute(client, cfg, "wait-session-cookie", []*browserautomationv1.BrowserCommand{
			getCookiesCommand("wait-session-cookies", []string{"https://chatgpt.com/"}, 5*time.Second),
			getPageStateCommand("wait-session-state", true, true, false, 5*time.Second),
		})
		if err == nil {
			state := browserAuthPageStateData(results, "wait-session-state")
			if state != nil {
				last = state
			}
			cookies := browserCookieMaps(commandResultMap(results, "wait-session-cookies"))
			if extractBrowserSessionToken(cookies) != "" {
				return nil
			}
		}
		_, _ = f.execute(client, cfg, "wait-session-cookie-delay", []*browserautomationv1.BrowserCommand{
			waitTimeoutCommand("wait-session-cookie-delay", 1500*time.Millisecond),
		})
	}
	if last == nil {
		last = f.browserAuthPageState(client, cfg, "wait-session-state")
	}
	return browserAuthStepError(f.mode, "session", "session_cookie_missing", last)
}

func (f *browserAuthFlow) tryBrowserAuthAgeProfile(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "complete-registration-profile", []*browserautomationv1.BrowserCommand{
		fillGroupCommand("fill-profile-name", browserAuthProfileNameSelector(), f.fullName, 2*time.Second, true),
		fillGroupCommand("fill-profile-age", browserAuthAgeSelector(), browserAuthAgeFromBirthday(f.birthday), 2*time.Second, true),
		clickCommand("submit-profile", browserAuthPostOTPContinueSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-profile-submit", 1500*time.Millisecond),
	})
	return err == nil && browserAuthAnyCommandSucceeded(results,
		"fill-profile-name",
		"fill-profile-age",
		"submit-profile",
	)
}

func (f *browserAuthFlow) tryBrowserAuthBirthdayProfile(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	birthday := browserAuthBirthdayPartsFrom(f.birthday)
	results, err := f.execute(client, cfg, "complete-registration-profile", []*browserautomationv1.BrowserCommand{
		fillGroupCommand("fill-profile-name", browserAuthProfileNameSelector(), f.fullName, 2*time.Second, true),
		fillGroupCommand("fill-profile-birthday", browserAuthBirthdaySelector(), birthday.US, 2*time.Second, true),
		fillGroupCommand("fill-profile-month", browserAuthMonthInputSelector(), birthday.Month, time.Second, true),
		fillGroupCommand("fill-profile-day", browserAuthDayInputSelector(), birthday.Day, time.Second, true),
		fillGroupCommand("fill-profile-year", browserAuthYearInputSelector(), birthday.Year, time.Second, true),
		selectOptionGroupCommand("select-profile-month", browserAuthMonthSelectSelector(), []string{birthday.Month, birthday.MonthPadded}, []string{birthday.MonthName, birthday.MonthShort}, nil, time.Second, true),
		selectOptionGroupCommand("select-profile-day", browserAuthDaySelectSelector(), []string{birthday.Day, birthday.DayPadded}, nil, nil, time.Second, true),
		selectOptionGroupCommand("select-profile-year", browserAuthYearSelectSelector(), []string{birthday.Year}, nil, nil, time.Second, true),
		clickCommand("submit-profile", browserAuthPostOTPContinueSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-profile-submit", 1500*time.Millisecond),
	})
	return err == nil && browserAuthAnyCommandSucceeded(results,
		"fill-profile-name",
		"fill-profile-birthday",
		"fill-profile-month",
		"fill-profile-day",
		"fill-profile-year",
		"select-profile-month",
		"select-profile-day",
		"select-profile-year",
		"submit-profile",
	)
}

func (f *browserAuthFlow) tryBrowserAuthPostOTPContinue(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "post-otp-continue", []*browserautomationv1.BrowserCommand{
		clickCommand("post-otp-continue", browserAuthPostOTPContinueSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-post-otp-continue", 1500*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "post-otp-continue")
}

func (f *browserAuthFlow) captureResult(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, requireCredentials bool) error {
	f.setStatus(browserAuthStageSessionCapture, "capturing browser session")
	results, err := f.execute(client, cfg, "capture-session", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-session-endpoint", "https://chatgpt.com/api/auth/session", 30*time.Second),
		extractTextCommand("extract-session-body", cssSelector("body"), 10*time.Second, false),
		getCookiesCommand("capture-cookies", []string{"https://chatgpt.com/"}, 10*time.Second),
	})
	if err != nil {
		return err
	}
	result := browserAuthRegisterResponse(results)
	if requireCredentials && result.GetSessionToken() == "" {
		return fmt.Errorf("missing session token after browser %s", f.mode)
	}
	if requireCredentials && result.GetAccessToken() == "" {
		return fmt.Errorf("missing access token after browser %s", f.mode)
	}
	f.mu.Lock()
	f.result = result
	f.success = true
	f.errMessage = ""
	f.done = true
	f.stage = browserAuthStageSucceeded
	f.message = fmt.Sprintf("browser %s completed", f.mode)
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
	return nil
}

func (f *browserAuthFlow) handleTerminalState(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, state string) bool {
	switch state {
	case "session_ready":
		if err := f.captureResult(client, cfg, true); err != nil {
			f.fail(err)
		}
		return true
	case "user_already_exists":
		f.fail(fmt.Errorf("account already exists"))
		return true
	default:
		return false
	}
}
