package activities

import (
	"fmt"
	"strings"
)

func codexOAuthPhonePageFailureState(data map[string]any) string {
	text := strings.ToLower(fmt.Sprint(data))
	switch {
	case containsAny(text, "already linked to the maximum number of accounts", "used too many", "too many times", "maximum number", "max number", "limit exceeded", "too many attempts"):
		return "phone_reuse_exceeded"
	case containsAny(text, "try a different phone", "try another phone", "can't use this phone", "cannot use this phone", "unsupported phone", "invalid phone", "not valid", "rejected"):
		return "phone_rejected"
	case containsAny(text, "temporarily unavailable", "rate limit", "blocked"):
		return "phone_rejected"
	default:
		return ""
	}
}

func codexOAuthPhoneFailureDisposition(message string) (string, string) {
	text := strings.ToLower(message)
	switch {
	case containsAny(text, "phone_reuse_exceeded", "already linked to the maximum number of accounts", "used too many", "too many times", "maximum number", "max number", "limit exceeded"):
		return "phone_reuse_exceeded", codexOAuthLeaseExhausted
	case containsAny(text, "phone_rejected", "try a different phone", "try another phone", "can't use this phone", "cannot use this phone", "unsupported phone", "invalid phone", "rejected"):
		return "phone_rejected", codexOAuthLeaseFailed
	case containsAny(text, "phone_sms_timeout", "sms_error_code_timeout", "waitforcode", "otp not found", "empty code"):
		return "phone_sms_timeout", codexOAuthLeaseFailed
	case containsAny(text, "phone_expired"):
		return "phone_expired", codexOAuthLeaseExpired
	default:
		return "", ""
	}
}

func codexOAuthFailureLikelyUsedPhone(message string) bool {
	failureKind, _ := codexOAuthPhoneFailureDisposition(message)
	switch failureKind {
	case "phone_reuse_exceeded", "phone_rejected", "phone_sms_timeout":
		return true
	default:
		return false
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func codexOAuthFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maskPhone(e164, national string) string {
	value := strings.TrimSpace(e164)
	if value == "" {
		value = strings.TrimSpace(national)
	}
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
}
