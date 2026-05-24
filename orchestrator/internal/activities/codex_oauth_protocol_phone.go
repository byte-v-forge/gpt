package activities

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) CodexOAuthAddPhoneProtocolActivity(ctx context.Context, input CodexOAuthAddPhoneBrowserInput) (CodexOAuthAddPhoneBrowserOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthAddPhoneBrowserOutput{PhoneReuseCount: input.GetPhone().GetReuseCount(), PhoneReuseLimit: input.GetPhone().GetReuseLimit()}
	data := codexOAuthBrowserData(label, input.GetPhone())
	data["driver"] = "protocol"
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthProtocolAddPhone, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthProtocolAddPhone, "handling codex oauth protocol add phone", data)
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if !input.GetAllowAddPhone() {
			data["add_phone_required"] = true
			if err := s.markCodexOAuthNeedPhone(ctx, account.GetAccountId(), label, data); err != nil {
				data["account_phone_need_write_error"] = err.Error()
			}
			return data, codexOAuthAddPhoneRequiredError()
		}
		if err := ensureCodexOAuthPhoneUsableForSMS(input.GetPhone(), cfg); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		state, err := s.loadCodexOAuthProtocolState(ctx, input.GetJobId(), input.GetSession())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		client, err := newCodexOAuthProtocolHTTPClient(cfg, state)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		phoneUsed, err := s.codexOAuthProtocolAddPhone(ctx, client, state, input, cfg, data)
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
			_ = s.releaseCodexPhone(ctx, input.GetPhone(), account.GetAccountId(), input.GetJobId(), label, phoneUsed, err.Error())
			output.ErrorMessage = err.Error()
			data["error_message"] = err.Error()
			return data, err
		}
		if err := s.markCodexPhoneSuccess(ctx, input.GetPhone(), account.GetAccountId(), input.GetJobId(), label); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if state.PhonePresent {
			if err := s.markCodexOAuthPhoneConfirmed(ctx, account.GetAccountId(), label, data); err != nil {
				data["error_message"] = err.Error()
				return data, err
			}
		}
		output.Success = true
		output.AddPhoneConfirmed = state.PhonePresent
		output.AddPhoneRequired = !state.PhonePresent
		output.PhoneReuseCount = input.GetPhone().GetReuseCount()
		output.PhoneReuseLimit = input.GetPhone().GetReuseLimit()
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}

func (s *Server) codexOAuthProtocolAddPhone(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, input CodexOAuthAddPhoneBrowserInput, cfg CodexOAuthConfig, data map[string]any) (bool, error) {
	phone := input.GetPhone()
	phoneNumber := strings.TrimSpace(phone.GetPhoneE164())
	if phoneNumber == "" {
		return false, fmt.Errorf("sms phone number is empty")
	}
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/add-phone/send", "https://auth.openai.com/add-phone", map[string]any{"phone_number": phoneNumber})
	if err != nil {
		return false, err
	}
	if err := codexOAuthProtocolRequireOK(resp, "add-phone/send"); err != nil {
		data["phone_validity_confirmed"] = false
		data["phone_validity_failure"] = codexOAuthProtocolPhoneFailure(resp)
		return false, err
	}
	data["phone_validity_confirmed"] = true
	stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "add_phone", data)
	if err != nil {
		return false, err
	}
	state.Stage = stage
	if phone.GetReused() {
		if err := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-additional-"+input.GetJobId()); err != nil {
			data["sms_request_additional_error"] = err.Error()
			return false, fmt.Errorf("phone_expired: request additional sms code failed: %w", err)
		}
	} else if err := s.markSMSMessageSent(ctx, phone.GetActivationId(), "codex-oauth-sent-"+input.GetJobId()); err != nil {
		data["sms_mark_sent_error"] = err.Error()
	}
	code, err := s.waitSMSCode(ctx, phone.GetActivationId(), cfg.PhoneFirstWaitSeconds)
	if err != nil {
		data["sms_first_wait_error"] = err.Error()
		_, _ = client.postJSON(ctx, "https://auth.openai.com/api/accounts/phone-otp/resend", "https://auth.openai.com/phone-verification", map[string]any{})
		if addErr := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-resend-"+input.GetJobId()); addErr != nil {
			data["sms_resend_request_error"] = addErr.Error()
		}
		code, err = s.waitSMSCode(ctx, phone.GetActivationId(), cfg.PhoneResendWaitSeconds)
		if err != nil {
			return false, fmt.Errorf("phone_sms_timeout: %w", err)
		}
	}
	data["phone_otp_received"] = true
	validate, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/phone-otp/validate", "https://auth.openai.com/phone-verification", map[string]any{"code": normalizeOTP(code)})
	if err != nil {
		return true, err
	}
	if err := codexOAuthProtocolRequireOK(validate, "phone-otp/validate"); err != nil {
		return true, err
	}
	stage, err = advanceCodexOAuthProtocolJSON(ctx, client, state, validate, "phone_otp", data)
	if err != nil {
		return true, err
	}
	_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
	state.Stage = codexOAuthProtocolStageFromDump(state, stage)
	if codexOAuthProtocolAddPhoneConfirmedByStage(state.Stage) {
		state.PhonePresent = true
	}
	data["post_add_phone_stage"] = state.Stage
	data["add_phone_confirmed"] = state.PhonePresent
	data["add_phone_required"] = !state.PhonePresent
	if !state.PhonePresent {
		if err := s.markCodexOAuthNeedPhone(ctx, input.GetAccountId(), input.GetLabel(), data); err != nil {
			data["account_phone_need_write_error"] = err.Error()
		}
		return true, fmt.Errorf("phone_rejected: add phone status not confirmed")
	}
	return true, nil
}

func codexOAuthProtocolAddPhoneConfirmedByStage(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "callback", "consent":
		return true
	default:
		return false
	}
}

func codexOAuthProtocolPhoneFailure(resp *codexOAuthProtocolHTTPResponse) string {
	if resp == nil {
		return "phone_rejected"
	}
	text := strings.ToLower(codexOAuthProtocolSafeText(string(resp.Body), 500))
	if failure := codexOAuthPhonePageFailureState(map[string]any{"body": text}); failure != "" {
		return failure
	}
	return "phone_rejected"
}
