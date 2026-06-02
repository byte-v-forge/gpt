package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/contracts"
	"orchestrator/internal/gptaccount"
	"strings"

	"orchestrator/internal/channelotpwait"
	"orchestrator/pb"
)

func (s *Server) CodexOAuthAddPhoneProtocolActivity(ctx context.Context, input *CodexOAuthAddPhoneBrowserInput) (*CodexOAuthAddPhoneBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := &CodexOAuthAddPhoneBrowserOutput{PhoneReuseCount: input.GetPhone().GetReuseCount(), PhoneReuseLimit: input.GetPhone().GetReuseLimit()}
	data := codexOAuthBrowserData(label, input.GetPhone())
	data.setDriver("protocol")
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthProtocolAddPhone, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepCodexOAuthProtocolAddPhone, "handling codex oauth protocol add phone", data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		if !input.GetAllowAddPhone() {
			data.setAddPhoneRequired(true)
			if err := s.markCodexOAuthNeedPhone(ctx, gptaccount.ID(account), label, data); err != nil {
				data.setAccountPhoneNeedWriteError(err)
			}
			return data.messageData(), codexOAuthAddPhoneRequiredError()
		}
		if err := ensureCodexOAuthPhoneUsableForSMS(input.GetPhone(), cfg); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		state, err := s.loadCodexOAuthProtocolState(ctx, input.GetJobId(), input.GetSession())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		client, err := s.newAccountGptClient(ctx, input.GetAccountId(), cfg, state)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		phoneUsed, err := s.codexOAuthProtocolAddPhone(ctx, client, state, input, cfg, data)
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
			_ = s.releaseCodexPhone(ctx, input.GetPhone(), gptaccount.ID(account), input.GetJobId(), label, phoneUsed, err.Error())
			output.ErrorMessage = err.Error()
			data.setError(err)
			return data.messageData(), err
		}
		if err := s.markCodexPhoneSuccess(ctx, input.GetPhone(), gptaccount.ID(account), input.GetJobId(), label); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		if state.PhonePresent {
			if err := s.markCodexOAuthPhoneConfirmed(ctx, gptaccount.ID(account), label, data); err != nil {
				data.setError(err)
				return data.messageData(), err
			}
		}
		output.Success = true
		output.AddPhoneConfirmed = state.PhonePresent
		output.AddPhoneRequired = !state.PhonePresent
		output.PhoneReuseCount = input.GetPhone().GetReuseCount()
		output.PhoneReuseLimit = input.GetPhone().GetReuseLimit()
		output.Data = data.messageData()
		return data.messageData(), nil
	})
	if output.Data == nil {
		output.Data = data.messageData()
	}
	return output, err
}

func (s *Server) codexOAuthProtocolAddPhone(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, input *CodexOAuthAddPhoneBrowserInput, cfg CodexOAuthConfig, data *codexOAuthStepData) (bool, error) {
	phone := input.GetPhone()
	phoneNumber := strings.TrimSpace(phone.GetPhoneE164())
	if phoneNumber == "" {
		return false, fmt.Errorf("sms phone number is empty")
	}
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/add-phone/send", "https://auth.openai.com/add-phone", map[string]any{"phone_number": phoneNumber})
	smsIssuedAfter := codexOAuthProtocolResponseSentAtUnix(client, resp)
	if smsIssuedAfter > 0 {
		data.setPhoneOTPIssuedAfter(smsIssuedAfter)
	}
	if err != nil {
		return false, err
	}
	if err := codexOAuthProtocolRequireOK(resp, "add-phone/send"); err != nil {
		data.setPhoneValidityConfirmed(false)
		data.setPhoneValidityFailure(codexOAuthProtocolPhoneFailure(resp))
		return false, err
	}
	data.setPhoneValidityConfirmed(true)
	stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "add_phone", data)
	if err != nil {
		return false, err
	}
	state.Stage = stage
	if phone.GetReused() {
		if err := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-additional-"+input.GetJobId()); err != nil {
			data.setSMSRequestAdditionalError(err)
			return false, fmt.Errorf("phone_expired: request additional sms code failed: %w", err)
		}
	} else if err := s.markSMSMessageSent(ctx, phone.GetActivationId(), "codex-oauth-sent-"+input.GetJobId()); err != nil {
		data.setSMSMarkSentError(err)
	}
	code, err := s.waitSMSCodeIssuedAfter(ctx, phone.GetActivationId(), cfg.PhoneWaitSeconds, smsIssuedAfter)
	if err != nil {
		data.setSMSFirstWaitError(err)
		resend, _ := client.postJSON(ctx, "https://auth.openai.com/api/accounts/phone-otp/resend", "https://auth.openai.com/phone-verification", map[string]any{})
		resendIssuedAfter := codexOAuthProtocolResponseSentAtUnix(client, resend)
		if resendIssuedAfter > 0 {
			smsIssuedAfter = resendIssuedAfter
			data.setPhoneOTPResendIssuedAfter(resendIssuedAfter)
		}
		if addErr := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-resend-"+input.GetJobId()); addErr != nil {
			data.setSMSResendRequestError(addErr)
		}
		code, err = s.waitSMSCodeIssuedAfter(ctx, phone.GetActivationId(), cfg.PhoneWaitSeconds, smsIssuedAfter)
		if err != nil {
			return false, fmt.Errorf("phone_sms_timeout: %w", err)
		}
	}
	data.setPhoneOTPReceived(true)
	validate, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/phone-otp/validate", "https://auth.openai.com/phone-verification", map[string]any{"code": channelotpwait.NormalizeCode(code)})
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
	data.setPostAddPhoneStage(state.Stage)
	data.setAddPhoneConfirmed(state.PhonePresent)
	data.setAddPhoneRequired(!state.PhonePresent)
	if !state.PhonePresent {
		if err := s.markCodexOAuthNeedPhone(ctx, input.GetAccountId(), input.GetLabel(), data); err != nil {
			data.setAccountPhoneNeedWriteError(err)
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
