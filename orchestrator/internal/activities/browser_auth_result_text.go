package activities

import (
	"fmt"
	"strconv"
	"strings"
)

func sanitizeBrowserAuthURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if before, _, ok := strings.Cut(raw, "?"); ok {
		raw = before
	}
	if before, _, ok := strings.Cut(raw, "#"); ok {
		raw = before
	}
	return raw
}

func browserAuthTextHints(text string) []string {
	keywords := []string{
		"Sign up", "Log in", "Sign in", "Create account", "Continue", "Next",
		"Open profile menu", "New chat", "Settings", "Log out", "Email",
	}
	seen := map[string]bool{}
	hints := make([]string, 0, 8)
	for _, rawLine := range strings.Split(text, "\n") {
		line := compactBrowserAuthText(rawLine, 80)
		if line == "" || seen[line] {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				seen[line] = true
				hints = append(hints, line)
				break
			}
		}
		if len(hints) >= 8 {
			break
		}
	}
	return hints
}

func stringMapValue(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func int64MapValue(data map[string]any, key string) int64 {
	if data == nil {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return n
	default:
		n, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(typed)), 10, 64)
		return n
	}
}

func unixSecondsFromMillis(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value / 1000
}
