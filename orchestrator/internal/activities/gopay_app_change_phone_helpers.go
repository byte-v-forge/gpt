package activities

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	pb "orchestrator/pb"
)

func (s *Server) changePhoneMaxFailureCount() int {
	if s.changePhoneMaxFailures <= 0 {
		return defaultChangePhoneMaxFailures
	}
	return s.changePhoneMaxFailures
}

func (s *Server) changePhoneOTPRetryCount() int {
	if s.changePhoneOTPRetryAttempts < 0 {
		return defaultChangePhoneOTPRetryAttempts
	}
	return s.changePhoneOTPRetryAttempts
}

func (s *Server) changePhoneGetNumberRetryInterval() time.Duration {
	if s.changePhoneGetNumberRetryDelay < 0 {
		return defaultChangePhoneGetNumberRetryDelay
	}
	return s.changePhoneGetNumberRetryDelay
}

func (s *Server) changePhoneSMSCancelWaitTimeout() time.Duration {
	if s.changePhoneSMSCancelTimeout <= 0 {
		return defaultChangePhoneSMSCancelTimeout
	}
	return s.changePhoneSMSCancelTimeout
}

func (s *Server) changePhoneSMSCancelRetryDelay() time.Duration {
	if s.changePhoneSMSCancelRetryInterval <= 0 {
		return defaultChangePhoneSMSCancelRetryInterval
	}
	return s.changePhoneSMSCancelRetryInterval
}

func (s *Server) recordChangePhoneFailure(ctx context.Context, activationID string, failures *int, reason string) error {
	if activationID != "" {
		if err := s.cancelSMSActivationForFailure(ctx, activationID, reason); err != nil {
			return err
		}
	}
	*failures++
	maxFailures := s.changePhoneMaxFailureCount()
	log.Printf("[gopay-app] Change phone retryable failure %d/%d: %s", *failures, maxFailures, reason)
	if *failures >= maxFailures {
		return fmt.Errorf("failed to change phone after %d consecutive failures: %s", maxFailures, reason)
	}
	return nil
}

func (s *Server) changePhoneErrorWithCancel(ctx context.Context, activationID string, reason string, err error, data map[string]any) error {
	if err == nil {
		err = fmt.Errorf("%s", reason)
	}
	if cancelErr := s.cancelSMSActivationForFailure(ctx, activationID, reason); cancelErr != nil {
		err = fmt.Errorf("%w; cleanup: %v", err, cancelErr)
	}
	data["error_message"] = err.Error()
	return err
}

func smsNoNumbers(message string) bool {
	upper := strings.ToUpper(strings.TrimSpace(message))
	return strings.Contains(upper, "NO_NUMBERS") || strings.Contains(upper, "NO_NUMBER_AVAILABLE")
}

func changePhoneStartRetryableError(message string) bool {
	switch strings.ToUpper(strings.TrimSpace(message)) {
	case "PHONE_REGISTERED", "PHONE_EXHAUSTED":
		return true
	default:
		return false
	}
}

func (s *Server) recordCompletedChangePhoneFailure(ctx context.Context, activationID string, failures *int, reason string) error {
	if activationID != "" {
		s.finishSMSActivation(ctx, activationID)
	}
	*failures++
	maxFailures := s.changePhoneMaxFailureCount()
	log.Printf("[gopay-app] Change phone retryable failure %d/%d: %s", *failures, maxFailures, reason)
	if *failures >= maxFailures {
		return fmt.Errorf("failed to change phone after %d consecutive failures: %s", maxFailures, reason)
	}
	return nil
}

func (s *Server) finishSMSActivation(ctx context.Context, activationID string) {
	if s.smsClient == nil || activationID == "" {
		return
	}
	if err := s.completeSMSActivation(ctx, activationID, ""); err != nil {
		log.Printf("[gopay-app] CompleteActivation failed: %v", err)
	}
}

func (s *Server) cancelSMSActivationForFailure(ctx context.Context, activationID string, reason string) error {
	if strings.TrimSpace(activationID) == "" {
		return nil
	}
	if strings.TrimSpace(reason) != "" {
		log.Printf("[gopay-app] CancelActivation for %s: %s", activationID, reason)
	}
	if err := s.cancelSMSActivationBeforeRotation(ctx, activationID); err != nil {
		return fmt.Errorf("CancelActivation cleanup failed: %w", err)
	}
	return nil
}

func (s *Server) cancelSMSActivationBeforeRotation(ctx context.Context, activationID string) error {
	if s.smsClient == nil {
		return fmt.Errorf("code receiver client not configured")
	}
	if activationID == "" {
		return fmt.Errorf("activation id missing")
	}

	deadline := time.Now().Add(s.changePhoneSMSCancelWaitTimeout())
	for {
		resp, err := s.smsClient.CancelActivation(ctx, &smsv1.CancelActivationRequest{ActivationId: activationID, Reason: "change phone rotation"})
		if err != nil {
			return fmt.Errorf("CancelActivation: %w", err)
		}
		if smsCancelSettled(resp) {
			if resp != nil && resp.GetError() != nil {
				log.Printf("[gopay-app] CancelActivation settled without ACCESS_CANCEL: %s", smsCancelResponseText(resp))
			}
			return nil
		}

		message := smsCancelResponseText(resp)
		if !smsEarlyCancelDenied(message) {
			return fmt.Errorf("CancelActivation: %s", message)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("CancelActivation: %s", message)
		}
		delay := minDuration(s.changePhoneSMSCancelRetryDelay(), remaining)
		log.Printf("[gopay-app] CancelActivation denied too early; retrying in %s", delay)
		if err := sleepContext(ctx, delay); err != nil {
			return fmt.Errorf("waiting to retry CancelActivation: %w", err)
		}
	}
}

func smsCancelSettled(resp *smsv1.CancelActivationResponse) bool {
	if resp == nil {
		return false
	}
	if resp.GetError() == nil {
		return true
	}
	switch resp.GetError().GetCode() {
	case smsv1.SmsErrorCode_SMS_ERROR_CODE_ACTIVATION_NOT_FOUND,
		smsv1.SmsErrorCode_SMS_ERROR_CODE_ACTIVATION_ALREADY_FINALIZED,
		smsv1.SmsErrorCode_SMS_ERROR_CODE_ACTIVATION_EXPIRED:
		return true
	default:
		return false
	}
}

func smsEarlyCancelDenied(message string) bool {
	upper := strings.ToUpper(message)
	return strings.Contains(upper, "EARLY_CANCEL_DENIED") || strings.Contains(upper, "CANCEL_NOT_ALLOWED")
}

func smsCancelResponseText(resp *smsv1.CancelActivationResponse) string {
	if resp == nil {
		return "empty response"
	}
	if resp.GetError() != nil {
		return smsErrorText(resp.GetError())
	}
	return "unknown error"
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func authOtpIssuedAfterUnix(resp *pb.StatusResponse, fallback int64) int64 {
	if resp == nil {
		return fallback
	}
	var issuedAfter int64
	switch strings.TrimSpace(resp.GetStage()) {
	case "login_otp_pending":
		issuedAfter = resp.GetLoginOtpSentAtUnix()
	case "signup_otp_pending":
		issuedAfter = resp.GetSignupOtpSentAtUnix()
	case "signup_pin_otp_pending":
		issuedAfter = resp.GetSignupPinOtpSentAtUnix()
	}
	if issuedAfter > 0 {
		return issuedAfter
	}
	return fallback
}
