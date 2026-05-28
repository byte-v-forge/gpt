package activities

import (
	"regexp"
	"strings"
)

var otpCodePattern = regexp.MustCompile(`[0-9]{4,8}`)

func (s *Server) paymentOtpTimeout() int32 {
	if s.otpTimeout <= 0 {
		return 180
	}
	return s.otpTimeout
}

func (s *Server) registrationOtpTimeout() int32 {
	if s.regOTPTimeout <= 0 {
		return 120
	}
	return s.regOTPTimeout
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
