package activities

import (
	"context"
	"regexp"
	"strings"

	"orchestrator/internal/gptsettings"
)

var otpCodePattern = regexp.MustCompile(`[0-9]{4,8}`)

func (s *Server) paymentOtpTimeout(ctx context.Context) int32 {
	return gptsettings.Int32Value(s.goPayPluginValues(ctx), "otp_timeout_seconds", 0)
}

func (s *Server) registrationOtpTimeout(ctx context.Context) int32 {
	return s.privateFlowRegistrationOTPTimeout(ctx)
}

func normalizeOTP(value string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "")
	return strings.TrimSpace(replacer.Replace(value))
}

func extractOTP(value string) string {
	return otpCodePattern.FindString(normalizeOTP(value))
}

func normalizeTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}
