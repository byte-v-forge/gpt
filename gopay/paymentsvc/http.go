package paymentsvc

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/fingerprinthttp"
	"github.com/byte-v-forge/common-lib/redactx"
)

const defaultTimeout = 30 * time.Second

type httpSession struct {
	client      *fingerprinthttp.Client
	proxyURL    string
	headers     stdhttp.Header
	fingerprint browserFingerprint
}

type requestOptions struct {
	headers    stdhttp.Header
	jsonBody   any
	formBody   url.Values
	query      url.Values
	noRedirect bool
}

type httpResult struct {
	status  int
	headers stdhttp.Header
	body    []byte
	json    map[string]any
}

func newHTTPSession(proxyURL string, fingerprints ...browserFingerprint) (*httpSession, error) {
	fingerprint := stablePaymentBrowserFingerprint(defaultBrowserLocale, "", "")
	if len(fingerprints) > 0 {
		fingerprint = fingerprints[0].withFallback(defaultBrowserLocale)
	}
	session := &httpSession{
		proxyURL:    strings.TrimSpace(proxyURL),
		headers:     make(stdhttp.Header),
		fingerprint: fingerprint,
	}
	if err := session.rebuildClient(fingerprint); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *httpSession) rebuildClient(fingerprint browserFingerprint) error {
	fingerprint = fingerprint.withFallback(defaultBrowserLocale)
	client, err := fingerprinthttp.New(fingerprinthttp.Config{
		Timeout:      defaultTimeout,
		ProxyURL:     s.proxyURL,
		Profile:      fingerprint.httpProfile(s.proxyURL),
		DisableHTTP3: true,
		RetryMax:     3,
	})
	if err != nil {
		return err
	}
	if s.client != nil {
		s.client.Close()
	}
	s.client = client
	s.fingerprint = fingerprint
	return nil
}

func (s *httpSession) setProxy(proxyURL string) error {
	if s == nil {
		return fmt.Errorf("http session is nil")
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if s.proxyURL == proxyURL {
		return nil
	}
	s.proxyURL = proxyURL
	if s.client != nil {
		if err := s.client.SetProxy(proxyURL); err != nil {
			return err
		}
	}
	return nil
}

func (s *httpSession) close() {
	if s != nil && s.client != nil {
		s.client.Close()
	}
}

func (s *httpSession) cookieHeader(rawURL string) string {
	if s == nil || s.client == nil {
		return ""
	}
	return s.client.CookieHeader(rawURL)
}

func mergeCookieHeaders(values ...string) string {
	parts := make([]string, 0)
	seen := map[string]bool{}
	for _, value := range values {
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
	}
	return strings.Join(parts, "; ")
}

func (s *httpSession) request(ctx context.Context, method, rawURL string, opts requestOptions) (*httpResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("http session is nil")
	}
	headers := cloneHeader(s.headers)
	mergeHeader(headers, opts.headers)
	resp, err := s.client.Request(ctx, method, rawURL, fingerprinthttp.RequestOptions{
		Headers:    headers,
		JSONBody:   opts.jsonBody,
		FormBody:   opts.formBody,
		Query:      opts.query,
		NoRedirect: opts.noRedirect,
	})
	if err != nil {
		return nil, err
	}
	return &httpResult{status: resp.StatusCode, headers: resp.Headers, body: resp.Body, json: resp.JSON}, nil
}

func (r *httpResult) data() map[string]any {
	if r == nil {
		return map[string]any{}
	}
	if data, ok := r.json["data"].(map[string]any); ok {
		return data
	}
	return r.json
}

func (r *httpResult) excerpt(limit int) string {
	if r == nil {
		return "<nil response>"
	}
	if limit <= 0 {
		limit = 600
	}
	text := strings.TrimSpace(string(r.body))
	if text == "" {
		raw, _ := json.Marshal(r.json)
		text = string(raw)
	}
	return redactx.Snippet(redactx.Text(text), limit)
}

func (r *httpResult) require(status int, label string) error {
	if r == nil {
		return fmt.Errorf("%s: empty response", label)
	}
	if r.status != status {
		return fmt.Errorf("%s %d: %s", label, r.status, r.excerpt(500))
	}
	return nil
}

func cloneHeader(src stdhttp.Header) stdhttp.Header {
	dst := make(stdhttp.Header)
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	return dst
}

func mergeHeader(dst stdhttp.Header, src stdhttp.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
