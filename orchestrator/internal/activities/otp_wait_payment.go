package activities

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
