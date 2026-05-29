package api

import (
	"context"
	"strings"

	"orchestrator/pb"
)

const activationGoPayPinSecretKey = "activation_gopay_pin:"

func (s *Server) requestGoPayPaymentSMSCode(ctx context.Context, input pb.GoPayActivityInput, reason string) error {
	if normalizeGoPayOTPChannel(input.GetOtpChannel()) != "sms" || strings.TrimSpace(input.GetSmsActivationId()) == "" {
		return nil
	}
	_, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{
		JobId:        input.GetJobId(),
		ActivationId: input.GetSmsActivationId(),
		Reason:       reason,
	})
	return err
}

func (s *Server) cancelGoPayPaymentAction(ctx context.Context, flowID string) {
	if strings.TrimSpace(flowID) == "" {
		return
	}
	_ = s.activities.GoPayPaymentCancelActivity(ctx, pb.GoPayPaymentCancelInput{FlowId: flowID})
}

func paymentFlowID(values ...string) string {
	return firstNonEmpty(values...)
}
