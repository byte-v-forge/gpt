package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) completeCodexOAuthAddPhone(ctx context.Context, s *Server, jobID string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, data map[string]any) (bool, error) {
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
	results, err := f.execute(s.browserAutomationClient, s.browserAuthConfig, "codex-oauth-add-phone", []*browserautomationv1.BrowserCommand{
		selectOptionGroupCommand("select-phone-country", codexOAuthPhoneCountrySelector(), []string{phone.GetCountryIso2()}, countryLabels, nil, 5*time.Second, true),
		clickCommand("open-phone-country-dropdown", codexOAuthPhoneCountryDropdownSelector(), 3*time.Second, true),
		clickCommand("click-phone-country", codexOAuthPhoneCountryOptionSelector(countryLabels), 3*time.Second, true),
		waitForSelectorCommand("wait-phone-input", codexOAuthPhoneInputSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-phone", codexOAuthPhoneInputSelector(), national, 10*time.Second, false),
		clickCommand("click-phone-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-phone-validity", codexOAuthPhoneValidationSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("phone-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return false, err
	}
	submitState := browserAuthPageStateData(results, "phone-submit-state")
	if failure := codexOAuthPhonePageFailureState(submitState); failure != "" {
		data["phone_validity_confirmed"] = false
		data["phone_validity_failure"] = failure
		return false, browserAuthStepError(f.mode, "add_phone", failure, submitState)
	}
	if !browserAuthCommandSucceeded(results, "wait-phone-validity") {
		state := "phone_otp_input_missing"
		data["phone_validity_confirmed"] = false
		data["phone_validity_failure"] = state
		return false, browserAuthStepError(f.mode, "add_phone", "phone_rejected: "+state, submitState)
	}
	data["phone_validity_confirmed"] = true
	if phone.GetReused() {
		if err := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-additional-"+jobID); err != nil {
			data["sms_request_additional_error"] = err.Error()
			return false, fmt.Errorf("phone_expired: request additional sms code failed: %w", err)
		}
	} else if err := s.markSMSMessageSent(ctx, phone.GetActivationId(), "codex-oauth-sent-"+jobID); err != nil {
		data["sms_mark_sent_error"] = err.Error()
	}
	code, err := s.waitSMSCode(ctx, phone.GetActivationId(), cfg.PhoneFirstWaitSeconds)
	if err != nil {
		data["sms_first_wait_error"] = err.Error()
		if resendErr := f.resendCodexOAuthPhoneCode(s.browserAutomationClient, s.browserAuthConfig); resendErr != nil {
			return false, resendErr
		}
		if addErr := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-resend-"+jobID); addErr != nil {
			data["sms_resend_request_error"] = addErr.Error()
		}
		code, err = s.waitSMSCode(ctx, phone.GetActivationId(), cfg.PhoneResendWaitSeconds)
		if err != nil {
			return false, fmt.Errorf("phone_sms_timeout: %w", err)
		}
	}
	data["phone_otp_received"] = true
	return true, f.submitCodexOAuthPhoneOTP(s.browserAutomationClient, s.browserAuthConfig, code)
}

func (f *browserAuthFlow) resendCodexOAuthPhoneCode(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	_, err := f.execute(client, cfg, "codex-oauth-phone-resend", []*browserautomationv1.BrowserCommand{
		clickCommand("click-resend-phone", selectorGroup(5*time.Second, roleSelector("button", "Resend text message", true), textSelector("Resend text message", true), roleSelector("button", "Resend", true), textSelector("Resend", true)), 10*time.Second, false),
		waitTimeoutCommand("wait-after-phone-resend", 500*time.Millisecond),
	})
	return err
}

func (f *browserAuthFlow) submitCodexOAuthPhoneOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, code string) error {
	results, err := f.execute(client, cfg, "codex-oauth-phone-otp", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-phone-code", codexOAuthPhoneOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-phone-code", codexOAuthPhoneOTPSelector(), normalizeOTP(code), 10*time.Second, false),
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
