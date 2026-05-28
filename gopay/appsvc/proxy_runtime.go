package appsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/hashx"
	"github.com/byte-v-forge/common-lib/httpx"
	"github.com/byte-v-forge/common-lib/randx"
	"github.com/byte-v-forge/common-lib/redactx"
)

type proxyRuntimeLeaseResponse struct {
	Lease struct {
		LeaseID           string               `json:"lease_id"`
		ProviderAccountID string               `json:"provider_account_id"`
		ExpiresAt         string               `json:"expires_at"`
		Session           proxyRuntimeSession  `json:"session"`
		Egress            proxyRuntimeEndpoint `json:"egress"`
	} `json:"lease"`
	Egress proxyRuntimeEndpoint `json:"egress"`
	Pool   struct {
		Endpoints []map[string]any `json:"endpoints"`
	} `json:"pool"`
}

type proxyRuntimeSession struct {
	SessionID  string `json:"session_id"`
	ProviderID string `json:"provider_id"`
}

type proxyRuntimeEndpoint struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

func (s *Server) createProxyRuntimeSession(ctx context.Context, state stateMap) (map[string]any, error) {
	baseURL := proxyRuntimeAPIBase(s.cfg.ProxyRuntimeHTTPAddr)
	if baseURL == "" {
		return nil, nil
	}
	accountID, err := proxyRuntimeAccountID(state)
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	body := []byte(fmt.Sprintf(`{"account_id":%q,"purpose":"gopay_device_proxy","force_new":true,"policy":{"mode":"PROXY_SESSION_MODE_STICKY","region":"ID","sticky_ttl":"600s","labels":{"purpose":"gopay_device_proxy","rotation":"active"},"upstream_kind":"PROXY_UPSTREAM_KIND_DYNAMIC_IP","rotation_mode":"PROXY_ROTATION_MODE_STICKY_SESSION"}}`, accountID))
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/leases/acquire", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy-runtime leases/acquire: %w", err)
	}
	defer resp.Body.Close()
	raw, err := httpx.ReadLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("read proxy-runtime leases/acquire response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("proxy-runtime leases/acquire failed: status %d %s", resp.StatusCode, redactx.Snippet(redactx.Text(string(raw)), 300))
	}

	var parsed proxyRuntimeLeaseResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse proxy-runtime leases/acquire response: %w", err)
	}
	egress := parsed.Egress
	if egress.Host == "" {
		egress = parsed.Lease.Egress
	}
	proxyURL, err := proxyRuntimeProxyURL(egress)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"_gopay_proxy":                      proxyURL,
		"_proxy_runtime_account_id":         accountID,
		"_proxy_runtime_lease_id":           parsed.Lease.LeaseID,
		"_proxy_runtime_provider_account":   parsed.Lease.ProviderAccountID,
		"_proxy_runtime_lease_expires_at":   parsed.Lease.ExpiresAt,
		"_proxy_runtime_session_started_at": time.Now().Unix(),
		"_proxy_runtime_pool_endpoints":     len(parsed.Pool.Endpoints),
		"_proxy_runtime_session_rotated":    true,
	}
	if parsed.Lease.Session.SessionID != "" {
		out["_proxy_runtime_session_hash"] = hashx.ShortSHA256(parsed.Lease.Session.SessionID, 12)
	}
	return out, nil
}

func proxyRuntimeAPIBase(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "/api/proxy-runtime") {
		return value
	}
	return value + "/api/proxy-runtime"
}

func proxyRuntimeAccountID(state stateMap) (string, error) {
	if accountID := stateString(state, "_proxy_runtime_account_id"); accountID != "" {
		return accountID, nil
	}
	suffix, err := randx.Hex(8)
	if err != nil {
		return "", err
	}
	return "gopay-device-" + suffix, nil
}

func proxyRuntimeProxyURL(endpoint proxyRuntimeEndpoint) (string, error) {
	if endpoint.Host == "" || endpoint.Port <= 0 {
		return "", fmt.Errorf("proxy-runtime returned invalid lease egress")
	}
	scheme := "http"
	if endpoint.Protocol == "PROXY_PROTOCOL_SOCKS5" {
		scheme = "socks5"
	}
	return (&url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)}).String(), nil
}
