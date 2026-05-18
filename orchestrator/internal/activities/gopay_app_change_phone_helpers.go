package activities

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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
		s.cancelSMSActivationAsync(activationID, reason)
	}
	*failures++
	maxFailures := s.changePhoneMaxFailureCount()
	log.Printf("[gopay-app] Change phone retryable failure %d/%d: %s", *failures, maxFailures, reason)
	if *failures >= maxFailures {
		return fmt.Errorf("failed to change phone after %d consecutive failures: %s", maxFailures, reason)
	}
	return nil
}

func smsNoNumbers(message string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(message)), "NO_NUMBERS")
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
	resp, err := s.smsClient.FinishActivation(ctx, &pb.FinishActivationRequest{ActivationId: activationID})
	if err != nil {
		log.Printf("[gopay-app] FinishActivation failed: %v", err)
		return
	}
	if !resp.GetSuccess() {
		log.Printf("[gopay-app] FinishActivation failed: %s", resp.GetErrorMessage())
	}
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
		resp, err := s.smsClient.CancelActivation(ctx, &pb.CancelActivationRequest{ActivationId: activationID})
		if err != nil {
			return fmt.Errorf("CancelActivation: %w", err)
		}
		if smsCancelSettled(resp) {
			if resp != nil && !resp.GetSuccess() {
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

func (s *Server) cancelSMSActivationAsync(activationID string, reason string) {
	if s.smsClient == nil || activationID == "" {
		return
	}
	go func() {
		timeout := s.changePhoneSMSCancelWaitTimeout() + 5*time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if strings.TrimSpace(reason) != "" {
			log.Printf("[gopay-app] async CancelActivation for %s: %s", activationID, reason)
		}
		if err := s.cancelSMSActivationBeforeRotation(ctx, activationID); err != nil {
			log.Printf("[gopay-app] async CancelActivation failed for %s: %v", activationID, err)
		}
	}()
}

func smsCancelSettled(resp *pb.ProviderActionResponse) bool {
	if resp == nil {
		return false
	}
	if resp.GetSuccess() {
		return true
	}
	message := strings.ToUpper(smsCancelResponseText(resp))
	return strings.Contains(message, "NO_ACTIVATION") || strings.Contains(message, "STATUS_CANCEL")
}

func smsEarlyCancelDenied(message string) bool {
	return strings.Contains(strings.ToUpper(message), "EARLY_CANCEL_DENIED")
}

func smsCancelResponseText(resp *pb.ProviderActionResponse) string {
	if resp == nil {
		return "empty response"
	}
	parts := []string{}
	if resp.GetErrorMessage() != "" {
		parts = append(parts, resp.GetErrorMessage())
	}
	if resp.GetRawResponse() != "" {
		parts = append(parts, resp.GetRawResponse())
	}
	if len(parts) == 0 {
		return "unknown error"
	}
	return strings.Join(parts, ": ")
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
