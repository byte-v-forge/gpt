package paymentsvc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/byte-v-forge/gpt/gopay/protocol"
)

func stringAt(value any, path ...string) string {
	return strings.TrimSpace(protocol.StringAt(value, path...))
}

func boolAt(value any, path ...string) bool {
	current := value
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current = obj[key]
	}
	switch typed := current.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func intAt(value any, path ...string) int64 {
	text := stringAt(value, path...)
	if text == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	return int64(parsed)
}

func normalizeDigits(value string) string {
	var out strings.Builder
	for _, ch := range value {
		if ch >= '0' && ch <= '9' {
			out.WriteRune(ch)
		}
	}
	return out.String()
}

func normalizeCountryCode(value string) string {
	return normalizeDigits(value)
}

func normalizeOTPChannel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sms", "otp_sms":
		return "sms"
	case "wa", "whatsapp", "otp_wa":
		return "wa"
	default:
		return ""
	}
}

func extractCheckoutSessionID(data map[string]any) string {
	for _, key := range []string{"checkout_session_id", "session_id", "id"} {
		value := stringAt(data, key)
		if strings.HasPrefix(value, "cs_") {
			return value
		}
	}
	for _, key := range []string{"url", "stripe_hosted_url", "checkout_url"} {
		if match := regexp.MustCompile(`cs_(?:live|test)_[A-Za-z0-9]+`).FindString(stringAt(data, key)); match != "" {
			return match
		}
	}
	return ""
}

func checkoutURLFromResponse(data map[string]any, csID string) string {
	for _, key := range []string{"url", "stripe_hosted_url", "checkout_url"} {
		if value := stringAt(data, key); value != "" {
			return value
		}
	}
	if csID != "" {
		return "https://checkout.stripe.com/c/pay/" + csID
	}
	return ""
}

func extractReferenceFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if parsed, err := url.Parse(text); err == nil {
		query := parsed.Query()
		for _, key := range []string{"reference", "reference_id", "referenceId", "tref"} {
			for _, item := range query[key] {
				if value := strings.TrimSpace(item); value != "" {
					return value
				}
			}
		}
	}
	match := regexp.MustCompile(`(?:[?&#]|^)(?:reference|reference_id|referenceId)=([A-Za-z0-9-]+)`).FindStringSubmatch(text)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func extractMidtransChargeReference(data any) string {
	var walk func(any, string) string
	walk = func(value any, path string) string {
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				if found := walk(item, path+"."+key); found != "" {
					return found
				}
			}
		case []any:
			for _, item := range typed {
				if found := walk(item, path); found != "" {
					return found
				}
			}
		case string:
			if reference := extractReferenceFromText(typed); reference != "" {
				return reference
			}
			if strings.Contains(strings.ToLower(path), "reference") && regexp.MustCompile(`^[A-Za-z0-9-]{6,}$`).MatchString(strings.TrimSpace(typed)) {
				return strings.TrimSpace(typed)
			}
		}
		return ""
	}
	return walk(data, "")
}

func extractMidtransURL(data map[string]any, names ...string) string {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[strings.ToLower(name)] = true
		if value := stringAt(data, name); value != "" {
			return value
		}
	}
	items, _ := data["actions"].([]any)
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if wanted[strings.ToLower(stringAt(obj, "name"))] {
			if value := stringAt(obj, "url"); value != "" {
				return value
			}
		}
	}
	return ""
}

func midtransChargeURLs(data map[string]any) map[string]string {
	return map[string]string{
		"deeplink_url":            extractMidtransURL(data, "deeplink_url", "deeplink"),
		"qr_code_url":             extractMidtransURL(data, "qr_code_url", "qr_code", "qrcode"),
		"finish_redirect_url":     extractMidtransURL(data, "finish_redirect_url"),
		"finish_200_redirect_url": extractMidtransURL(data, "finish_200_redirect_url"),
	}
}

func decodeJWTPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func jsonExcerpt(value any, limit int) string {
	raw, err := protocol.CompactJSON(value)
	if err != nil {
		return protocol.Snippet(protocol.RedactText(fmt.Sprint(value)), limit)
	}
	return protocol.Snippet(protocol.RedactText(string(raw)), limit)
}

func regexpMust(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

func mapValues(key, value string) url.Values {
	values := url.Values{}
	values.Set(key, value)
	return values
}
