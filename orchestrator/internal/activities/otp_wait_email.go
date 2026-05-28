package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"

	"orchestrator/internal/accountmail"
)

func (s *Server) waitEmailOTP(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	target := input.GetEmail()
	email := ""
	if target != nil {
		email = strings.TrimSpace(target.GetEmail())
	}
	if email == "" {
		return OTPWaitOutput{}, fmt.Errorf("email otp target missing")
	}
	if s.otpProjection == nil {
		return OTPWaitOutput{}, fmt.Errorf("otp projection is not configured")
	}
	timeoutSeconds := input.GetTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}
	data := map[string]any{
		"channel":           otpWaitChannelEmail,
		"email":             email,
		"timeout_seconds":   timeoutSeconds,
		"issued_after_unix": input.GetIssuedAfterUnix(),
	}
	step := s.activityStep(ctx, input.GetJobId(), input.GetStepName(), false, true)
	step.progress("waiting for email otp", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), input.GetStepName(), "waiting for email otp", data)
	defer stopHeartbeat()
	timeout := time.Duration(timeoutSeconds) * time.Second
	if s.mailboxPollRequester != nil {
		if err := s.mailboxPollRequester.RequestMailboxEmailPoll(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED, input.GetIssuedAfterUnix(), timeout, "gpt_email_otp_wait"); err != nil {
			data["error_message"] = err.Error()
			return OTPWaitOutput{Data: protoData(data)}, err
		}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	message, code, found, err := s.otpProjection.WaitMailboxSignal(reqCtx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP, input.GetIssuedAfterUnix(), timeout, defaultSMSPollInterval)
	if err != nil {
		data["error_message"] = err.Error()
		return OTPWaitOutput{Data: protoData(data)}, err
	}
	if code == "" {
		code = extractOTPFromEmailMessage(message)
	}
	if found && code != "" {
		if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{
			input.GetOtpParam():         code,
			input.GetSubmittedAtParam(): fmt.Sprintf("%d", time.Now().Unix()),
		}); err != nil {
			data["error_message"] = err.Error()
			return OTPWaitOutput{Data: protoData(data)}, err
		}
		data["found"] = true
		if message != nil {
			data["email_provider_key"] = message.GetProviderKey()
			data["message_id"] = message.GetId()
		}
		return OTPWaitOutput{Found: true, Source: otpWaitChannelEmail, Data: protoData(data)}, nil
	}
	err = fmt.Errorf("email otp not found")
	data["found"] = false
	data["error_message"] = err.Error()
	return OTPWaitOutput{Data: protoData(data)}, err
}

func extractOTPFromEmailMessage(message *mailboxv1.EmailInboxMessage) string {
	return accountmail.OTPCode(message)
}
