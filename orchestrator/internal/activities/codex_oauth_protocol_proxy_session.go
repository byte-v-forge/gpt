package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type codexOAuthProtocolProxySessionResponse struct {
	Session struct {
		SessionID string `json:"session_id"`
	} `json:"session"`
	Pool struct {
		Endpoints []map[string]any `json:"endpoints"`
	} `json:"pool"`
}

func (s *Server) startCodexOAuthProtocolProxySession(ctx context.Context, cfg CodexOAuthConfig, purpose string, data map[string]any) error {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.ProtocolProxyRuntimeHTTPAddr), "/")
	if baseURL == "" {
		data["protocol_proxy_session_skipped"] = true
		return nil
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		data["protocol_proxy_session_attempt"] = attempt
		parsed, err := createCodexOAuthProtocolProxySession(ctx, baseURL, purpose)
		if err != nil {
			lastErr = err
			data["protocol_proxy_session_error"] = codexOAuthProtocolSafeText(err.Error(), 180)
			continue
		}
		recordCodexOAuthProtocolProxySession(data, purpose, parsed)
		if err := waitCodexOAuthProtocolProxyReady(ctx, baseURL, cfg.ProtocolProxyURL); err != nil {
			lastErr = err
			data["protocol_proxy_probe_error"] = codexOAuthProtocolSafeText(err.Error(), 180)
			continue
		}
		if err := probeCodexOAuthProtocolProxyEgress(ctx, cfg); err != nil {
			lastErr = err
			data["protocol_proxy_probe_error"] = codexOAuthProtocolSafeText(err.Error(), 180)
			continue
		}
		data["protocol_proxy_probe_ok"] = true
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown proxy probe error")
	}
	return fmt.Errorf("codex protocol proxy unavailable after probe retries: %w", lastErr)
}

func createCodexOAuthProtocolProxySession(ctx context.Context, baseURL string, purpose string) (codexOAuthProtocolProxySessionResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	payload := map[string]any{
		"force_new": true,
		"policy": map[string]any{
			"mode":               "PROXY_SESSION_MODE_STICKY",
			"region":             "JP",
			"city":               "Tokyo",
			"sticky_ttl_minutes": 10,
			"upstream_kind":      "PROXY_UPSTREAM_KIND_DYNAMIC_IP",
			"rotation_mode":      "PROXY_ROTATION_MODE_STICKY_SESSION",
			"labels": map[string]string{
				"purpose":  purpose,
				"driver":   "codex_protocol",
				"rotation": "active",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return codexOAuthProtocolProxySessionResponse{}, err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/api/proxy-runtime/session/new", bytes.NewReader(body))
	if err != nil {
		return codexOAuthProtocolProxySessionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return codexOAuthProtocolProxySessionResponse{}, fmt.Errorf("codex protocol proxy new session: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return codexOAuthProtocolProxySessionResponse{}, fmt.Errorf("read codex protocol proxy new session response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return codexOAuthProtocolProxySessionResponse{}, fmt.Errorf("codex protocol proxy new session failed: status %d %s", resp.StatusCode, compactBrowserAuthText(string(raw), 300))
	}
	var parsed codexOAuthProtocolProxySessionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return codexOAuthProtocolProxySessionResponse{}, fmt.Errorf("parse codex protocol proxy new session response: %w", err)
	}
	return parsed, nil
}

func recordCodexOAuthProtocolProxySession(data map[string]any, purpose string, parsed codexOAuthProtocolProxySessionResponse) {
	data["protocol_proxy_session_new"] = true
	data["protocol_proxy_session_purpose"] = purpose
	data["protocol_proxy_pool_endpoints"] = len(parsed.Pool.Endpoints)
	if parsed.Session.SessionID != "" {
		data["protocol_proxy_session_hash"] = shortStateHash(parsed.Session.SessionID)
	}
}

func waitCodexOAuthProtocolProxyReady(ctx context.Context, baseURL string, proxyURL string) error {
	if err := waitCodexOAuthProtocolRuntimeReady(ctx, baseURL); err != nil {
		return err
	}
	return waitCodexOAuthProtocolProxyPort(ctx, proxyURL)
}

func waitCodexOAuthProtocolRuntimeReady(ctx context.Context, baseURL string) error {
	var lastErr error
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/readyz", nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
					cancel()
					return nil
				}
				lastErr = fmt.Errorf("runtime ready status %d", resp.StatusCode)
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
		cancel()
		if err := codexOAuthProtocolSleepContext(ctx, 400*time.Millisecond); err != nil {
			return err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("runtime ready timeout")
	}
	return fmt.Errorf("codex protocol proxy runtime not ready: %w", lastErr)
}

func waitCodexOAuthProtocolProxyPort(ctx context.Context, proxyURL string) error {
	addr, err := codexOAuthProtocolProxyAddr(proxyURL)
	if err != nil || addr == "" {
		return err
	}
	var lastErr error
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{Timeout: 800 * time.Millisecond}).DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		if err := codexOAuthProtocolSleepContext(ctx, 400*time.Millisecond); err != nil {
			return err
		}
	}
	return fmt.Errorf("codex protocol proxy listener not ready: %w", lastErr)
}

func codexOAuthProtocolProxyAddr(proxyURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil {
		return "", fmt.Errorf("parse protocol proxy url: %w", err)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("protocol proxy url host missing")
	}
	return parsed.Host, nil
}

func probeCodexOAuthProtocolProxyEgress(ctx context.Context, cfg CodexOAuthConfig) error {
	if strings.TrimSpace(cfg.ProtocolProxyURL) == "" {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	client, err := newCodexOAuthProtocolHTTPClient(cfg, &codexOAuthProtocolState{})
	if err != nil {
		return err
	}
	resp, err := client.get(probeCtx, "https://chatgpt.com/api/auth/csrf", protocolAuthChatGPTLoginURL, false)
	if err != nil {
		return fmt.Errorf("protocol proxy csrf probe: %w", err)
	}
	if err := codexOAuthProtocolRequireOK(resp, "protocol proxy csrf probe"); err != nil {
		return err
	}
	if strings.TrimSpace(stringAny(codexOAuthProtocolResponseJSON(resp)["csrfToken"])) == "" {
		return fmt.Errorf("protocol proxy csrf probe token missing")
	}
	return nil
}

func codexOAuthProtocolSleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
