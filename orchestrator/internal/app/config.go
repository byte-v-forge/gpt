package app

import (
	"log"
	"os"
	"time"

	workflowruntime "github.com/byte-v-forge/workflow-runtime"
)

type orchestratorConfig struct {
	ListenAddr string

	BrowserAutomationAddr string
	PaymentAddr           string
	GoPayAppAddr          string
	SmsAddr               string
	AccountDBAddr         string
	MailboxAddr           string

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
	CodexOAuthPhoneMaxPriceUSD              string
	CodexOAuthPhoneWaitSeconds              int32
	CodexOAuthPhoneMinReuseRemainingSeconds int32

	GoPayOTPWebhookListenAddr              string
	MailboxWebhookToken                    string
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

	WorkflowRuntime workflowruntime.Config
}

func loadOrchestratorConfig() orchestratorConfig {
	return orchestratorConfig{
		ListenAddr: envDefault("LISTEN_ADDR", ":50051"),

		BrowserAutomationAddr: envDefault("BROWSER_AUTOMATION_ADDR", "browser-automation:50051"),
		PaymentAddr:           envDefault("GPT_GOPAY_PAYMENT_ADDR", "127.0.0.1:50054"),
		GoPayAppAddr:          envDefault("GPT_GOPAY_APP_ADDR", "127.0.0.1:50060"),
		SmsAddr:               envDefault("SMS_ADDR", "sms-service:50051"),
		AccountDBAddr:         envDefault("GPT_ACCOUNT_ADDR", "127.0.0.1:50052"),
		MailboxAddr:           envDefault("MAILBOX_ADDR", "mailbox:50051"),

		BrowserAuthProxyRef:       envDefault("BROWSER_AUTH_PROXY_REF", "register"),
		BrowserAuthLocale:         envDefault("BROWSER_AUTH_LOCALE", "en-US"),
		BrowserAuthAcceptLanguage: envDefault("BROWSER_AUTH_ACCEPT_LANGUAGE", "en-US,en;q=0.9"),
		BrowserAuthTimezone:       envDefault("BROWSER_AUTH_TIMEZONE", ""),
		BrowserAuthUserAgent:      envDefault("BROWSER_AUTH_USER_AGENT", ""),
		BrowserAuthWindowWidth:    envInt("BROWSER_AUTH_WINDOW_WIDTH", 1365),
		BrowserAuthWindowHeight:   envInt("BROWSER_AUTH_WINDOW_HEIGHT", 768),
		BrowserAuthSessionTTL:     envPositiveDurationSeconds("BROWSER_AUTH_SESSION_TTL_SECONDS", 30*time.Minute),
		BrowserAuthCommandTimeout: envPositiveDurationSeconds("BROWSER_AUTH_COMMAND_TIMEOUT_SECONDS", 120*time.Second),
		BrowserAuthBlockImages:    envBool("BROWSER_AUTH_BLOCK_IMAGES", false),
		BrowserAuthGeoIP:          envBool("BROWSER_AUTH_GEOIP", true),
		BrowserAuthHumanize:       envDefault("BROWSER_AUTH_HUMANIZE", "true"),

		CodexOAuthClientID:                      envDefault("CODEX_OAUTH_CLIENT_ID", "app_EMoamEEZ73f0CkXaXp7hrann"),
		CodexOAuthRedirectURI:                   envDefault("CODEX_OAUTH_REDIRECT_URI", "http://localhost:1455/auth/callback"),
		CodexOAuthAuthURL:                       envDefault("CODEX_OAUTH_AUTH_URL", "https://auth.openai.com/oauth/authorize"),
		CodexOAuthTokenURL:                      envDefault("CODEX_OAUTH_TOKEN_URL", "https://auth.openai.com/oauth/token"),
		CodexOAuthTokenProxyURL:                 envDefault("CODEX_OAUTH_TOKEN_PROXY_URL", ""),
		CodexOAuthProtocolProxyURL:              envDefault("CODEX_OAUTH_PROTOCOL_PROXY_URL", ""),
		CodexOAuthProtocolProxyRuntimeHTTPAddr:  envDefault("CODEX_OAUTH_PROTOCOL_PROXY_RUNTIME_HTTP_ADDR", ""),
		CodexOAuthProtocolTLSProfile:            envDefault("CODEX_OAUTH_PROTOCOL_TLS_PROFILE", "chrome_146"),
		CodexOAuthProtocolSessionDumpEnabled:    envBool("CODEX_OAUTH_PROTOCOL_SESSION_DUMP_ENABLED", true),
		CodexOAuthScope:                         envDefault("CODEX_OAUTH_SCOPE", "openid profile email offline_access api.connectors.read api.connectors.invoke"),
		CodexOAuthPhoneLabel:                    envDefault("CODEX_OAUTH_PHONE_LABEL", "codex-oauth-add-phone"),
		CodexOAuthPhoneProfileKey:               envDefault("CODEX_OAUTH_PHONE_PROFILE_KEY", "openai-th"),
		CodexOAuthPhoneMaxReuseCount:            envInt("CODEX_OAUTH_PHONE_MAX_REUSE_COUNT", 3),
		CodexOAuthPhoneCountryISO2:              envDefault("CODEX_OAUTH_PHONE_COUNTRY_ISO2", "TH"),
		CodexOAuthPhoneCountryCallingCode:       envDefault("CODEX_OAUTH_PHONE_COUNTRY_CALLING_CODE", "66"),
		CodexOAuthPhoneMaxPriceUSD:              envDefault("CODEX_OAUTH_PHONE_MAX_PRICE_USD", "0.067"),
		CodexOAuthPhoneWaitSeconds:              envInt32("CODEX_OAUTH_PHONE_WAIT_SECONDS", 120),
		CodexOAuthPhoneMinReuseRemainingSeconds: envInt32("CODEX_OAUTH_PHONE_MIN_REUSE_REMAINING_SECONDS", 300),

		GoPayOTPWebhookListenAddr:              envDefault("GOPAY_OTP_WEBHOOK_LISTEN_ADDR", ":8081"),
		MailboxWebhookToken:                    envDefault("GPT_MAILBOX_WEBHOOK_TOKEN", ""),
		GoPayOTPWebhookTTL:                     envPositiveDurationSeconds("GOPAY_OTP_WEBHOOK_TTL_SECONDS", 10*time.Minute),
		GoPayOTPWebhookMaxItems:                envInt("GOPAY_OTP_WEBHOOK_MAX_ITEMS", 100),
		GoPayOTPTimeout:                        envInt32("GOPAY_OTP_TIMEOUT_SECONDS", 180),
		RegistrationOTPWait:                    envInt32("REGISTRATION_OTP_TIMEOUT_SECONDS", 180),
		GoPayAppStepBodyLimit:                  int32(envInt("GOPAY_APP_STEP_BODY_LIMIT", 6000)),
		GoPayAppLinkPaymentTimeout:             envPositiveDurationSeconds("GOPAY_APP_LINK_PAYMENT_TIMEOUT_SECONDS", 180*time.Second),
		GoPayAppUnlinkTimeout:                  envPositiveDurationSeconds("GOPAY_APP_UNLINK_TIMEOUT_SECONDS", 15*time.Second),
		GoPayAddBalanceMode:                    envDefault("GOPAY_ADD_BALANCE_MODE", "manual_transfer"),
		GoPayAddBalanceEnvelopeLink:            envDefault("GOPAY_ADD_BALANCE_ENVELOPE_LINK", ""),
		GoPayAddBalanceTransferInstructions:    envDefault("GOPAY_ADD_BALANCE_TRANSFER_INSTRUCTIONS", ""),
		GoPayAddBalanceTransferAmountRp:        int64(envInt("GOPAY_ADD_BALANCE_TRANSFER_AMOUNT_RP", 1)),
		GoPayAddBalanceTransferCurrency:        envDefault("GOPAY_ADD_BALANCE_TRANSFER_CURRENCY", "IDR"),
		GoPayAddBalanceRekberinajaEndpoint:     envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_ENDPOINT_URL", "https://api.rekberinaja.com/api/transaction/product/checkout"),
		GoPayAddBalanceRekberinajaToken:        envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_BEARER_TOKEN", ""),
		GoPayAddBalanceRekberinajaDeviceID:     envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_DEVICE_ID", ""),
		GoPayAddBalanceRekberinajaStore:        envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_STORE", "rekberinaja"),
		GoPayAddBalanceRekberinajaProductID:    envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_PRODUCT_ID", ""),
		GoPayAddBalanceRekberinajaServiceID:    envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_SERVICE_ID", ""),
		GoPayAddBalanceRekberinajaPayment:      envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_PAYMENT_METHOD", "saldo"),
		GoPayAddBalanceRekberinajaEmail:        envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_INVOICE_EMAIL", ""),
		GoPayAddBalanceRekberinajaPromoCode:    envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_PROMO_CODE", ""),
		GoPayAddBalanceRekberinajaUsePoin:      envBool("GOPAY_ADD_BALANCE_REKBERINAJA_USE_POIN", false),
		GoPayAddBalanceRekberinajaUserAgent:    envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_USER_AGENT", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"),
		GoPayAddBalanceRekberinajaOrigin:       envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_ORIGIN", "https://rekberinaja.com"),
		GoPayAddBalanceRekberinajaReferer:      envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_REFERER", "https://rekberinaja.com/"),
		GoPayAddBalanceRekberinajaRefresh:      envDefault("GOPAY_ADD_BALANCE_REKBERINAJA_REFRESH_TOKEN", ""),
		GoPayAddBalanceRekberinajaFeeTotal:     int64(envInt("GOPAY_ADD_BALANCE_REKBERINAJA_FEE_TOTAL_RP", 1300)),
		GoPayAddBalanceRekberinajaPollTimeout:  envInt32("GOPAY_ADD_BALANCE_REKBERINAJA_POLL_TIMEOUT_SECONDS", 180),
		GoPayAddBalanceRekberinajaPollInterval: envInt32("GOPAY_ADD_BALANCE_REKBERINAJA_POLL_INTERVAL_SECONDS", 5),
		GoPayAddBalanceConfirmTimeoutSeconds:   envInt32("GOPAY_ADD_BALANCE_CONFIRM_TIMEOUT_SECONDS", 1800),

		ChangePhoneMaxFailures:         envInt("GOPAY_CHANGE_PHONE_MAX_FAILURES", defaultChangePhoneMaxFailures),
		ChangePhoneDisabled:            envBool("GOPAY_CHANGE_PHONE_DISABLED", false),
		ChangePhoneOTPRetryAttempts:    envIntNonNegative("GOPAY_CHANGE_PHONE_OTP_RETRY_ATTEMPTS", defaultChangePhoneOTPRetryAttempts),
		ChangePhoneGetNumberRetryDelay: envNonNegativeDurationSeconds("GOPAY_CHANGE_PHONE_GET_NUMBER_RETRY_SECONDS", defaultChangePhoneGetNumberRetryDelay),

		WorkflowRuntime: loadWorkflowRuntimeConfig(),
	}
}

func loadWorkflowRuntimeConfig() workflowruntime.Config {
	config, err := workflowruntime.LoadConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("load workflow runtime config: %v", err)
	}
	return config
}
