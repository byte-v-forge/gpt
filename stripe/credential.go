package stripe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/byte-v-forge/common-lib/fingerprinthttp"
	"github.com/byte-v-forge/common-lib/jwtx"
)

type CredentialKind string

const (
	CredentialSessionToken CredentialKind = "session_token"
	CredentialAccessToken  CredentialKind = "access_token"
)

type Credential struct {
	Kind  CredentialKind
	Token string
}

func NewCredential(sessionToken string, accessToken string) (Credential, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken != "" {
		return Credential{Kind: CredentialAccessToken, Token: accessToken}, nil
	}
	if sessionToken != "" {
		return Credential{Kind: CredentialSessionToken, Token: sessionToken}, nil
	}
	return Credential{}, fmt.Errorf("chatgpt credential is required")
}

func (c Credential) IsZero() bool {
	return c.Kind == "" || strings.TrimSpace(c.Token) == ""
}

func (c Credential) AuthConfig() map[string]string {
	if c.IsZero() {
		return map[string]string{}
	}
	return map[string]string{string(c.Kind): c.Token}
}

func (c Credential) ApplyChatGPTHeaders(headers http.Header, profile fingerprinthttp.Profile) {
	profile = cleanGptProfile(profile)
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", profile.UserAgent)
	}
	headers.Set("Accept", "*/*")
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("Origin", "https://chatgpt.com")
	headers.Set("Referer", "https://chatgpt.com/")
	headers.Set("Content-Type", "application/json")
	headers.Del("sec-ch-ua")
	headers.Del("sec-ch-ua-mobile")
	headers.Del("sec-ch-ua-platform")
	headers.Del("oai-device-id")
	headers.Del("oai-language")
	if c.Kind == CredentialAccessToken {
		headers.Set("Authorization", "Bearer "+c.Token)
		return
	}
	cookie := SessionCookieHeader(c.Token, "")
	if cookie != "" {
		headers.Set("Cookie", cookie)
	}
}

const (
	sessionCookieName         = "__Secure-next-auth.session-token"
	sessionCookieFallbackName = "next-auth.session-token"
	sessionCookieChunkSize    = 4096 - 163
)

var sessionCookieNameRe = regexp.MustCompile(`^(__Secure-next-auth\.session-token|next-auth\.session-token)(\.\d+)?$`)

func SessionCookieHeader(sessionToken string, deviceID string) string {
	parts := SessionCookieParts(sessionToken)
	if deviceID = strings.TrimSpace(deviceID); deviceID != "" {
		parts = append(parts, "oai-did="+deviceID)
	}
	return strings.Join(parts, "; ")
}

func SessionCookieParts(value string) []string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "cookie:") {
		raw = strings.TrimSpace(strings.SplitN(raw, ":", 2)[1])
	}
	if token := sessionTokenFromJSON(raw); token != "" {
		raw = token
	}
	if strings.Contains(raw, "=") {
		found := make([]string, 0)
		for _, chunk := range strings.Split(raw, ";") {
			part := strings.Trim(strings.TrimSpace(chunk), `'"`)
			name, cookieValue, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			cookieValue = strings.Trim(strings.TrimSpace(cookieValue), `'"`)
			if sessionCookieNameRe.MatchString(name) && cookieValue != "" {
				found = append(found, name+"="+cookieValue)
			}
		}
		if len(found) > 0 {
			sort.SliceStable(found, func(i, j int) bool {
				return sessionCookieSortKey(found[i]) < sessionCookieSortKey(found[j])
			})
			return found
		}
	}
	token := strings.Trim(raw, `'"`)
	if token == "" {
		return nil
	}
	if len(token) <= sessionCookieChunkSize {
		return []string{sessionCookieName + "=" + token}
	}
	var parts []string
	for index, offset := 0, 0; offset < len(token); index, offset = index+1, offset+sessionCookieChunkSize {
		end := offset + sessionCookieChunkSize
		if end > len(token) {
			end = len(token)
		}
		parts = append(parts, fmt.Sprintf("%s.%d=%s", sessionCookieName, index, token[offset:end]))
	}
	return parts
}

func sessionTokenFromJSON(value string) string {
	text := strings.TrimSpace(value)
	if !strings.HasPrefix(text, "{") {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return ""
	}
	for _, key := range []string{"sessionToken", "session_token"} {
		if token := strings.TrimSpace(fmt.Sprint(payload[key])); token != "" && token != "<nil>" {
			return token
		}
	}
	return ""
}

func sessionCookieSortKey(part string) string {
	name, _, _ := strings.Cut(part, "=")
	baseOrder := "9"
	if strings.HasPrefix(name, sessionCookieName) {
		baseOrder = "0"
	} else if strings.HasPrefix(name, sessionCookieFallbackName) {
		baseOrder = "1"
	}
	chunk := "-1"
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		chunk = fmt.Sprintf("%08s", name[dot+1:])
	}
	return baseOrder + ":" + chunk + ":" + name
}

func AccessTokenClaims(accessToken string) map[string]any {
	return jwtx.PayloadOrNil(accessToken)
}

func AccessTokenAccountID(accessToken string) string {
	auth := accessTokenAuthClaims(accessToken)
	for _, key := range []string{"chatgpt_account_id", "account_id"} {
		if value := strings.TrimSpace(fmt.Sprint(auth[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func AccessTokenPlan(accessToken string) string {
	auth := accessTokenAuthClaims(accessToken)
	for _, key := range []string{"chatgpt_plan_type", "chatgpt_planType", "plan_type", "planType"} {
		if plan := normalizePlan(fmt.Sprint(auth[key])); plan != "" {
			return plan
		}
	}
	return ""
}

func accessTokenAuthClaims(accessToken string) map[string]any {
	claims := AccessTokenClaims(accessToken)
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if auth == nil {
		return map[string]any{}
	}
	return auth
}

func normalizePlan(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	switch text {
	case "plus", "pro", "team", "business", "enterprise", "free":
		return text
	}
	for _, plan := range []string{"plus", "pro", "team", "business", "enterprise", "free"} {
		if strings.Contains(text, plan) {
			return plan
		}
	}
	return ""
}
