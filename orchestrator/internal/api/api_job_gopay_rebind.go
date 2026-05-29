package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

const (
	goPayLocalSource               = "local"
	goPayPaymentRebindPinSecretKey = "gopay_payment_rebind_pin:"
)

func (s *Server) finishGoPayChangePhoneSMS(ctx context.Context, jobID, activationID, reason string) error {
	if strings.TrimSpace(activationID) == "" {
		return fmt.Errorf("change phone activation id is missing")
	}
	_, err := s.activities.GoPayAppSMSFinishActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: activationID, Reason: reason})
	return err
}

func normalizeGoPayOTPChannel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "wa", "whatsapp", "otp_wa":
		return "wa"
	case "sms", "otp_sms":
		return "sms"
	default:
		return ""
	}
}

func isOTPWaitNotReceivedError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	return strings.Contains(normalized, "otp not received") || strings.Contains(normalized, "otp not found") || strings.Contains(normalized, "waitcode")
}
