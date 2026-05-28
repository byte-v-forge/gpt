package appsvc

import (
	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/stringx"

	"os"
	"regexp"
	"strings"
	"time"
)

const (
	customerBaseURL = "https://customer.gopayapi.com"
	gojekBaseURL    = "https://api.gojekapi.com"
	gotoAuthBaseURL = "https://accounts.goto-products.com"
	appGoPayHost    = "app.gopay.co.id"
)

type Config struct {
	Port                       string
	StateRedisURL              string
	StateKeyPrefix             string
	StateTTL                   time.Duration
	SignupAuthUUID             string
	PINClientID                string
	GotoClientID               string
	GotoClientSecret           string
	DynamicEgress              []string
	ProxyRuntimeHTTPAddr       string
	SignupInitiateJitterMin    time.Duration
	SignupInitiateJitterMax    time.Duration
	SignupRateLimitCooldown    time.Duration
	OTPTimeout                 time.Duration
	TokenRefreshMinTTL         time.Duration
	ChangePhoneConfirmTimeout  time.Duration
	ChangePhoneConfirmInterval time.Duration
	EnvelopeShortlinkTimeout   time.Duration
	ChangePhoneCountrySync     bool
	MinBalanceRp               int64
}

func ConfigFromEnv() Config {
	return Config{
		Port:                       stringx.FirstNonEmpty(os.Getenv("GOPAY_APP_PORT"), "50051"),
		StateRedisURL:              strings.TrimSpace(os.Getenv("GOPAY_STATE_REDIS_URL")),
		StateKeyPrefix:             stringx.FirstNonEmpty(os.Getenv("GOPAY_STATE_KEY_PREFIX"), "byte-v-forge:gpt:gopay-app-state"),
		StateTTL:                   envx.PositiveDurationSeconds("GOPAY_STATE_TTL_SECONDS", 7*24*time.Hour),
		SignupAuthUUID:             "bb648413-b637-443a-8ebf-176cf9b5dc32",
		PINClientID:                "6d11d261d7ae462dbd4be0dc5f36a697-MFAGOJEK",
		GotoClientID:               "gopay:consumer:app",
		GotoClientSecret:           "raOUumeMRBNifqvZRFjvsgTnjAlaA9",
		OTPTimeout:                 180 * time.Second,
		TokenRefreshMinTTL:         900 * time.Second,
		ChangePhoneConfirmTimeout:  8 * time.Second,
		ChangePhoneConfirmInterval: time.Second,
		EnvelopeShortlinkTimeout:   10 * time.Second,
		DynamicEgress:              splitDynamicEgress(os.Getenv("GOPAY_DYNAMIC_EGRESS")),
		ProxyRuntimeHTTPAddr:       strings.TrimSpace(os.Getenv("PROXY_RUNTIME_HTTP_ADDR")),
		SignupInitiateJitterMin:    envx.NonNegativeDurationSeconds("GOPAY_SIGNUP_INITIATE_JITTER_MIN_SECONDS", 8*time.Second),
		SignupInitiateJitterMax:    envx.NonNegativeDurationSeconds("GOPAY_SIGNUP_INITIATE_JITTER_MAX_SECONDS", 25*time.Second),
		SignupRateLimitCooldown:    envx.NonNegativeDurationSeconds("GOPAY_SIGNUP_RATE_LIMIT_COOLDOWN_SECONDS", 900*time.Second),
		MinBalanceRp:               1,
	}
}

func splitDynamicEgress(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := regexp.MustCompile(`[\s,]+`).Split(raw, -1)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}
