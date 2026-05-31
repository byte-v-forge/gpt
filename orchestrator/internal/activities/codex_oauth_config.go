package activities

import (
	"context"
	"strings"

	"orchestrator/internal/contracts"
	"orchestrator/internal/gptsettings"
)

const (
	defaultCodexOAuthClientID                = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultCodexOAuthRedirectURI             = "http://localhost:1455/auth/callback"
	defaultCodexOAuthAuthURL                 = "https://auth.openai.com/oauth/authorize"
	defaultCodexOAuthTokenURL                = "https://auth.openai.com/oauth/token"
	defaultCodexOAuthScope                   = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	defaultCodexOAuthPhoneProfileKey         = "openai-th"
	defaultCodexOAuthPhoneCountryISO2        = "TH"
	defaultCodexOAuthPhoneCountryCallingCode = "66"
	defaultCodexOAuthPhoneMaxReuseCount      = 3
	defaultCodexOAuthPhoneWaitSeconds        = 120
	defaultCodexOAuthPhoneMinReuseRemaining  = 180
	defaultCodexOAuthProtocolTLSProfile      = "chrome_146"
)

type CodexOAuthConfig struct {
	ClientID                      string
	RedirectURI                   string
	AuthURL                       string
	TokenURL                      string
	TokenProxyURL                 string
	ProtocolProxyURL              string
	ProtocolProxyRuntimeHTTPAddr  string
	ProtocolTLSProfile            string
	ProtocolSessionDumpEnabled    bool
	Scope                         string
	PhoneLabel                    string
	PhoneProfileKey               string
	PhoneMaxReuseCount            int
	PhoneCountryISO2              string
	PhoneCountryCallingCode       string
	PhoneWaitSeconds              int32
	PhoneMinReuseRemainingSeconds int32
}

func (c CodexOAuthConfig) withDefaults() CodexOAuthConfig {
	if strings.TrimSpace(c.ClientID) == "" {
		c.ClientID = defaultCodexOAuthClientID
	}
	if strings.TrimSpace(c.RedirectURI) == "" {
		c.RedirectURI = defaultCodexOAuthRedirectURI
	}
	if strings.TrimSpace(c.AuthURL) == "" {
		c.AuthURL = defaultCodexOAuthAuthURL
	}
	if strings.TrimSpace(c.TokenURL) == "" {
		c.TokenURL = defaultCodexOAuthTokenURL
	}
	c.TokenProxyURL = strings.TrimSpace(c.TokenProxyURL)
	c.ProtocolProxyURL = strings.TrimSpace(c.ProtocolProxyURL)
	c.ProtocolProxyRuntimeHTTPAddr = strings.TrimSpace(c.ProtocolProxyRuntimeHTTPAddr)
	if c.ProtocolProxyURL == "" {
		c.ProtocolProxyURL = c.TokenProxyURL
	}
	if strings.TrimSpace(c.ProtocolTLSProfile) == "" {
		c.ProtocolTLSProfile = defaultCodexOAuthProtocolTLSProfile
	}
	c.ProtocolTLSProfile = strings.TrimSpace(c.ProtocolTLSProfile)
	if strings.TrimSpace(c.Scope) == "" {
		c.Scope = defaultCodexOAuthScope
	}
	if strings.TrimSpace(c.PhoneLabel) == "" {
		c.PhoneLabel = contracts.WorkflowLabelForAction(contracts.ActionCodexOAuthAddPhone)
	}
	if strings.TrimSpace(c.PhoneProfileKey) == "" {
		c.PhoneProfileKey = defaultCodexOAuthPhoneProfileKey
	}
	if c.PhoneMaxReuseCount <= 0 {
		c.PhoneMaxReuseCount = defaultCodexOAuthPhoneMaxReuseCount
	}
	if strings.TrimSpace(c.PhoneCountryISO2) == "" {
		c.PhoneCountryISO2 = defaultCodexOAuthPhoneCountryISO2
	}
	c.PhoneCountryISO2 = strings.ToUpper(strings.TrimSpace(c.PhoneCountryISO2))
	if strings.TrimSpace(c.PhoneCountryCallingCode) == "" {
		c.PhoneCountryCallingCode = defaultCodexOAuthPhoneCountryCallingCode
	}
	c.PhoneCountryCallingCode = strings.TrimPrefix(strings.TrimSpace(c.PhoneCountryCallingCode), "+")
	if c.PhoneWaitSeconds <= 0 {
		c.PhoneWaitSeconds = defaultCodexOAuthPhoneWaitSeconds
	}
	if c.PhoneMinReuseRemainingSeconds <= 0 {
		c.PhoneMinReuseRemainingSeconds = defaultCodexOAuthPhoneMinReuseRemaining
	}
	return c
}

func (s *Server) codexOAuthSettings(ctx context.Context) CodexOAuthConfig {
	cfg := s.codexOAuthConfig.withDefaults()
	values := s.pluginValues(ctx, "codex_oauth")
	cfg.ClientID = gptsettings.StringValue(values, "client_id", cfg.ClientID)
	cfg.RedirectURI = gptsettings.StringValue(values, "redirect_uri", cfg.RedirectURI)
	cfg.AuthURL = gptsettings.StringValue(values, "auth_url", cfg.AuthURL)
	cfg.TokenURL = gptsettings.StringValue(values, "token_url", cfg.TokenURL)
	cfg.TokenProxyURL = gptsettings.StringValue(values, "token_proxy_url", cfg.TokenProxyURL)
	cfg.ProtocolProxyURL = gptsettings.StringValue(values, "protocol_proxy_url", cfg.ProtocolProxyURL)
	cfg.ProtocolProxyRuntimeHTTPAddr = gptsettings.StringValue(values, "protocol_proxy_runtime_http_addr", cfg.ProtocolProxyRuntimeHTTPAddr)
	cfg.ProtocolTLSProfile = gptsettings.StringValue(values, "protocol_tls_profile", cfg.ProtocolTLSProfile)
	cfg.ProtocolSessionDumpEnabled = gptsettings.BoolValue(values, "protocol_session_dump_enabled", cfg.ProtocolSessionDumpEnabled)
	cfg.Scope = gptsettings.StringValue(values, "scope", cfg.Scope)
	cfg.PhoneLabel = gptsettings.StringValue(values, "phone_label", cfg.PhoneLabel)
	cfg.PhoneProfileKey = gptsettings.StringValue(values, "phone_profile_key", cfg.PhoneProfileKey)
	cfg.PhoneMaxReuseCount = gptsettings.IntValue(values, "phone_max_reuse_count", cfg.PhoneMaxReuseCount)
	cfg.PhoneCountryISO2 = gptsettings.StringValue(values, "phone_country_iso2", cfg.PhoneCountryISO2)
	cfg.PhoneCountryCallingCode = gptsettings.StringValue(values, "phone_country_calling_code", cfg.PhoneCountryCallingCode)
	cfg.PhoneWaitSeconds = gptsettings.Int32Value(values, "phone_wait_seconds", cfg.PhoneWaitSeconds)
	cfg.PhoneMinReuseRemainingSeconds = gptsettings.Int32Value(values, "phone_min_reuse_remaining_seconds", cfg.PhoneMinReuseRemainingSeconds)
	return cfg.withDefaults()
}

func (c CodexOAuthConfig) label(override string) string {
	if label := strings.TrimSpace(override); label != "" {
		return label
	}
	return c.withDefaults().PhoneLabel
}
