package paymentsvc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/byte-v-forge/common-lib/stringx"
)

const (
	defaultStripePublishableKey = "pk_live_51HOrSwC6h1nxGoI3lTAgRjYVrz4dU3fVOabyCcKR3pbEJguCVAlqCxdxCUvoRh1XWwRacViovU3kLKvpkjh7IqkW00iXQsjo3n"
	defaultMidtransClientID     = "Mid-client-3TX8nUa-f_RgNrky"
	defaultTokenization         = "true"
	defaultBrowserLocale        = "en-US"
	defaultPINLocale            = "id"
	defaultBrowserPlatform      = "Windows"
	defaultTLSProfile           = "chrome_146"
)

type Config struct {
	ListenAddr           string
	CheckoutProfile      requestProfile
	PaymentProfile       requestProfile
	StripePublishableKey string
	MidtransClientID     string
	Runtime              map[string]string
	CheckoutPlan         map[string]string
	Billing              map[string]string
}

type requestProfile struct {
	Name           string `json:"name"`
	ProxyURL       string `json:"proxy_url"`
	TLSProfile     string `json:"tls_profile"`
	UserAgent      string `json:"user_agent"`
	SecCHUA        string `json:"sec_ch_ua"`
	SecCHPlatform  string `json:"sec_ch_ua_platform"`
	AcceptLanguage string `json:"accept_language"`
	OAILanguage    string `json:"oai_language"`
	Locale         string `json:"locale"`
	DeviceID       string `json:"device_id"`
	Platform       string `json:"platform"`
	PINLocale      string `json:"pin_locale"`
}

func ConfigFromEnv() Config {
	return Config{
		ListenAddr:           stringx.FirstNonEmpty(os.Getenv("GOPAY_PAYMENT_LISTEN_ADDR"), os.Getenv("GPT_GOPAY_PAYMENT_LISTEN_ADDR"), ":50054"),
		CheckoutProfile:      requestProfileFromEnv("GOPAY_CHECKOUT_PROFILE_JSON", defaultRequestProfile("checkout")),
		PaymentProfile:       requestProfileFromEnv("GOPAY_PAYMENT_PROFILE_JSON", defaultRequestProfile("payment")),
		StripePublishableKey: stringx.FirstNonEmpty(os.Getenv("GOPAY_STRIPE_PUBLISHABLE_KEY"), defaultStripePublishableKey),
		MidtransClientID:     stringx.FirstNonEmpty(os.Getenv("GOPAY_MIDTRANS_CLIENT_ID"), defaultMidtransClientID),
		Runtime: map[string]string{
			"version":                         stringx.FirstNonEmpty(os.Getenv("GOPAY_STRIPE_RUNTIME_VERSION"), "fed52f3bc6"),
			"js_checksum":                     strings.TrimSpace(os.Getenv("GOPAY_STRIPE_JS_CHECKSUM")),
			"rv_timestamp":                    strings.TrimSpace(os.Getenv("GOPAY_STRIPE_RV_TIMESTAMP")),
			"expected_amount":                 strings.TrimSpace(os.Getenv("GOPAY_EXPECTED_AMOUNT")),
			"allow_nonzero_expected_amount":   strings.TrimSpace(os.Getenv("GOPAY_ALLOW_NONZERO_EXPECTED_AMOUNT")),
			"fail_on_unknown_expected_amount": strings.TrimSpace(os.Getenv("GOPAY_FAIL_ON_UNKNOWN_EXPECTED_AMOUNT")),
		},
		CheckoutPlan: map[string]string{
			"promo_campaign_id": os.Getenv("GOPAY_PROMO_CAMPAIGN_ID"),
			"entry_point":       os.Getenv("GOPAY_CHECKOUT_ENTRY_POINT"),
			"plan_name":         os.Getenv("GOPAY_CHECKOUT_PLAN_NAME"),
			"billing_country":   os.Getenv("GOPAY_CHECKOUT_BILLING_COUNTRY"),
			"billing_currency":  os.Getenv("GOPAY_CHECKOUT_BILLING_CURRENCY"),
			"checkout_ui_mode":  os.Getenv("GOPAY_CHECKOUT_UI_MODE"),
			"cancel_url":        os.Getenv("GOPAY_CHECKOUT_CANCEL_URL"),
		},
		Billing: map[string]string{
			"name":        os.Getenv("GOPAY_BILLING_NAME"),
			"email":       os.Getenv("GOPAY_BILLING_EMAIL"),
			"country":     os.Getenv("GOPAY_BILLING_COUNTRY"),
			"line1":       os.Getenv("GOPAY_BILLING_LINE1"),
			"city":        os.Getenv("GOPAY_BILLING_CITY"),
			"postal_code": os.Getenv("GOPAY_BILLING_POSTAL_CODE"),
			"state":       os.Getenv("GOPAY_BILLING_STATE"),
		},
	}
}

func defaultRequestProfile(name string) requestProfile {
	return requestProfile{
		Name:       name,
		TLSProfile: defaultTLSProfile,
		Locale:     defaultBrowserLocale,
		Platform:   defaultBrowserPlatform,
		PINLocale:  defaultPINLocale,
	}
}

func requestProfileFromEnv(envName string, fallback requestProfile) requestProfile {
	profile := fallback
	if raw := strings.TrimSpace(os.Getenv(envName)); raw != "" {
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			panic(fmt.Sprintf("invalid %s: %v", envName, err))
		}
	}
	return profile.withDefaults(fallback)
}

func (p requestProfile) withDefaults(fallback requestProfile) requestProfile {
	p.Name = stringx.FirstNonEmpty(p.Name, fallback.Name)
	p.ProxyURL = stringx.FirstNonEmpty(p.ProxyURL, fallback.ProxyURL)
	p.TLSProfile = stringx.FirstNonEmpty(p.TLSProfile, fallback.TLSProfile, defaultTLSProfile)
	p.Locale = stringx.FirstNonEmpty(p.Locale, fallback.Locale, defaultBrowserLocale)
	p.Platform = stringx.FirstNonEmpty(p.Platform, fallback.Platform, defaultBrowserPlatform)
	p.PINLocale = stringx.FirstNonEmpty(p.PINLocale, fallback.PINLocale, defaultPINLocale)
	p.UserAgent = strings.TrimSpace(p.UserAgent)
	p.SecCHUA = strings.TrimSpace(p.SecCHUA)
	p.SecCHPlatform = strings.TrimSpace(p.SecCHPlatform)
	p.AcceptLanguage = strings.TrimSpace(p.AcceptLanguage)
	p.OAILanguage = strings.TrimSpace(p.OAILanguage)
	p.DeviceID = strings.TrimSpace(p.DeviceID)
	return p
}

func (p requestProfile) fingerprint() browserFingerprint {
	return browserFingerprintFromProfile(p)
}

func normalizeListen(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ":50054"
	}
	if strings.HasPrefix(value, ":") {
		return value
	}
	if strings.Contains(value, ":") {
		return value
	}
	return ":" + value
}

func NormalizeListenForMain(value string) string {
	return normalizeListen(value)
}
