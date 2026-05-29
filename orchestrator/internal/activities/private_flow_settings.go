package activities

import (
	"context"

	"orchestrator/internal/gptsettings"
)

const (
	privateFlowsPluginKey = "private_flows"
	browserAuthPluginKey  = "browser_auth"
)

func (s *Server) pluginValues(ctx context.Context, pluginKey string) map[string]string {
	defaults := s.pluginDefaultValues(pluginKey)
	if s.gptSettings == nil {
		return defaults
	}
	settings, err := s.gptSettings.Get(ctx)
	if err != nil {
		return defaults
	}
	return gptsettings.MergePluginValues(defaults, gptsettings.PluginValues(settings, pluginKey))
}

func (s *Server) pluginDefaultValues(pluginKey string) map[string]string {
	if s == nil || s.actionRegistry == nil {
		return nil
	}
	return s.actionRegistry.PluginDefaults(pluginKey)
}

func (s *Server) privateFlowValues(ctx context.Context) map[string]string {
	return s.pluginValues(ctx, privateFlowsPluginKey)
}

func (s *Server) browserAuthValues(ctx context.Context) map[string]string {
	return s.pluginValues(ctx, browserAuthPluginKey)
}

func (s *Server) browserAuthSettings(ctx context.Context) BrowserAuthConfig {
	cfg := s.browserAuthConfig.withDefaults()
	values := s.browserAuthValues(ctx)
	cfg.ProxyRef = gptsettings.StringValue(values, "browser_auth_proxy_ref", cfg.ProxyRef)
	cfg.Locale = gptsettings.StringValue(values, "browser_auth_locale", cfg.Locale)
	cfg.AcceptLanguage = gptsettings.StringValue(values, "browser_auth_accept_language", cfg.AcceptLanguage)
	cfg.Timezone = gptsettings.StringValue(values, "browser_auth_timezone", cfg.Timezone)
	cfg.WindowWidth = gptsettings.IntValue(values, "browser_auth_window_width", cfg.WindowWidth)
	cfg.WindowHeight = gptsettings.IntValue(values, "browser_auth_window_height", cfg.WindowHeight)
	cfg.SessionTTL = gptsettings.DurationSecondsValue(values, "browser_auth_session_ttl_seconds", cfg.SessionTTL)
	cfg.CommandTimeout = gptsettings.DurationSecondsValue(values, "browser_auth_command_timeout_seconds", cfg.CommandTimeout)
	cfg.BlockImages = gptsettings.BoolValue(values, "browser_auth_block_images", cfg.BlockImages)
	cfg.GeoIP = gptsettings.BoolValue(values, "browser_auth_geoip", cfg.GeoIP)
	cfg.Humanize = gptsettings.StringValue(values, "browser_auth_humanize", cfg.Humanize)
	return cfg.withDefaults()
}

func (s *Server) privateFlowRegistrationOTPTimeout(ctx context.Context) int32 {
	return gptsettings.Int32Value(s.privateFlowValues(ctx), "registration_otp_timeout_seconds", 0)
}
