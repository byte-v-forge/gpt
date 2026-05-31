package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/byte-v-forge/common-lib/browserhttp"
	"github.com/byte-v-forge/common-lib/stringx"

	"orchestrator/pb"
)

const (
	codexOAuthProtocolUserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	codexOAuthProtocolAcceptLanguage = "en-US,en;q=0.9"
	codexOAuthProtocolLanguage       = "en-US"
)

type gptProtocolHTTPProfile struct {
	ProxyURL       string
	TLSProfileName string
}

type GptClient struct {
	cfg                     CodexOAuthConfig
	state                   *pb.CodexOAuthProtocolState
	client                  *browserhttp.Client
	profile                 gptProtocolHTTPProfile
	lastRequestSentAtUnixMs int64
}

type codexOAuthProtocolHTTPResponse struct {
	StatusCode   int
	Header       fhttp.Header
	Body         []byte
	JSON         map[string]any
	SentAtUnixMs int64
}

func newGptClient(cfg CodexOAuthConfig, state *pb.CodexOAuthProtocolState, profile gptProtocolHTTPProfile) (*GptClient, error) {
	cfg = cfg.withDefaults()
	profile = codexOAuthProtocolCleanProfile(cfg, profile)
	client, err := browserhttp.New(browserhttp.Config{
		Timeout:            45 * time.Second,
		ProxyURL:           profile.ProxyURL,
		TLSProfileName:     profile.TLSProfileName,
		DisableHTTP3:       true,
		NotFollowRedirects: true,
	})
	if err != nil {
		return nil, err
	}
	if state != nil {
		restoreCodexOAuthProtocolCookies(client.CookieJar(), state.Cookies)
	}
	return &GptClient{cfg: cfg, state: state, client: client, profile: profile}, nil
}

func restoreCodexOAuthProtocolCookies(jar browserhttp.CookieJar, cookies []*pb.CodexOAuthProtocolCookie) {
	byHost := map[string][]*fhttp.Cookie{}
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.GetHostKey()) == "" || strings.TrimSpace(cookie.GetName()) == "" || cookie.GetValue() == "" {
			continue
		}
		byHost[cookie.GetHostKey()] = append(byHost[cookie.GetHostKey()], codexOAuthProtocolCookieFromState(cookie))
	}
	for hostKey, values := range byHost {
		u := &url.URL{Scheme: "https", Host: hostKey, Path: "/"}
		jar.SetCookies(u, values)
	}
}

func (c *GptClient) Close() {
	if c != nil && c.client != nil {
		c.client.CloseIdleConnections()
	}
}

func (c *GptClient) get(ctx context.Context, rawURL, referer string, acceptHTML bool) (*codexOAuthProtocolHTTPResponse, error) {
	return c.request(ctx, fhttp.MethodGet, rawURL, referer, acceptHTML, nil)
}

func (c *GptClient) postJSON(ctx context.Context, rawURL, referer string, payload any, extraHeaders ...map[string]string) (*codexOAuthProtocolHTTPResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, fhttp.MethodPost, rawURL, referer, false, body, extraHeaders...)
}

func (c *GptClient) postForm(ctx context.Context, rawURL, referer string, form url.Values, extraHeaders ...map[string]string) (*codexOAuthProtocolHTTPResponse, error) {
	headers := append([]map[string]string{{"Content-Type": "application/x-www-form-urlencoded"}}, extraHeaders...)
	return c.request(ctx, fhttp.MethodPost, rawURL, referer, false, []byte(form.Encode()), headers...)
}

