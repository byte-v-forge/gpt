package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"

	"orchestrator/internal/channelotpwait"
)

func (f *browserAuthFlow) completeCodexOAuthAddPhone(ctx context.Context, s *Server, jobID string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, data *codexOAuthStepData) (bool, error) {
	if err := validateCodexOAuthSMSCountry(phone.GetCountryIso2()); err != nil {
		return false, err
	}
	countryLabels := codexOAuthPhoneCountryLabels(phone)
	national := phone.GetPhoneNational()
	if strings.TrimSpace(national) == "" {
		national = strings.TrimPrefix(phone.GetPhoneE164(), "+"+phone.GetCountryCallingCode())
	}
	if strings.TrimSpace(national) == "" {
		return false, fmt.Errorf("sms phone number is empty")
	}
	networkFilterStartedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(s.browserAutomationClient, s.browserAuthConfig, "codex-oauth-add-phone", []*browserautomationv1.BrowserCommand{
		selectOptionGroupCommand("select-phone-country", codexOAuthPhoneCountrySelector(), []string{phone.GetCountryIso2()}, countryLabels, nil, 5*time.Second, true),
		clickCommand("open-phone-country-dropdown", codexOAuthPhoneCountryDropdownSelector(), 3*time.Second, true),
		clickCommand("click-phone-country", codexOAuthPhoneCountryOptionSelector(countryLabels), 3*time.Second, true),
		waitForSelectorCommand("wait-phone-input", codexOAuthPhoneInputSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-phone", codexOAuthPhoneInputSelector(), national, 10*time.Second, false),
		clickCommand("click-phone-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-codex-oauth-add-phone-request", "https://auth.openai.com/api/accounts/add-phone/send", "POST", 200, 499, networkFilterStartedAfter, 30*time.Second, true),
		waitForSelectorGroupCommand("wait-phone-validity", codexOAuthPhoneValidationSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("phone-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return false, err
	}
	submitState := browserAuthPageStateData(results, "phone-submit-state")
	if failure := codexOAuthPhonePageFailureState(submitState); failure != "" {
		data.setPhoneValidityConfirmed(false)
		data.setPhoneValidityFailure(failure)
		return false, browserAuthStepError(f.mode, "add_phone", failure, submitState)
	}
	if !browserAuthCommandSucceeded(results, "wait-phone-validity") {
		state := "phone_otp_input_missing"
		data.setPhoneValidityConfirmed(false)
		data.setPhoneValidityFailure(state)
		return false, browserAuthStepError(f.mode, "add_phone", "phone_rejected: "+state, submitState)
	}
	data.setPhoneValidityConfirmed(true)
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-codex-oauth-add-phone-request")
	if startedAt <= 0 {
		return false, browserAuthStepError(f.mode, "add_phone", "add_phone_request_started_at_missing", submitState)
	}
	smsIssuedAfter := unixSecondsFromMillis(startedAt)
	if smsIssuedAfter > 0 {
		data.setPhoneOTPIssuedAfter(smsIssuedAfter)
	}
	if phone.GetReused() {
		if err := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-additional-"+jobID); err != nil {
			data.setSMSRequestAdditionalError(err)
			return false, fmt.Errorf("phone_expired: request additional sms code failed: %w", err)
		}
	} else if err := s.markSMSMessageSent(ctx, phone.GetActivationId(), "codex-oauth-sent-"+jobID); err != nil {
		data.setSMSMarkSentError(err)
	}
	code, err := s.waitSMSCodeIssuedAfter(ctx, phone.GetActivationId(), cfg.PhoneWaitSeconds, smsIssuedAfter)
	if err != nil {
		data.setSMSFirstWaitError(err)
		resendIssuedAfter, resendErr := f.resendCodexOAuthPhoneCode(s.browserAutomationClient, s.browserAuthConfig)
		if resendErr != nil {
			data.setPhoneResendClickError(resendErr)
		} else if resendIssuedAfter > 0 {
			smsIssuedAfter = resendIssuedAfter
			data.setPhoneOTPResendIssuedAfter(resendIssuedAfter)
		}
		if addErr := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-resend-"+jobID); addErr != nil {
			data.setSMSResendRequestError(addErr)
		}
		code, err = s.waitSMSCodeIssuedAfter(ctx, phone.GetActivationId(), cfg.PhoneWaitSeconds, smsIssuedAfter)
		if err != nil {
			return false, fmt.Errorf("phone_sms_timeout: %w", err)
		}
	}
	data.setPhoneOTPReceived(true)
	return true, f.submitCodexOAuthPhoneOTP(s.browserAutomationClient, s.browserAuthConfig, code)
}

func (f *browserAuthFlow) resendCodexOAuthPhoneCode(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	networkFilterStartedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "codex-oauth-phone-resend", []*browserautomationv1.BrowserCommand{
		clickCommand("click-resend-phone", selectorGroup(5*time.Second, roleSelector("button", "Resend text message", true), textSelector("Resend text message", true), roleSelector("button", "Resend", true), textSelector("Resend", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-codex-oauth-phone-resend-request", "https://auth.openai.com/api/accounts/phone-otp/resend", "POST", 200, 499, networkFilterStartedAfter, 30*time.Second, true),
		waitTimeoutCommand("wait-after-phone-resend", 500*time.Millisecond),
	})
	if err != nil {
		return 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-codex-oauth-phone-resend-request")
	if startedAt <= 0 {
		return 0, fmt.Errorf("phone_resend_request_started_at_missing")
	}
	return unixSecondsFromMillis(startedAt), nil
}

func (f *browserAuthFlow) submitCodexOAuthPhoneOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, code string) error {
	results, err := f.execute(client, cfg, "codex-oauth-phone-otp", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-phone-code", codexOAuthPhoneOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-phone-code", codexOAuthPhoneOTPSelector(), channelotpwait.NormalizeCode(code), 10*time.Second, false),
		clickCommand("click-phone-code-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-post-phone", codexOAuthStageSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("phone-otp-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	state := browserAuthPageStateData(results, "phone-otp-state")
	if browserAuthCommandSucceeded(results, "wait-post-phone") {
		return nil
	}
	failure := codexOAuthPhonePageFailureState(state)
	if failure == "" {
		failure = "next_step_missing"
	}
	return browserAuthStepError(f.mode, "phone_otp", failure, state)
}
