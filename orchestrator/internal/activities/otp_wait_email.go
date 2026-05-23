package activities

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	pb "orchestrator/pb"
)

var emailOTPPattern = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)

func (s *Server) waitEmailOTP(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	target := input.GetEmail()
	email := ""
	if target != nil {
		email = strings.TrimSpace(target.GetEmail())
	}
	if email == "" {
		return OTPWaitOutput{}, fmt.Errorf("email otp target missing")
	}
	if s.mailboxClient == nil {
		return OTPWaitOutput{}, fmt.Errorf("mailbox client not configured")
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
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()
	resp, err := s.mailboxClient.WaitForMailboxEmail(reqCtx, &pb.WaitForEmailRequest{
		EmailAddress:    email,
		TimeoutSeconds:  timeoutSeconds,
		IssuedAfterUnix: input.GetIssuedAfterUnix(),
		SignalKind:      pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP,
	})
	if err != nil {
		data["error_message"] = err.Error()
		return OTPWaitOutput{Data: protoData(data)}, err
	}
	if resp == nil {
		err := fmt.Errorf("mailbox service returned empty email response")
		data["error_message"] = err.Error()
		return OTPWaitOutput{Data: protoData(data)}, err
	}
	message := resp.GetMessage()
	code := extractOTPFromEmailMessage(message)
	if resp.GetFound() && code != "" {
		if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{
			input.GetOtpParam():         code,
			input.GetSubmittedAtParam(): fmt.Sprintf("%d", time.Now().Unix()),
		}); err != nil {
			data["error_message"] = err.Error()
			return OTPWaitOutput{Data: protoData(data)}, err
		}
		data["found"] = true
		if message != nil {
			data["email_provider"] = message.GetProvider()
			data["message_id"] = message.GetId()
		}
		return OTPWaitOutput{Found: true, Source: otpWaitChannelEmail, Data: protoData(data)}, nil
	}
	err = fmt.Errorf("email otp not found")
	data["found"] = false
	data["error_message"] = err.Error()
	return OTPWaitOutput{Data: protoData(data)}, err
}

func extractOTPFromEmailMessage(message *pb.EmailInboxMessage) string {
	if message == nil {
		return ""
	}
	if signal := message.GetPrimarySignal(); signal.GetKind() == pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP && signal.GetCode() != "" {
		return normalizeOTP(signal.GetCode())
	}
	for _, signal := range message.GetSignals() {
		if signal.GetKind() == pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP && signal.GetCode() != "" {
			return normalizeOTP(signal.GetCode())
		}
	}
	combined := strings.Join([]string{
		message.GetSubject(),
		message.GetBodyPreview(),
		message.GetBodyText(),
	}, "\n")
	match := emailOTPPattern.FindStringSubmatch(combined)
	if len(match) < 3 {
		return ""
	}
	return normalizeOTP(match[2])
}
