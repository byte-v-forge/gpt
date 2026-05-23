package paymentsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	sessionCookieName         = "__Secure-next-auth.session-token"
	sessionCookieFallbackName = "next-auth.session-token"
	sessionCookieChunkSize    = 4096 - 163
)

func (s *Server) newChatGPTSession(ctx context.Context, cred credential, fingerprints ...browserFingerprint) (*httpSession, error) {
	if cred.empty() {
		return nil, fmt.Errorf("auth missing: need session_token or access_token")
	}
	fingerprint := randomPaymentBrowserFingerprint(s.cfg.BrowserLocale)
	if len(fingerprints) > 0 {
		fingerprint = fingerprints[0].withFallback(s.cfg.BrowserLocale)
	}
	session, err := newHTTPSession(s.cfg.CheckoutProxyURL, fingerprint)
	if err != nil {
		return nil, err
	}
	deviceID := uuid.NewString()
	setChatGPTBrowserHeaders(session, deviceID, fingerprint)
	session.headers.Set("Content-Type", "application/json")
	if cred.accessToken != "" {
		session.headers.Set("Authorization", "Bearer "+cred.accessToken)
	}
	if cookie := chatGPTCookieHeader(cred.sessionToken, deviceID); cookie != "" {
		session.headers.Set("Cookie", cookie)
	}
	if cred.sessionToken != "" {
		resp, err := session.request(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", requestOptions{
			headers: http.Header{
				"Accept":  []string{"application/json"},
				"Referer": []string{"https://chatgpt.com/"},
			},
		})
		if err == nil && resp.status == http.StatusOK {
			if accessToken := stringAt(resp.json, "accessToken"); accessToken != "" {
				session.headers.Set("Authorization", "Bearer "+accessToken)
			}
		}
	}
	return session, nil
}

func setChatGPTBrowserHeaders(session *httpSession, deviceID string, fingerprints ...browserFingerprint) {
	if session == nil {
		return
	}
	fingerprint := session.fingerprint.withFallback(defaultBrowserLocale)
	if len(fingerprints) > 0 {
		fingerprint = fingerprints[0].withFallback(defaultBrowserLocale)
	}
	fingerprint.applyBrowserHeaders(session.headers)
	session.headers.Set("Accept", "*/*")
	session.headers.Set("Origin", "https://chatgpt.com")
	session.headers.Set("Referer", "https://chatgpt.com/")
	session.headers.Set("oai-device-id", strings.TrimSpace(deviceID))
	session.headers.Set("oai-language", fingerprint.OAILanguage)
	session.headers.Set("sec-fetch-dest", "empty")
	session.headers.Set("sec-fetch-mode", "cors")
	session.headers.Set("sec-fetch-site", "same-origin")
}

func chatGPTCookieHeader(sessionToken, deviceID string) string {
	parts := sessionCookieParts(sessionToken)
	deviceID = firstNonEmpty(cookiePartValue(parts, "oai-did"), strings.TrimSpace(deviceID))
	if deviceID != "" && !hasCookiePart(parts, "oai-did") {
		parts = append(parts, "oai-did="+deviceID)
	}
	return strings.Join(parts, "; ")
}

func hasCookiePart(parts []string, name string) bool {
	return cookiePartValue(parts, name) != ""
}

func cookiePartValue(parts []string, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, part := range parts {
		if !strings.Contains(part, "=") {
			continue
		}
		fields := strings.SplitN(part, "=", 2)
		if strings.TrimSpace(fields[0]) == name {
			return strings.TrimSpace(fields[1])
		}
	}
	return ""
}

func sessionCookieParts(value string) []string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "cookie:") {
		raw = strings.TrimSpace(raw[strings.Index(raw, ":")+1:])
	}
	if token := sessionTokenFromJSON(raw); token != "" {
		raw = token
	}
	if strings.Contains(raw, "=") {
		var parts []string
		foundSession := false
		seen := map[string]bool{}
		for _, chunk := range strings.Split(raw, ";") {
			part := strings.Trim(strings.TrimSpace(chunk), `'"`)
			if !strings.Contains(part, "=") {
				continue
			}
			name := strings.TrimSpace(strings.SplitN(part, "=", 2)[0])
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			parts = append(parts, part)
			if sessionCookieNameMatches(name) {
				foundSession = true
			}
		}
		if foundSession {
			return parts
		}
	}
	token := strings.Trim(raw, `'"`)
	if token == "" {
		return nil
	}
	if len(token) <= sessionCookieChunkSize {
		return []string{sessionCookieName + "=" + token}
	}
	var out []string
	for idx, offset := 0, 0; offset < len(token); idx, offset = idx+1, offset+sessionCookieChunkSize {
		end := offset + sessionCookieChunkSize
		if end > len(token) {
			end = len(token)
		}
		out = append(out, fmt.Sprintf("%s.%d=%s", sessionCookieName, idx, token[offset:end]))
	}
	return out
}

func sessionTokenFromJSON(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{") {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return ""
	}
	return firstNonEmpty(stringAt(payload, "sessionToken"), stringAt(payload, "session_token"))
}

func sessionCookieNameMatches(name string) bool {
	name = strings.TrimSpace(name)
	if name == sessionCookieName || name == sessionCookieFallbackName {
		return true
	}
	for _, base := range []string{sessionCookieName, sessionCookieFallbackName} {
		if strings.HasPrefix(name, base+".") && regexp.MustCompile(`^\d+$`).MatchString(strings.TrimPrefix(name, base+".")) {
			return true
		}
	}
	return false
}

func accessTokenTier(token string, sourcePrefix string) tierProbe {
	auth := accessTokenAuthClaims(token)
	for _, key := range []string{"chatgpt_plan_type", "chatgpt_planType", "plan_type", "planType"} {
		if result := tierResult(auth[key], sourcePrefix+"."+key); result.Checked {
			return result
		}
	}
	return tierProbe{}
}

func accessTokenAuthClaims(token string) map[string]any {
	payload := decodeJWTPayload(token)
	if payload == nil {
		return nil
	}
	auth, _ := payload["https://api.openai.com/auth"].(map[string]any)
	return auth
}

func accessTokenAccountID(token string) string {
	auth := accessTokenAuthClaims(token)
	return firstNonEmpty(stringAt(auth, "chatgpt_account_id"), stringAt(auth, "account_id"))
}

func basicAuth(value string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(value+":"))
}