func (c *GptClient) request(ctx context.Context, method, rawURL, referer string, acceptHTML bool, body []byte, extraHeaders ...map[string]string) (*codexOAuthProtocolHTTPResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("codex oauth protocol client is nil")
	}
	if err := requireGptOpenAIURL(rawURL); err != nil {
		return nil, err
	}
	c.lastRequestSentAtUnixMs = 0
	headers := c.headers(referer, acceptHTML, body != nil)
	for _, extra := range extraHeaders {
		for key, value := range extra {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
				headers.Set(key, value)
			}
		}
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	sentAtUnixMs := time.Now().UnixMilli()
	c.lastRequestSentAtUnixMs = sentAtUnixMs
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &codexOAuthProtocolHTTPResponse{StatusCode: resp.StatusCode, Header: browserhttp.ToFHTTPHeader(resp.Header), SentAtUnixMs: sentAtUnixMs}
	out.Body, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if c.state != nil {
		applyCodexOAuthProtocolCookieSnapshot(c.state, c.client.CookieJar().GetAllCookies())
	}
	if len(out.Body) > 0 && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		_ = json.Unmarshal(out.Body, &out.JSON)
	}
	return out, nil
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

func (c *GptClient) headers(referer string, acceptHTML bool, hasBody bool) http.Header {
	if strings.TrimSpace(referer) == "" {
		referer = "https://auth.openai.com/"
	}
	origin := "https://auth.openai.com"
	if parsed, err := url.Parse(referer); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	accept := "application/json"
	if acceptHTML {
		accept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	}
	headers := http.Header{
		"Accept":  {accept},
		"Origin":  {origin},
		"Referer": {referer},
	}
	if hasBody {
		headers.Set("Content-Type", "application/json")
	}
	c.applyGptIdentityHeaders(headers)
	return headers
}

func (c *GptClient) applyGptIdentityHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", codexOAuthProtocolUserAgent)
	}
	headers.Set("Accept-Language", codexOAuthProtocolAcceptLanguage)
	if deviceID := c.deviceID(); deviceID != "" {
		headers.Set("oai-device-id", deviceID)
	}
	for key, value := range codexOAuthProtocolDatadogHeaders() {
		headers.Set(key, value)
	}
}

func (c *GptClient) deviceID() string {
	if c == nil {
		return ""
	}
	if c.state != nil {
		if deviceID := strings.TrimSpace(c.state.GetDeviceId()); deviceID != "" {
			return deviceID
		}
	}
	if c.client != nil && c.client.CookieJar() != nil {
		for _, cookies := range c.client.CookieJar().GetAllCookies() {
			for _, cookie := range cookies {
				if cookie != nil && strings.EqualFold(cookie.Name, "oai-did") && strings.TrimSpace(cookie.Value) != "" {
					deviceID := strings.TrimSpace(cookie.Value)
					if c.state != nil {
						c.state.DeviceId = deviceID
					}
					return deviceID
				}
			}
		}
	}
	return ""
}

func (c *GptClient) userAgent() string {
	return codexOAuthProtocolUserAgent
}

func (c *GptClient) cookieValue(name string, hostHints ...string) string {
	name = strings.TrimSpace(name)
	if c == nil || c.client == nil || c.client.CookieJar() == nil || name == "" {
		return ""
	}
	for hostKey, cookies := range c.client.CookieJar().GetAllCookies() {
		if !protocolCookieHostMatches(hostKey, hostHints...) {
			continue
		}
		for _, cookie := range cookies {
			if cookie != nil && strings.EqualFold(cookie.Name, name) && strings.TrimSpace(cookie.Value) != "" {
				return strings.TrimSpace(cookie.Value)
			}
		}
	}
	return ""
}

func (c *GptClient) sessionTokenValue(hostHints ...string) string {
	if c == nil || c.client == nil || c.client.CookieJar() == nil {
		return ""
	}
	cookies := make([]map[string]string, 0)
	for hostKey, values := range c.client.CookieJar().GetAllCookies() {
		if !protocolCookieHostMatches(hostKey, hostHints...) {
			continue
		}
		for _, cookie := range values {
			if cookie == nil || strings.TrimSpace(cookie.Value) == "" {
				continue
			}
			cookies = append(cookies, map[string]string{
				"name":   strings.TrimSpace(cookie.Name),
				"value":  strings.TrimSpace(cookie.Value),
				"domain": strings.TrimSpace(cookie.Domain),
			})
		}
	}
	return extractBrowserSessionToken(cookies)
}

