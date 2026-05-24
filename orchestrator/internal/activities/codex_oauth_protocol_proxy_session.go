package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		return err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/api/proxy-runtime/session/new", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("codex protocol proxy new session: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read codex protocol proxy new session response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("codex protocol proxy new session failed: status %d %s", resp.StatusCode, compactBrowserAuthText(string(raw), 300))
	}
	var parsed codexOAuthProtocolProxySessionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parse codex protocol proxy new session response: %w", err)
	}
	data["protocol_proxy_session_new"] = true
	data["protocol_proxy_session_purpose"] = purpose
	data["protocol_proxy_pool_endpoints"] = len(parsed.Pool.Endpoints)
	if parsed.Session.SessionID != "" {
		data["protocol_proxy_session_hash"] = shortStateHash(parsed.Session.SessionID)
	}
	return nil
}
