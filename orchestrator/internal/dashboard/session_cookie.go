package dashboard

import (
	"fmt"
	"orchestrator/internal/chatgptauth"
	"sort"
	"strconv"
	"strings"
)

const (
	nextAuthSessionCookieName         = "__Secure-next-auth.session-token"
	nextAuthSessionCookieFallbackName = "next-auth.session-token"
	nextAuthSessionCookieChunkSize    = 4096 - 163
)

func extractSessionToken(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if sessionToken := chatgptauth.ExtractSessionToken(text); sessionToken != "" {
		return sessionToken
	}
	exact := ""
	chunks := map[int]string{}
	for _, part := range strings.Split(text, ";") {
		name, value, ok := parseSessionCookiePart(part)
		if !ok {
			continue
		}
		if name == nextAuthSessionCookieName || name == nextAuthSessionCookieFallbackName {
			exact = value
			continue
		}
		if index, ok := sessionCookieChunkIndex(name); ok {
			chunks[index] = value
		}
	}
	if exact != "" {
		return exact
	}
	if len(chunks) == 0 {
		return ""
	}
	indexes := make([]int, 0, len(chunks))
	for index := range chunks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	var b strings.Builder
	for _, index := range indexes {
		b.WriteString(chunks[index])
	}
	return b.String()
}

func chatGPTSessionCookieHeader(sessionToken string) string {
	token := extractSessionToken(sessionToken)
	if token == "" {
		token = strings.TrimSpace(sessionToken)
	}
	if token == "" {
		return ""
	}
	if strings.Contains(token, "=") {
		parts := make([]string, 0, 2)
		for _, part := range strings.Split(token, ";") {
			name, value, ok := parseSessionCookiePart(part)
			if ok {
				parts = append(parts, name+"="+value)
			}
		}
		if len(parts) > 0 {
			sort.SliceStable(parts, func(i, j int) bool {
				return sessionCookieSortKey(parts[i]) < sessionCookieSortKey(parts[j])
			})
			return strings.Join(parts, "; ")
		}
	}
	if len(token) <= nextAuthSessionCookieChunkSize {
		return nextAuthSessionCookieName + "=" + token
	}
	parts := make([]string, 0, (len(token)+nextAuthSessionCookieChunkSize-1)/nextAuthSessionCookieChunkSize)
	for index, offset := 0, 0; offset < len(token); index, offset = index+1, offset+nextAuthSessionCookieChunkSize {
		end := offset + nextAuthSessionCookieChunkSize
		if end > len(token) {
			end = len(token)
		}
		parts = append(parts, fmt.Sprintf("%s.%d=%s", nextAuthSessionCookieName, index, token[offset:end]))
	}
	return strings.Join(parts, "; ")
}

func parseSessionCookiePart(raw string) (string, string, bool) {
	part := strings.Trim(raw, " \t\r\n'\"\\")
	for _, base := range []string{nextAuthSessionCookieName, nextAuthSessionCookieFallbackName} {
		if idx := strings.Index(part, base); idx >= 0 {
			part = part[idx:]
			break
		}
	}
	if !strings.Contains(part, "=") {
		return "", "", false
	}
	name, value, _ := strings.Cut(part, "=")
	name = strings.TrimSpace(name)
	value = strings.Trim(value, " \t\r\n'\"\\")
	for i, r := range value {
		if r == '\'' || r == '"' || r == '\\' || r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			value = value[:i]
			break
		}
	}
	if !isSessionCookieName(name) || value == "" {
		return "", "", false
	}
	return name, value, true
}

func isSessionCookieName(name string) bool {
	if name == nextAuthSessionCookieName || name == nextAuthSessionCookieFallbackName {
		return true
	}
	_, ok := sessionCookieChunkIndex(name)
	return ok
}

func sessionCookieChunkIndex(name string) (int, bool) {
	for _, base := range []string{nextAuthSessionCookieName, nextAuthSessionCookieFallbackName} {
		prefix := base + "."
		if strings.HasPrefix(name, prefix) {
			index, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
			return index, err == nil
		}
	}
	return 0, false
}

func sessionCookieSortKey(part string) int {
	name, _, _ := strings.Cut(part, "=")
	if index, ok := sessionCookieChunkIndex(name); ok {
		return index
	}
	return -1
}
