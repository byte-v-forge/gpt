package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const codexOAuthProtocolUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

type codexOAuthProtocolHTTPClient struct {
	cfg    CodexOAuthConfig
	state  *codexOAuthProtocolState
	client tlsclient.HttpClient
	jar    tlsclient.CookieJar
}

type codexOAuthProtocolHTTPResponse struct {
	StatusCode int
	Header     fhttp.Header
	Body       []byte
	JSON       map[string]any
}

func newCodexOAuthProtocolHTTPClient(cfg CodexOAuthConfig, state *codexOAuthProtocolState) (*codexOAuthProtocolHTTPClient, error) {
	cfg = cfg.withDefaults()
	jar := tlsclient.NewCookieJar()
	if state != nil {
		restoreCodexOAuthProtocolCookies(jar, state.Cookies)
	}
	profile, ok := codexOAuthProtocolTLSProfile(cfg.ProtocolTLSProfile)
	if !ok {
		profile, _ = codexOAuthProtocolTLSProfile(defaultCodexOAuthProtocolTLSProfile)
	}
	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(45),
		tlsclient.WithClientProfile(profile),
		tlsclient.WithCookieJar(jar),
		tlsclient.WithNotFollowRedirects(),
		tlsclient.WithDisableHttp3(),
	}
	if proxyURL := strings.TrimSpace(cfg.ProtocolProxyURL); proxyURL != "" {
		options = append(options, tlsclient.WithProxyUrl(proxyURL))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}
	return &codexOAuthProtocolHTTPClient{cfg: cfg, state: state, client: client, jar: jar}, nil
}

func restoreCodexOAuthProtocolCookies(jar tlsclient.CookieJar, cookies []codexOAuthProtocolCookie) {
	byHost := map[string][]*fhttp.Cookie{}
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.HostKey) == "" || strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		byHost[cookie.HostKey] = append(byHost[cookie.HostKey], codexOAuthProtocolCookieFromState(cookie))
	}
	for hostKey, values := range byHost {
		u := &url.URL{Scheme: "https", Host: hostKey, Path: "/"}
		jar.SetCookies(u, values)
	}
}

func codexOAuthProtocolTLSProfile(name string) (profiles.ClientProfile, bool) {
	name = strings.TrimSpace(name)
	for candidate, profile := range profiles.MappedTLSClients {
		if strings.EqualFold(candidate, name) {
			return profile, true
		}
	}
	return profiles.ClientProfile{}, false
}

func (c *codexOAuthProtocolHTTPClient) get(ctx context.Context, rawURL, referer string, acceptHTML bool) (*codexOAuthProtocolHTTPResponse, error) {
	return c.request(ctx, fhttp.MethodGet, rawURL, referer, acceptHTML, nil)
}

func (c *codexOAuthProtocolHTTPClient) postJSON(ctx context.Context, rawURL, referer string, payload any, extraHeaders ...map[string]string) (*codexOAuthProtocolHTTPResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, fhttp.MethodPost, rawURL, referer, false, body, extraHeaders...)
}

func (c *codexOAuthProtocolHTTPClient) postForm(ctx context.Context, rawURL, referer string, form url.Values, extraHeaders ...map[string]string) (*codexOAuthProtocolHTTPResponse, error) {
	headers := append([]map[string]string{{"Content-Type": "application/x-www-form-urlencoded"}}, extraHeaders...)
	return c.request(ctx, fhttp.MethodPost, rawURL, referer, false, []byte(form.Encode()), headers...)
}

func (c *codexOAuthProtocolHTTPClient) request(ctx context.Context, method, rawURL, referer string, acceptHTML bool, body []byte, extraHeaders ...map[string]string) (*codexOAuthProtocolHTTPResponse, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := fhttp.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header = codexOAuthProtocolHeaders(referer, c.deviceID(), acceptHTML, body != nil)
	for _, headers := range extraHeaders {
		for key, value := range headers {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
				req.Header.Set(key, value)
			}
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &codexOAuthProtocolHTTPResponse{StatusCode: resp.StatusCode, Header: resp.Header}
	out.Body, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if c.state != nil {
		c.state.applyCookieSnapshot(c.jar.GetAllCookies())
	}
	if len(out.Body) > 0 && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		_ = json.Unmarshal(out.Body, &out.JSON)
	}
	return out, nil
}

func (c *codexOAuthProtocolHTTPClient) deviceID() string {
	if c == nil || c.state == nil {
		return ""
	}
	if deviceID := strings.TrimSpace(c.state.DeviceID); deviceID != "" {
		return deviceID
	}
	for _, cookies := range c.jar.GetAllCookies() {
		for _, cookie := range cookies {
			if cookie != nil && strings.EqualFold(cookie.Name, "oai-did") && strings.TrimSpace(cookie.Value) != "" {
				c.state.DeviceID = strings.TrimSpace(cookie.Value)
				return c.state.DeviceID
			}
		}
	}
	return ""
}

func (c *codexOAuthProtocolHTTPClient) cookieValue(name string, hostHints ...string) string {
	name = strings.TrimSpace(name)
	if c == nil || name == "" {
		return ""
	}
	for hostKey, cookies := range c.jar.GetAllCookies() {
		if len(hostHints) > 0 {
			matched := false
			for _, hint := range hostHints {
				if hint = strings.TrimSpace(strings.ToLower(hint)); hint != "" && strings.Contains(strings.ToLower(hostKey), hint) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		for _, cookie := range cookies {
			if cookie != nil && strings.EqualFold(cookie.Name, name) && strings.TrimSpace(cookie.Value) != "" {
				return strings.TrimSpace(cookie.Value)
			}
		}
	}
	return ""
}

func codexOAuthProtocolHeaders(referer, deviceID string, acceptHTML bool, hasBody bool) fhttp.Header {
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
	headers := fhttp.Header{
		"Accept":          {accept},
		"Accept-Language": {"en-US,en;q=0.9"},
		"Origin":          {origin},
		"Referer":         {referer},
		"User-Agent":      {codexOAuthProtocolUserAgent},
	}
	if hasBody {
		headers.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(deviceID) != "" {
		headers.Set("oai-device-id", strings.TrimSpace(deviceID))
	}
	for key, value := range codexOAuthProtocolDatadogHeaders() {
		headers.Set(key, value)
	}
	return headers
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
	return fmt.Errorf("%s failed: status %d %s", label, resp.StatusCode, codexOAuthProtocolSafeText(string(resp.Body), 360))
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
	return compactBrowserAuthText(value, maxLen)
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

func codexOAuthProtocolShortSleep(ctx context.Context) error {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
