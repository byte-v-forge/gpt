package activities

import (
	"sort"
	"strconv"
	"strings"
)

func browserCookieMaps(data map[string]any) []map[string]string {
	if data == nil {
		return nil
	}
	rawCookies, ok := data["cookies"].([]any)
	if !ok {
		return nil
	}
	cookies := make([]map[string]string, 0, len(rawCookies))
	for _, raw := range rawCookies {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cookies = append(cookies, map[string]string{
			"name":   stringMapValue(item, "name"),
			"value":  stringMapValue(item, "value"),
			"domain": stringMapValue(item, "domain"),
		})
	}
	return cookies
}

func extractBrowserSessionToken(cookies []map[string]string) string {
	type cookiePart struct {
		name  string
		value string
	}
	parts := make([]cookiePart, 0)
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie["name"])
		value := strings.TrimSpace(cookie["value"])
		if value != "" && isBrowserSessionCookieName(name) {
			parts = append(parts, cookiePart{name: name, value: value})
		}
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Slice(parts, func(i, j int) bool {
		baseI, suffixI := browserSessionCookieOrder(parts[i].name)
		baseJ, suffixJ := browserSessionCookieOrder(parts[j].name)
		if baseI != baseJ {
			return baseI < baseJ
		}
		if suffixI != suffixJ {
			return suffixI < suffixJ
		}
		return parts[i].name < parts[j].name
	})
	if _, suffix := browserSessionCookieOrder(parts[0].name); suffix < 0 {
		return parts[0].value
	}
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(part.value)
	}
	return builder.String()
}

func isBrowserSessionCookieName(name string) bool {
	base, _ := browserSessionCookieOrder(name)
	return base < 99
}

func browserSessionCookieOrder(name string) (int, int) {
	bases := []string{
		"__Secure-next-auth.session-token",
		"next-auth.session-token",
		"__Secure-authjs.session-token",
		"authjs.session-token",
	}
	for baseOrder, base := range bases {
		if name == base {
			return baseOrder, -1
		}
		prefix := base + "."
		if strings.HasPrefix(name, prefix) {
			if suffix, err := strconv.Atoi(strings.TrimPrefix(name, prefix)); err == nil {
				return baseOrder, suffix
			}
		}
	}
	return 99, 0
}

func extractCookieValue(cookies []map[string]string, names ...string) string {
	for _, cookie := range cookies {
		for _, name := range names {
			if cookie["name"] == name {
				return cookie["value"]
			}
		}
	}
	return ""
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
