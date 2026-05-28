package paymentsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/byte-v-forge/common-lib/stringx"
)

const (
	sessionCookieName         = "__Secure-next-auth.session-token"
	sessionCookieFallbackName = "next-auth.session-token"
	sessionCookieChunkSize    = 4096 - 163
	gptAcceptLanguage         = "en-US,en;q=0.9"
	gptOAILanguage            = "en-US"
)

type GptClient struct {
	session *httpSession
	profile requestProfile
}

func (s *Server) newGptClient(ctx context.Context, cred credential, profile requestProfile) (*GptClient, error) {
	if cred.empty() {
		return nil, fmt.Errorf("auth missing: need session_token or access_token")
	}
	profile = profile.withDefaults(defaultRequestProfile(profile.Name))
	fingerprint := profile.fingerprint()
	session, err := newHTTPSession(profile.ProxyURL, fingerprint)
	if err != nil {
		return nil, err
	}
	client := &GptClient{session: session, profile: profile}
	client.applyChatGPTHeaders()
	client.setHeader("Content-Type", "application/json")
	if cred.accessToken != "" {
		client.setAuthorization("Bearer " + cred.accessToken)
	}
	if cookie := chatGPTCookieHeader(cred.sessionToken, fingerprint.DeviceID); cookie != "" {
		client.setHeader("Cookie", cookie)
	}
	if cred.sessionToken != "" {
		resp, err := client.request(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", requestOptions{
			headers: http.Header{
				"Accept":  []string{"application/json"},
				"Referer": []string{"https://chatgpt.com/"},
			},
		})
		if err == nil && resp.status == http.StatusOK {
			if accessToken := stringAt(resp.json, "accessToken"); accessToken != "" {
				client.setAuthorization("Bearer " + accessToken)
			}
		}
	}
	return client, nil
}

func newGptClientWithFingerprint(proxyURL string, fingerprint browserFingerprint) (*GptClient, error) {
	session, err := newHTTPSession(proxyURL, fingerprint)
	if err != nil {
		return nil, err
	}
	client := &GptClient{session: session, profile: requestProfile{ProxyURL: proxyURL, TLSProfile: fingerprint.TLSProfileName, Locale: fingerprint.Language}}
	client.applyChatGPTHeaders()
	return client, nil
}

func (c *GptClient) request(ctx context.Context, method, rawURL string, opts requestOptions) (*httpResult, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("gpt client is nil")
	}
	if err := requireGptOpenAIURL(rawURL); err != nil {
		return nil, err
	}
	c.applyChatGPTHeaders()
	opts.headers = c.chatGPTHeaders(opts.headers)
	return c.session.request(ctx, method, rawURL, opts)
}

func (c *GptClient) setProxy(proxyURL string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("gpt client is nil")
	}
	return c.session.setProxy(proxyURL)
}

func (c *GptClient) close() {
	if c != nil && c.session != nil {
		c.session.close()
	}
}

func (c *GptClient) cookieHeader(rawURL string) string {
	if c == nil || c.session == nil {
		return ""
	}
	return c.session.cookieHeader(rawURL)
}

func (c *GptClient) fingerprint() browserFingerprint {
	if c != nil && c.session != nil {
		return c.session.fingerprint.withFallback(c.profile.Locale)
	}
	return defaultRequestProfile("checkout").fingerprint()
}

func (c *GptClient) header(name string) string {
	if c == nil || c.session == nil {
		return ""
	}
	return c.session.headers.Get(name)
}

func (c *GptClient) setHeader(name string, value string) {
	if c == nil || c.session == nil {
		return
	}
	c.session.headers.Set(name, value)
}

func (c *GptClient) setAuthorization(value string) {
	c.setHeader("Authorization", value)
}

func (c *GptClient) applyChatGPTHeaders() {
	if c == nil || c.session == nil {
		return
	}
	mergeHeader(c.session.headers, c.chatGPTHeaders(nil))
}

func (c *GptClient) chatGPTHeaders(extra http.Header) http.Header {
	fingerprint := c.fingerprint()
	headers := cloneHeader(extra)
	fingerprint.applyBrowserHeaders(headers)
	headers.Set("Accept-Language", gptAcceptLanguage)
	if headers.Get("Accept") == "" {
		headers.Set("Accept", "*/*")
	}
	headers.Set("Origin", "https://chatgpt.com")
	if headers.Get("Referer") == "" {
		headers.Set("Referer", "https://chatgpt.com/")
	}
	headers.Set("oai-device-id", strings.TrimSpace(fingerprint.DeviceID))
	headers.Set("oai-language", gptOAILanguage)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	return headers
}

func requireGptOpenAIURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "chatgpt.com" || strings.HasSuffix(host, ".chatgpt.com") || host == "openai.com" || strings.HasSuffix(host, ".openai.com") {
		return nil
	}
	return fmt.Errorf("gpt client refuses non gpt/openai url: %s", host)
}

func chatGPTCookieHeader(sessionToken, deviceID string) string {
	return cookieHeaderWithDeviceID(sessionCookieParts(sessionToken), deviceID)
}

func splitCookieHeader(value string) []string {
	var parts []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(value, ";") {
		part := strings.TrimSpace(raw)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		name := strings.TrimSpace(strings.SplitN(part, "=", 2)[0])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		parts = append(parts, part)
	}
	return parts
}

func cookieHeaderWithDeviceID(parts []string, deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return strings.Join(parts, "; ")
	}
	out := make([]string, 0, len(parts)+1)
	found := false
	seen := map[string]bool{}
	for _, part := range parts {
		if !strings.Contains(part, "=") {
			continue
		}
		name := strings.TrimSpace(strings.SplitN(part, "=", 2)[0])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if name == "oai-did" {
			out = append(out, "oai-did="+deviceID)
			found = true
			continue
		}
		out = append(out, part)
	}
	if !found {
		out = append(out, "oai-did="+deviceID)
	}
	return strings.Join(out, "; ")
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
	return stringx.FirstNonEmpty(stringAt(payload, "sessionToken"), stringAt(payload, "session_token"))
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
	return stringx.FirstNonEmpty(stringAt(auth, "chatgpt_account_id"), stringAt(auth, "account_id"))
}

func basicAuth(value string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(value+":"))
}
