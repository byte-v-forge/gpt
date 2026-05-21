package paymentsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/byte-v-forge/gpt/gopay/protocol"
)

const defaultTimeout = 30 * time.Second

type httpSession struct {
	client  *http.Client
	headers http.Header
}

type requestOptions struct {
	headers    http.Header
	jsonBody   any
	formBody   url.Values
	query      url.Values
	noRedirect bool
}

type httpResult struct {
	status  int
	headers http.Header
	body    []byte
	json    map[string]any
}

func newHTTPSession(proxyURL string) (*httpSession, error) {
	client, err := protocol.NewHTTPClient(defaultTimeout, proxyURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client.Jar = jar
	return &httpSession{client: client, headers: make(http.Header)}, nil
}

func (s *httpSession) close() {}

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
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), target.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	client := s.client
	if opts.noRedirect {
		clone := *s.client
		clone.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &clone
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
			return &httpResult{status: resp.StatusCode, headers: resp.Header.Clone(), body: raw, json: map[string]any(payload)}, nil
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

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header)
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	return dst
}

func mergeHeader(dst http.Header, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
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
