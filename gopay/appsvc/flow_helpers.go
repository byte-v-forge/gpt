package appsvc

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/byte-v-forge/gpt/gopay/protocol"
)

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := anyString(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := anyString(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func methodsFrom(value any) []string {
	if methods := stringSlice(value); len(methods) > 0 {
		return methods
	}
	if obj, ok := jsonObject(value); ok {
		for key, item := range obj {
			if normalizeJSONKey(key) == "methods" {
				if methods := stringSlice(item); len(methods) > 0 {
					return methods
				}
			}
		}
		for _, item := range obj {
			if methods := methodsFrom(item); len(methods) > 0 {
				return methods
			}
		}
		return nil
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if methods := methodsFrom(item); len(methods) > 0 {
				return methods
			}
		}
	}
	return nil
}

func verificationIDFrom(value any) string {
	if text := stringForAnyKey(value, "verification_id", "verificationId"); text != "" {
		return text
	}
	return verificationScopedID(value)
}

func stringForAnyKey(value any, keys ...string) string {
	wanted := map[string]struct{}{}
	for _, key := range keys {
		wanted[normalizeJSONKey(key)] = struct{}{}
	}
	var walk func(any) string
	walk = func(current any) string {
		if obj, ok := jsonObject(current); ok {
			for key, item := range obj {
				if _, ok := wanted[normalizeJSONKey(key)]; ok {
					if text := anyString(item); text != "" {
						return text
					}
				}
			}
			for _, item := range obj {
				if text := walk(item); text != "" {
					return text
				}
			}
			return ""
		}
		switch typed := current.(type) {
		case []any:
			for _, item := range typed {
				if text := walk(item); text != "" {
					return text
				}
			}
		}
		return ""
	}
	return walk(value)
}

func jsonObject(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, typed != nil
	case protocol.JSONMap:
		return map[string]any(typed), typed != nil
	default:
		return nil, false
	}
}

func normalizeJSONKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func verificationScopedID(value any) string {
	var walk func(any, bool) string
	walk = func(current any, inVerificationScope bool) string {
		if obj, ok := jsonObject(current); ok {
			for key, item := range obj {
				normalized := normalizeJSONKey(key)
				nextScope := inVerificationScope || strings.Contains(normalized, "verification")
				if nextScope && normalized == "id" {
					if text := anyString(item); text != "" {
						return text
					}
				}
				if text := walk(item, nextScope); text != "" {
					return text
				}
			}
			return ""
		}
		switch typed := current.(type) {
		case []any:
			for _, item := range typed {
				if text := walk(item, inVerificationScope); text != "" {
					return text
				}
			}
		}
		return ""
	}
	return walk(value, false)
}

func responseShape(resp *protocol.Response) map[string]any {
	if resp == nil {
		return map[string]any{"status": 0}
	}
	payloadKeys := sortedKeys(resp.Payload)
	data := resp.Data()
	return map[string]any{
		"status":        resp.StatusCode,
		"payload_keys":  payloadKeys,
		"data_keys":     sortedKeys(data),
		"success":       resp.Payload["success"],
		"methods_count": len(methodsFrom(data)),
	}
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func retryableGoPayTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "eof") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "timeout")
}

func loginMethodsRateLimitedError(attempts, proxyCount int) string {
	return "GOPAY_PROXY_POOL exhausted before login methods succeeded"
}

func loginMethodsBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	if attempt > 5 {
		attempt = 5
	}
	return time.Duration(attempt) * time.Second
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func otpMethodFromChannel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sms", "otp_sms":
		return "otp_sms"
	case "wa", "whatsapp", "otp_wa":
		return "otp_wa"
	default:
		return ""
	}
}

func chooseOTPMethod(methods []string, preferred, defaultMethod string) string {
	explicit := otpMethodFromChannel(preferred)
	if strings.TrimSpace(preferred) != "" && explicit == "" {
		return ""
	}
	if explicit != "" {
		if len(methods) == 0 || contains(methods, explicit) {
			return explicit
		}
		return ""
	}
	defaultMethod = firstNonEmpty(otpMethodFromChannel(defaultMethod), "otp_sms")
	fallbacks := []string{defaultMethod, "otp_sms", "otp_wa"}
	if defaultMethod == "otp_wa" {
		fallbacks = []string{defaultMethod, "otp_wa", "otp_sms"}
	}
	for _, method := range fallbacks {
		if method != "" && (len(methods) == 0 || contains(methods, method)) {
			return method
		}
	}
	return defaultMethod
}

func firstAccountID(value any) string {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return ""
	}
	return protocol.StringAt(first, "account_id")
}

func (s *Server) persistLoginReady(state stateMap, tokenData map[string]any, phone string) {
	s.storeTokenResponse(state, tokenData, false)
	state["phone"] = phone
	state["stage"] = "ready"
	state["ready_at"] = time.Now().Unix()
	delete(state, "last_error")
	deleteKeys(state, loginStateKeys...)
}

