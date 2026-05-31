package activities

import (
	"context"
	"regexp"
	"strings"

	"orchestrator/internal/channelotpwait"
)

var otpCodePattern = regexp.MustCompile(`[0-9]{4,8}`)

func (s *Server) registrationOtpTimeout(ctx context.Context) int32 {
	return s.privateFlowRegistrationOTPTimeout(ctx)
}

func extractOTP(value string) string {
	return otpCodePattern.FindString(channelotpwait.NormalizeCode(value))
}

func normalizeTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}
