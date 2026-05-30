package app

import (
	"time"

	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/natseventbus"
)

type orchestratorConfig struct {
	ListenAddr        string
	DashboardHTTPAddr string
	N8NWebhookBaseURL string

	BrowserAutomationAddr  string
	PaymentAddr            string
	GoPayAppAddr           string
	SmsAddr                string
	GPTAccountAddr         string
	RuntimeSecretRedisURL  string
	OTPRelayRedisURL       string
	RuntimeSecretKeyPrefix string
	RuntimeSecretTTL       time.Duration
	PlatformNATSURL        string
	EventStreamName        string

	CodexOAuthClientID                      string
	CodexOAuthRedirectURI                   string
	CodexOAuthAuthURL                       string
	CodexOAuthTokenURL                      string
	CodexOAuthTokenProxyURL                 string
	CodexOAuthProtocolProxyURL              string
	CodexOAuthProtocolProxyRuntimeHTTPAddr  string
	CodexOAuthProtocolTLSProfile            string
	CodexOAuthProtocolSessionDumpEnabled    bool
	CodexOAuthScope                         string
	CodexOAuthPhoneLabel                    string
	CodexOAuthPhoneProfileKey               string
	CodexOAuthPhoneMaxReuseCount            int
	CodexOAuthPhoneCountryISO2              string
	CodexOAuthPhoneCountryCallingCode       string
	CodexOAuthPhoneWaitSeconds              int32
	CodexOAuthPhoneMinReuseRemainingSeconds int32

	GoPayOTPRelayKeyPrefix  string
	GoPayOTPWebhookTTL      time.Duration
	GoPayOTPWebhookMaxItems int
}

func loadOrchestratorConfig() orchestratorConfig {
	return orchestratorConfig{
		ListenAddr:        envx.StringDefault("LISTEN_ADDR", ":50051"),
		DashboardHTTPAddr: envx.StringDefault("GPT_DASHBOARD_HTTP_ADDR", ":8080"),
		N8NWebhookBaseURL: envx.StringDefault("GPT_N8N_WEBHOOK_BASE_URL", ""),

		BrowserAutomationAddr:  envx.StringDefault("BROWSER_AUTOMATION_ADDR", "browser-automation:50051"),
		PaymentAddr:            envx.StringDefault("GPT_GOPAY_PAYMENT_ADDR", "127.0.0.1:50054"),
		GoPayAppAddr:           envx.StringDefault("GPT_GOPAY_APP_ADDR", "127.0.0.1:50060"),
		SmsAddr:                envx.StringDefault("SMS_ADDR", "sms-service:50051"),
		GPTAccountAddr:         envx.StringDefault("GPT_ACCOUNT_ADDR", "127.0.0.1:50052"),
		RuntimeSecretRedisURL:  envx.StringDefault("GPT_RUNTIME_SECRET_REDIS_URL", ""),
		OTPRelayRedisURL:       envx.StringDefault("GPT_OTP_RELAY_REDIS_URL", ""),
		RuntimeSecretKeyPrefix: envx.StringDefault("GPT_RUNTIME_SECRET_KEY_PREFIX", "byte-v-forge:gpt:runtime-secrets"),
		RuntimeSecretTTL:       envx.PositiveDurationSeconds("GPT_RUNTIME_SECRET_TTL_SECONDS", 24*time.Hour),
		PlatformNATSURL:        envx.StringDefault("PLATFORM_NATS_URL", ""),
		EventStreamName:        envx.StringDefault("PLATFORM_EVENT_STREAM_NAME", natseventbus.DefaultStream),

		CodexOAuthClientID:                      envx.StringDefault("CODEX_OAUTH_CLIENT_ID", "app_EMoamEEZ73f0CkXaXp7hrann"),
		CodexOAuthRedirectURI:                   envx.StringDefault("CODEX_OAUTH_REDIRECT_URI", "http://localhost:1455/auth/callback"),
		CodexOAuthAuthURL:                       envx.StringDefault("CODEX_OAUTH_AUTH_URL", "https://auth.openai.com/oauth/authorize"),
		CodexOAuthTokenURL:                      envx.StringDefault("CODEX_OAUTH_TOKEN_URL", "https://auth.openai.com/oauth/token"),
		CodexOAuthTokenProxyURL:                 envx.StringDefault("CODEX_OAUTH_TOKEN_PROXY_URL", ""),
		CodexOAuthProtocolProxyURL:              envx.StringDefault("CODEX_OAUTH_PROTOCOL_PROXY_URL", ""),
		CodexOAuthProtocolProxyRuntimeHTTPAddr:  envx.StringDefault("CODEX_OAUTH_PROTOCOL_PROXY_RUNTIME_HTTP_ADDR", ""),
		CodexOAuthProtocolTLSProfile:            envx.StringDefault("CODEX_OAUTH_PROTOCOL_TLS_PROFILE", "chrome_146"),
		CodexOAuthProtocolSessionDumpEnabled:    envx.Bool("CODEX_OAUTH_PROTOCOL_SESSION_DUMP_ENABLED", true),
		CodexOAuthScope:                         envx.StringDefault("CODEX_OAUTH_SCOPE", "openid profile email offline_access api.connectors.read api.connectors.invoke"),
		CodexOAuthPhoneLabel:                    envx.StringDefault("CODEX_OAUTH_PHONE_LABEL", "codex-oauth-add-phone"),
		CodexOAuthPhoneProfileKey:               envx.StringDefault("CODEX_OAUTH_PHONE_PROFILE_KEY", "openai-th"),
		CodexOAuthPhoneMaxReuseCount:            envx.PositiveInt("CODEX_OAUTH_PHONE_MAX_REUSE_COUNT", 3),
		CodexOAuthPhoneCountryISO2:              envx.StringDefault("CODEX_OAUTH_PHONE_COUNTRY_ISO2", "TH"),
		CodexOAuthPhoneCountryCallingCode:       envx.StringDefault("CODEX_OAUTH_PHONE_COUNTRY_CALLING_CODE", "66"),
		CodexOAuthPhoneWaitSeconds:              envx.PositiveInt32("CODEX_OAUTH_PHONE_WAIT_SECONDS", 120),
		CodexOAuthPhoneMinReuseRemainingSeconds: envx.PositiveInt32("CODEX_OAUTH_PHONE_MIN_REUSE_REMAINING_SECONDS", 300),

		GoPayOTPRelayKeyPrefix:  envx.StringDefault("GOPAY_OTP_RELAY_KEY_PREFIX", "byte-v-forge:gpt:gopay-otp"),
		GoPayOTPWebhookTTL:      envx.PositiveDurationSeconds("GOPAY_OTP_WEBHOOK_TTL_SECONDS", 10*time.Minute),
		GoPayOTPWebhookMaxItems: envx.PositiveInt("GOPAY_OTP_WEBHOOK_MAX_ITEMS", 100),
	}
}