func (s *Server) persistLoginOTP(state stateMap, phone, countryCode, verificationID, method, otpToken, twoFAToken string) {
	now := time.Now().Unix()
	state["_login_phone"] = phone
	state["_login_country_code"] = countryCode
	state["_login_verification_id"] = verificationID
	state["_login_verification_method"] = method
	state["_login_otp_token"] = otpToken
	state["_login_2fa_token"] = twoFAToken
	state["_login_otp_sent_at"] = now
	state["_login_otp_expires_at"] = now + int64(s.cfg.OTPTimeout.Seconds())
	state["stage"] = "login_otp_pending"
	delete(state, "last_error")
}

func (s *Server) persistSignupOTP(state stateMap, verificationID, method, otpToken string) {
	now := time.Now().Unix()
	state["_signup_verification_id"] = verificationID
	state["_signup_verification_method"] = method
	state["_signup_otp_token"] = otpToken
	state["_signup_otp_sent_at"] = now
	state["_signup_otp_expires_at"] = now + int64(s.cfg.OTPTimeout.Seconds())
	state["stage"] = "signup_otp_pending"
	delete(state, "last_error")
}

func (s *Server) clearLoginState(state stateMap, reason string) {
	deleteKeys(state, loginStateKeys...)
	if stage := stateString(state, "stage"); stage == "login" || stage == "login_otp_pending" {
		if stateInt(state, "deactivated_at") > 0 {
			state["stage"] = "deactivated"
		} else {
			state["stage"] = "idle"
		}
	}
	if reason != "" {
		state["last_error"] = reason
	}
}

func (s *Server) clearSignupState(state stateMap, reason string) {
	deleteKeys(state, signupAccountStateKeys...)
	deleteKeys(state, signupOTPStateKeys...)
	deleteKeys(state, signupPINStateKeys...)
	stage := stateString(state, "stage")
	if stage == "signup" || stage == "signup_otp_pending" || stage == "signup_pin_required" || stage == "signup_pin_otp_pending" {
		if stateInt(state, "deactivated_at") > 0 && stateString(state, "token") == "" {
			state["stage"] = "deactivated"
		} else {
			state["stage"] = "idle"
		}
	}
	if reason != "" {
		state["last_error"] = reason
	}
}

func (s *Server) expireLoginIfNeeded(state stateMap) bool {
	if stateString(state, "stage") != "login_otp_pending" {
		return false
	}
	now := time.Now().Unix()
	expiresAt := stateInt(state, "_login_otp_expires_at")
	if expiresAt > 0 && now < expiresAt {
		return false
	}
	if expiresAt == 0 {
		sentAt := stateInt(state, "_login_otp_sent_at")
		if sentAt > 0 && now < sentAt+int64(s.cfg.OTPTimeout.Seconds()) {
			return false
		}
	}
	s.clearLoginState(state, "LOGIN_OTP_TIMEOUT")
	return true
}

func (s *Server) expireSignupIfNeeded(state stateMap) bool {
	stage := stateString(state, "stage")
	now := time.Now().Unix()
	if stage == "signup_otp_pending" && pendingExpired(now, stateInt(state, "_signup_otp_sent_at"), stateInt(state, "_signup_otp_expires_at"), s.cfg.OTPTimeout) {
		deleteKeys(state, signupOTPStateKeys...)
		state["stage"] = "idle"
		state["last_error"] = "SIGNUP_OTP_TIMEOUT"
		return true
	}
	if stage == "signup_pin_otp_pending" && pendingExpired(now, stateInt(state, "_signup_pin_otp_sent_at"), stateInt(state, "_signup_pin_otp_expires_at"), s.cfg.OTPTimeout) {
		deleteKeys(state, signupPINStateKeys...)
		if stateString(state, "token") != "" {
			state["stage"] = "signup_pin_required"
		} else {
			state["stage"] = "idle"
		}
		state["last_error"] = "SIGNUP_PIN_OTP_TIMEOUT"
		return true
	}
	return false
}

func pendingExpired(now, sentAt, expiresAt int64, timeout time.Duration) bool {
	if expiresAt > 0 {
		return now >= expiresAt
	}
	if sentAt > 0 {
		return now >= sentAt+int64(timeout.Seconds())
	}
	return true
}

func walletBalance(value any) (int64, string) {
	items, ok := value.([]any)
	if !ok {
		if obj, ok := value.(map[string]any); ok {
			items = []any{obj}
		}
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok || anyString(obj["type"]) != "GOPAY_WALLET" {
			continue
		}
		balance := nestedMap(obj["balance"])
		amount := parseBalanceAmount(balance["value"])
		if amount < 0 {
			amount = parseBalanceAmount(balance["display_value"])
		}
		return amount, firstNonEmpty(anyString(balance["currency"]), anyString(obj["currency"]))
	}
	return -1, ""
}

func parseBalanceAmount(value any) int64 {
	if value == nil {
		return -1
	}
	text := anyString(value)
	if text == "" {
		return -1
	}
	var digits strings.Builder
	for _, ch := range text {
		if (ch >= '0' && ch <= '9') || ch == '-' {
			digits.WriteRune(ch)
		}
	}
	raw := digits.String()
	if raw == "" || raw == "-" {
		return -1
	}
	var out int64
	if _, err := fmt.Sscan(raw, &out); err != nil {
		return -1
	}
	return out
}
