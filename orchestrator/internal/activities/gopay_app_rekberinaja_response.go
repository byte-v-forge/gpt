package activities

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func rekberinajaAPIBaseURL(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("absolute URL is required")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if index := strings.Index(parsed.Path, "/api/"); index >= 0 {
		parsed.Path = parsed.Path[:index+len("/api")]
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	return "", fmt.Errorf("endpoint path must contain /api/")
}

func rekberinajaJoinURL(base string, suffix string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

func rekberinajaURLWithCacheBuster(endpoint string, now time.Time) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("absolute URL is required")
	}
	query := parsed.Query()
	if query.Get("_") == "" {
		query.Set("_", strconv.FormatInt(now.UnixMilli(), 10))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func rekberinajaDataObject(response map[string]any) map[string]any {
	if response == nil {
		return map[string]any{}
	}
	data, ok := response["data"].(map[string]any)
	if !ok || data == nil {
		return map[string]any{}
	}
	return data
}

func rekberinajaStringAt(response map[string]any, path ...string) string {
	var current any = response
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[key]
	}
	return rekberinajaString(current)
}

func rekberinajaString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func rekberinajaInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func rekberinajaBoolFalse(response map[string]any, key string) bool {
	if response == nil {
		return false
	}
	value, ok := response[key].(bool)
	return ok && !value
}

func rekberinajaResponseMessage(response map[string]any, raw string) string {
	for _, key := range []string{"error_message", "message", "error"} {
		if value := strings.TrimSpace(fmt.Sprint(response[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return limitStepText(strings.TrimSpace(raw), 500)
}

func limitStepText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...<truncated>"
}
