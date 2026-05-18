package activities

import (
	"strings"
)

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

func normalizeTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}

func shouldSkipPlusTrialProbe(account AccountRef) bool {
	return account.GetPlusActive() || normalizeTier(account.GetTier()) == "plus"
}

func skippedPlusTrialProbeData(account AccountRef) map[string]any {
	reason := "plus_active"
	if normalizeTier(account.GetTier()) == "plus" {
		reason = "tier_plus"
	}
	return map[string]any{
		"skipped":     true,
		"reason":      reason,
		"tier":        account.GetTier(),
		"plus_active": account.GetPlusActive(),
	}
}
