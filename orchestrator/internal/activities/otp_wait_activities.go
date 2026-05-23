package activities

import (
	"context"
	"fmt"

	"orchestrator/internal/otpwait"
)

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
