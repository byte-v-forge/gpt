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

	BrowserAuthProxyRef       string
	BrowserAuthLocale         string
	BrowserAuthAcceptLanguage string
	BrowserAuthTimezone       string
	BrowserAuthUserAgent      string
	BrowserAuthWindowWidth    int
	BrowserAuthWindowHeight   int
	BrowserAuthSessionTTL     time.Duration
	BrowserAuthCommandTimeout time.Duration
	BrowserAuthBlockImages    bool
	BrowserAuthGeoIP          bool
	BrowserAuthHumanize       string

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

	GoPayOTPWebhookListenAddr              string
	GoPayOTPRelayKeyPrefix                 string
	GoPayOTPWebhookTTL                     time.Duration
	GoPayOTPWebhookMaxItems                int
	GoPayOTPTimeout                        int32
	RegistrationOTPWait                    int32
	GoPayAppStepBodyLimit                  int32
	GoPayAppLinkPaymentTimeout             time.Duration
	GoPayAppUnlinkTimeout                  time.Duration
	GoPayAddBalanceMode                    string
	GoPayAddBalanceEnvelopeLink            string
	GoPayAddBalanceTransferInstructions    string
	GoPayAddBalanceTransferAmountRp        int64
	GoPayAddBalanceTransferCurrency        string
	GoPayAddBalanceRekberinajaEndpoint     string
	GoPayAddBalanceRekberinajaToken        string
	GoPayAddBalanceRekberinajaDeviceID     string
	GoPayAddBalanceRekberinajaStore        string
	GoPayAddBalanceRekberinajaProductID    string
	GoPayAddBalanceRekberinajaServiceID    string
	GoPayAddBalanceRekberinajaPayment      string
	GoPayAddBalanceRekberinajaEmail        string
	GoPayAddBalanceRekberinajaPromoCode    string
	GoPayAddBalanceRekberinajaUsePoin      bool
	GoPayAddBalanceRekberinajaUserAgent    string
	GoPayAddBalanceRekberinajaOrigin       string
	GoPayAddBalanceRekberinajaReferer      string
	GoPayAddBalanceRekberinajaRefresh      string
	GoPayAddBalanceRekberinajaFeeTotal     int64
	GoPayAddBalanceRekberinajaPollTimeout  int32
	GoPayAddBalanceRekberinajaPollInterval int32
	GoPayAddBalanceConfirmTimeoutSeconds   int32

	ChangePhoneMaxFailures         int
	ChangePhoneDisabled            bool
	ChangePhoneOTPRetryAttempts    int
	ChangePhoneGetNumberRetryDelay time.Duration
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

		BrowserAuthProxyRef:       envx.StringDefault("BROWSER_AUTH_PROXY_REF", "register"),
		BrowserAuthLocale:         envx.StringDefault("BROWSER_AUTH_LOCALE", "en-US"),
		BrowserAuthAcceptLanguage: envx.StringDefault("BROWSER_AUTH_ACCEPT_LANGUAGE", "en-US,en;q=0.9"),
		BrowserAuthTimezone:       envx.StringDefault("BROWSER_AUTH_TIMEZONE", ""),
		BrowserAuthUserAgent:      envx.StringDefault("BROWSER_AUTH_USER_AGENT", ""),
		BrowserAuthWindowWidth:    envx.PositiveInt("BROWSER_AUTH_WINDOW_WIDTH", 1365),
		BrowserAuthWindowHeight:   envx.PositiveInt("BROWSER_AUTH_WINDOW_HEIGHT", 768),
		BrowserAuthSessionTTL:     envx.PositiveDurationSeconds("BROWSER_AUTH_SESSION_TTL_SECONDS", 30*time.Minute),
		BrowserAuthCommandTimeout: envx.PositiveDurationSeconds("BROWSER_AUTH_COMMAND_TIMEOUT_SECONDS", 120*time.Second),
		BrowserAuthBlockImages:    envx.Bool("BROWSER_AUTH_BLOCK_IMAGES", false),
		BrowserAuthGeoIP:          envx.Bool("BROWSER_AUTH_GEOIP", true),
		BrowserAuthHumanize:       envx.StringDefault("BROWSER_AUTH_HUMANIZE", "true"),

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

		GoPayOTPWebhookListenAddr:              envx.StringDefault("GOPAY_OTP_WEBHOOK_LISTEN_ADDR", ":8081"),
		GoPayOTPRelayKeyPrefix:                 envx.StringDefault("GOPAY_OTP_RELAY_KEY_PREFIX", "byte-v-forge:gpt:gopay-otp"),
		GoPayOTPWebhookTTL:                     envx.PositiveDurationSeconds("GOPAY_OTP_WEBHOOK_TTL_SECONDS", 10*time.Minute),
		GoPayOTPWebhookMaxItems:                envx.PositiveInt("GOPAY_OTP_WEBHOOK_MAX_ITEMS", 100),
		GoPayOTPTimeout:                        envx.PositiveInt32("GOPAY_OTP_TIMEOUT_SECONDS", 180),
		RegistrationOTPWait:                    envx.PositiveInt32("REGISTRATION_OTP_TIMEOUT_SECONDS", 180),
		GoPayAppStepBodyLimit:                  int32(envx.PositiveInt("GOPAY_APP_STEP_BODY_LIMIT", 6000)),
		GoPayAppLinkPaymentTimeout:             envx.PositiveDurationSeconds("GOPAY_APP_LINK_PAYMENT_TIMEOUT_SECONDS", 180*time.Second),
		GoPayAppUnlinkTimeout:                  envx.PositiveDurationSeconds("GOPAY_APP_UNLINK_TIMEOUT_SECONDS", 15*time.Second),
		GoPayAddBalanceMode:                    envx.StringDefault("GOPAY_ADD_BALANCE_MODE", "manual_transfer"),
		GoPayAddBalanceEnvelopeLink:            envx.StringDefault("GOPAY_ADD_BALANCE_ENVELOPE_LINK", ""),
		GoPayAddBalanceTransferInstructions:    envx.StringDefault("GOPAY_ADD_BALANCE_TRANSFER_INSTRUCTIONS", ""),
		GoPayAddBalanceTransferAmountRp:        int64(envx.PositiveInt("GOPAY_ADD_BALANCE_TRANSFER_AMOUNT_RP", 1)),
		GoPayAddBalanceTransferCurrency:        envx.StringDefault("GOPAY_ADD_BALANCE_TRANSFER_CURRENCY", "IDR"),
		GoPayAddBalanceRekberinajaEndpoint:     envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_ENDPOINT_URL", "https://api.rekberinaja.com/api/transaction/product/checkout"),
		GoPayAddBalanceRekberinajaToken:        envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_BEARER_TOKEN", ""),
		GoPayAddBalanceRekberinajaDeviceID:     envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_DEVICE_ID", ""),
		GoPayAddBalanceRekberinajaStore:        envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_STORE", "rekberinaja"),
		GoPayAddBalanceRekberinajaProductID:    envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_PRODUCT_ID", ""),
		GoPayAddBalanceRekberinajaServiceID:    envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_SERVICE_ID", ""),
		GoPayAddBalanceRekberinajaPayment:      envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_PAYMENT_METHOD", "saldo"),
		GoPayAddBalanceRekberinajaEmail:        envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_INVOICE_EMAIL", ""),
		GoPayAddBalanceRekberinajaPromoCode:    envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_PROMO_CODE", ""),
		GoPayAddBalanceRekberinajaUsePoin:      envx.Bool("GOPAY_ADD_BALANCE_REKBERINAJA_USE_POIN", false),
		GoPayAddBalanceRekberinajaUserAgent:    envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_USER_AGENT", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"),
		GoPayAddBalanceRekberinajaOrigin:       envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_ORIGIN", "https://rekberinaja.com"),
		GoPayAddBalanceRekberinajaReferer:      envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_REFERER", "https://rekberinaja.com/"),
		GoPayAddBalanceRekberinajaRefresh:      envx.StringDefault("GOPAY_ADD_BALANCE_REKBERINAJA_REFRESH_TOKEN", ""),
		GoPayAddBalanceRekberinajaFeeTotal:     int64(envx.PositiveInt("GOPAY_ADD_BALANCE_REKBERINAJA_FEE_TOTAL_RP", 1300)),
		GoPayAddBalanceRekberinajaPollTimeout:  envx.PositiveInt32("GOPAY_ADD_BALANCE_REKBERINAJA_POLL_TIMEOUT_SECONDS", 180),
		GoPayAddBalanceRekberinajaPollInterval: envx.PositiveInt32("GOPAY_ADD_BALANCE_REKBERINAJA_POLL_INTERVAL_SECONDS", 5),
		GoPayAddBalanceConfirmTimeoutSeconds:   envx.PositiveInt32("GOPAY_ADD_BALANCE_CONFIRM_TIMEOUT_SECONDS", 1800),

		ChangePhoneMaxFailures:         envx.PositiveInt("GOPAY_CHANGE_PHONE_MAX_FAILURES", defaultChangePhoneMaxFailures),
		ChangePhoneDisabled:            envx.Bool("GOPAY_CHANGE_PHONE_DISABLED", false),
		ChangePhoneOTPRetryAttempts:    envx.NonNegativeInt("GOPAY_CHANGE_PHONE_OTP_RETRY_ATTEMPTS", defaultChangePhoneOTPRetryAttempts),
		ChangePhoneGetNumberRetryDelay: envx.NonNegativeDurationSeconds("GOPAY_CHANGE_PHONE_GET_NUMBER_RETRY_SECONDS", defaultChangePhoneGetNumberRetryDelay),
	}
}