func protocolCookieHostMatches(hostKey string, hostHints ...string) bool {
	if len(hostHints) == 0 {
		return true
	}
	hostKey = strings.ToLower(strings.TrimSpace(hostKey))
	for _, hint := range hostHints {
		hint = strings.TrimSpace(strings.ToLower(hint))
		if hint != "" && strings.Contains(hostKey, hint) {
			return true
		}
	}
	return false
}

func codexOAuthProtocolDefaultProfile(cfg CodexOAuthConfig) gptProtocolHTTPProfile {
	cfg = cfg.withDefaults()
	return gptProtocolHTTPProfile{ProxyURL: cfg.ProtocolProxyURL, TLSProfileName: cfg.ProtocolTLSProfile}
}

func codexOAuthProtocolCleanProfile(cfg CodexOAuthConfig, profile gptProtocolHTTPProfile) gptProtocolHTTPProfile {
	cfg = cfg.withDefaults()
	if strings.TrimSpace(profile.ProxyURL) == "" {
		profile.ProxyURL = cfg.ProtocolProxyURL
	} else {
		profile.ProxyURL = strings.TrimSpace(profile.ProxyURL)
	}
	profile.TLSProfileName = cfg.ProtocolTLSProfile
	return profile
}

func codexOAuthProtocolResponseJSON(resp *codexOAuthProtocolHTTPResponse) map[string]any {
	if resp == nil || resp.JSON == nil {
		return nil
	}
	return resp.JSON
}

func codexOAuthProtocolRequireOK(resp *codexOAuthProtocolHTTPResponse, label string) error {
	if resp == nil {
		return fmt.Errorf("%s failed: response missing", label)
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	if codexOAuthProtocolEdgeChallenge(resp) {
		return fmt.Errorf("%s failed: edge challenge status %d", label, resp.StatusCode)
	}
	return fmt.Errorf("%s failed: status %d %s", label, resp.StatusCode, codexOAuthProtocolSafeText(string(resp.Body), 360))
}

func codexOAuthProtocolEdgeChallenge(resp *codexOAuthProtocolHTTPResponse) bool {
	if resp == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("cf-mitigated")), "challenge") {
		return true
	}
	body := strings.ToLower(string(resp.Body))
	for _, marker := range []string{"challenge-platform", "cf-chl", "cf-mitigated", "just a moment"} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

var (
	codexOAuthProtocolEmailRE = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	codexOAuthProtocolPhoneRE = regexp.MustCompile(`\+\d{6,}`)
	codexOAuthProtocolCodeRE  = regexp.MustCompile(`(?i)"(code|otp|password|token|cookie)"\s*:\s*"[^"]+"`)
)

func codexOAuthProtocolSafeText(value string, maxLen int) string {
	value = codexOAuthProtocolCodeRE.ReplaceAllString(value, `"$1":"<redacted>"`)
	value = codexOAuthProtocolEmailRE.ReplaceAllString(value, "<email>")
	value = codexOAuthProtocolPhoneRE.ReplaceAllString(value, "<phone>")
	return stringx.CompactSnippet(value, maxLen)
}

func codexOAuthProtocolAbsoluteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		return "https://auth.openai.com" + raw
	}
	return raw
}

func codexOAuthProtocolRefererForStage(stage string) string {
	switch stage {
	case "email":
		return "https://auth.openai.com/log-in"
	case "create_password":
		return "https://auth.openai.com/create-account/password"
	case "password":
		return "https://auth.openai.com/log-in/password"
	case "email_otp":
		return "https://auth.openai.com/email-verification"
	case "about_you":
		return "https://auth.openai.com/about-you"
	case "add_phone":
		return "https://auth.openai.com/add-phone"
	case "phone_otp":
		return "https://auth.openai.com/phone-verification"
	case "consent":
		return "https://auth.openai.com/sign-in-with-chatgpt/codex/consent"
	default:
		return "https://auth.openai.com/"
	}
}
