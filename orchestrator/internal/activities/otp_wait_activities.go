package activities

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/otpwait"
	"orchestrator/pb"
)

var emailOTPPattern = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)

const (
	otpWaitChannelEmail   = otpwait.ChannelEmail
	otpWaitChannelPayment = otpwait.ChannelPayment
	otpWaitChannelSMS     = otpwait.ChannelSMS
)

func (s *Server) OTPWaitActivity(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	switch otpWaitInputChannel(input) {
	case otpWaitChannelEmail:
		return s.waitEmailOTP(ctx, input)
	case otpWaitChannelPayment:
		return s.waitPaymentWebhookOTP(ctx, input)
	case otpWaitChannelSMS:
		return s.waitSMSOTP(ctx, input)
	default:
		return OTPWaitOutput{}, fmt.Errorf("otp wait target missing")
	}
}

func otpWaitInputChannel(input OTPWaitInput) string {
	return otpwait.Channel(&input)
}

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

func (s *Server) waitPaymentWebhookOTP(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	target := input.GetPayment()
	if target == nil {
		return OTPWaitOutput{}, fmt.Errorf("gopay otp target missing")
	}
	if s.otpRelay == nil {
		return OTPWaitOutput{}, fmt.Errorf("gopay otp relay not configured")
	}
	purposeSegment := strings.TrimSpace(target.GetPurpose())
	if purposeSegment == "" {
		purposeSegment = "gopay"
	}
	purpose, err := goPayOTPQueueKey(target.GetSource(), purposeSegment)
	if err != nil {
		return OTPWaitOutput{}, err
	}
	timeoutSeconds := input.GetTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = s.paymentOtpTimeout()
	}

	type otpServiceResult struct {
		code   string
		source string
		err    error
	}
	otpCtx, cancelOTP := context.WithCancel(ctx)
	defer cancelOTP()

	otpCh := make(chan otpServiceResult, 1)
	go func() {
		entry, found, err := s.otpRelay.Wait(otpCtx, purpose, input.GetIssuedAfterUnix(), time.Duration(timeoutSeconds+10)*time.Second)
		if err != nil {
			otpCh <- otpServiceResult{err: fmt.Errorf("otp not received after %ds: %w", timeoutSeconds, err)}
			return
		}
		code := normalizeOTP(entry.OTP)
		if found && code != "" {
			source := strings.TrimSpace(entry.Source)
			if source == "" {
				source = "webhook"
			}
			otpCh <- otpServiceResult{code: code, source: source}
			return
		}
		otpCh <- otpServiceResult{err: fmt.Errorf("otp not received after %ds: otp not found", timeoutSeconds)}
	}()

	deadline := time.NewTimer(time.Duration(timeoutSeconds) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var heartbeatAt time.Time
	var lastErr error
	data := map[string]any{
		"channel":           otpWaitChannelPayment,
		"purpose":           purpose,
		"timeout_seconds":   timeoutSeconds,
		"issued_after_unix": input.GetIssuedAfterUnix(),
	}
	step := s.activityStep(ctx, input.GetJobId(), input.GetStepName(), false, true)
	waitMessage := goPayWebhookOTPWaitMessage(input.GetStepName())
	notReceivedMessage := goPayWebhookOTPNotReceivedMessage(input.GetStepName())

	for {
		select {
		case otpResult := <-otpCh:
			code := normalizeOTP(otpResult.code)
			if code != "" {
				if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{
					input.GetOtpParam():         code,
					input.GetSubmittedAtParam(): fmt.Sprintf("%d", time.Now().Unix()),
				}); err != nil {
					data["error_message"] = err.Error()
					return OTPWaitOutput{Data: protoData(data)}, err
				}
				data["found"] = true
				source := strings.TrimSpace(otpResult.source)
				if source == "" {
					source = "webhook"
				}
				data["source"] = source
				return OTPWaitOutput{Found: true, Source: source, Data: protoData(data)}, nil
			}
			if otpResult.err != nil {
				lastErr = otpResult.err
			}
			otpCh = nil
		case <-ticker.C:
			step.progressEvery(&heartbeatAt, waitMessage, data)
		case <-deadline.C:
			if lastErr != nil {
				err := fmt.Errorf("%s after %ds: %w", notReceivedMessage, timeoutSeconds, lastErr)
				data["error_message"] = err.Error()
				return OTPWaitOutput{Data: protoData(data)}, err
			}
			err := fmt.Errorf("%s after %ds", notReceivedMessage, timeoutSeconds)
			data["error_message"] = err.Error()
			return OTPWaitOutput{Data: protoData(data)}, err
		case <-ctx.Done():
			return OTPWaitOutput{Data: protoData(data)}, ctx.Err()
		}
	}
}

func goPayWebhookOTPWaitMessage(stepName string) string {
	switch strings.TrimSpace(stepName) {
	case stepGoPayAppSignup:
		return "waiting for gopay signup otp"
	case stepGoPayAppEnsurePINSetup:
		return "waiting for gopay create pin otp"
	case stepGoPayPayment:
		return "waiting for gopay payment otp"
	default:
		return "waiting for gopay otp"
	}
}

func goPayWebhookOTPNotReceivedMessage(stepName string) string {
	switch strings.TrimSpace(stepName) {
	case stepGoPayAppSignup:
		return "gopay signup otp not received"
	case stepGoPayAppEnsurePINSetup:
		return "gopay create pin otp not received"
	case stepGoPayPayment:
		return "gopay payment otp not received"
	default:
		return "gopay otp not received"
	}
}

func (s *Server) waitSMSOTP(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	target := input.GetSms()
	activationID := ""
	if target != nil {
		activationID = strings.TrimSpace(target.GetActivationId())
	}
	output := OTPWaitOutput{
		ActivationId: activationID,
		Source:       otpWaitChannelSMS,
	}
	data := map[string]any{
		"channel":       otpWaitChannelSMS,
		"activation_id": activationID,
	}
	stepName := input.GetStepName()
	if stepName == "" {
		stepName = stepGoPayAppChangePhoneSMSWait
	}
	step := s.activityStep(ctx, input.GetJobId(), stepName, false, true)
	_, err := step.run(func() (any, error) {
		if s.smsClient == nil {
			err := fmt.Errorf("code receiver client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		if activationID == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		timeoutSeconds := input.GetTimeoutSeconds()
		if timeoutSeconds <= 0 {
			timeoutSeconds = s.paymentOtpTimeout()
		}
		data["timeout_seconds"] = timeoutSeconds
		step.progress("waiting for sms otp", data)
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "waiting for sms otp", data)
		defer stopHeartbeat()
		code, err := s.waitSMSCode(ctx, activationID, timeoutSeconds)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if strings.TrimSpace(code) != "" {
			output.Found = true
			output.Code = normalizeOTP(code)
			if input.GetOtpParam() != "" {
				if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{
					input.GetOtpParam():         output.GetCode(),
					input.GetSubmittedAtParam(): fmt.Sprintf("%d", time.Now().Unix()),
				}); err != nil {
					data["error_message"] = err.Error()
					return data, err
				}
			}
			data["found"] = true
			return data, nil
		}
		message := "otp not found"
		output.ErrorMessage = message
		data["found"] = false
		data["error_message"] = message
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
