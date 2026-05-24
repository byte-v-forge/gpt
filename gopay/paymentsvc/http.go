package paymentsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/byte-v-forge/gpt/gopay/protocol"
)

const defaultTimeout = 30 * time.Second

type httpSession struct {
	client      tlsclient.HttpClient
	cookieJar   fhttp.CookieJar
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
		cookieJar:   tlsclient.NewCookieJar(),
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
	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(int(defaultTimeout.Seconds())),
		tlsclient.WithClientProfile(fingerprint.TLSProfile),
		tlsclient.WithDisableHttp3(),
		tlsclient.WithCookieJar(s.cookieJar),
	}
	if s.proxyURL != "" {
		options = append(options, tlsclient.WithProxyUrl(s.proxyURL))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return err
	}
	if s.client != nil {
		s.client.CloseIdleConnections()
	}
	s.client = client
	s.fingerprint = fingerprint
	return nil
}

func (s *httpSession) close() {
	if s != nil && s.client != nil {
		s.client.CloseIdleConnections()
	}
}

func (s *httpSession) cookieHeader(rawURL string) string {
	if s == nil || s.cookieJar == nil {
		return ""
	}
	target, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	cookies := s.cookieJar.Cookies(target)
	parts := make([]string, 0, len(cookies))
	seen := map[string]bool{}
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		if seen[cookie.Name] {
			continue
		}
		seen[cookie.Name] = true
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
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
	var body io.Reader
	headers := cloneHeader(s.headers)
	mergeHeader(headers, opts.headers)
	if opts.jsonBody != nil {
		raw, err := protocol.CompactJSON(opts.jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
		if headers.Get("Content-Type") == "" {
			headers.Set("Content-Type", "application/json")
		}
	} else if opts.formBody != nil {
		body = strings.NewReader(opts.formBody.Encode())
		headers.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if len(opts.query) > 0 {
		query := target.Query()
		for key, values := range opts.query {
			for _, value := range values {
				query.Add(key, value)
			}
		}
		target.RawQuery = query.Encode()
	}
	req, err := fhttp.NewRequestWithContext(ctx, strings.ToUpper(method), target.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header = toFHTTPHeader(headers)
	client := s.client
	if opts.noRedirect {
		client.SetFollowRedirect(false)
		defer client.SetFollowRedirect(true)
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
			if readErr != nil {
				return nil, readErr
			}
			payload, _ := protocol.DecodeJSONMap(raw)
			return &httpResult{status: resp.StatusCode, headers: fromFHTTPHeader(resp.Header), body: raw, json: map[string]any(payload)}, nil
		}
		lastErr = err
		if attempt >= 3 || !retryableTransportError(err) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return nil, lastErr
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
	return protocol.Snippet(protocol.RedactText(text), limit)
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

func toFHTTPHeader(src stdhttp.Header) fhttp.Header {
	dst := make(fhttp.Header)
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	return dst
}

func fromFHTTPHeader(src fhttp.Header) stdhttp.Header {
	dst := make(stdhttp.Header)
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	return dst
}

func retryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, hint := range []string{"tls", "connection reset", "connection aborted", "timed out", "timeout", "temporarily unavailable", "network is unreachable", "proxy", "eof"} {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}
