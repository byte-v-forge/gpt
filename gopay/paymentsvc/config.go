package paymentsvc

import (
	"os"
	"strings"
)

const (
	defaultStripePublishableKey = "pk_live_51HOrSwC6h1nxGoI3lTAgRjYVrz4dU3fVOabyCcKR3pbEJguCVAlqCxdxCUvoRh1XWwRacViovU3kLKvpkjh7IqkW00iXQsjo3n"
	defaultMidtransClientID     = "Mid-client-3TX8nUa-f_RgNrky"
	defaultTokenization         = "true"
	defaultBrowserLocale        = "zh-CN"
	defaultPINLocale            = "id"
	defaultBrowserPlatform      = "Mac OS 10.15.7"
)

type Config struct {
	ListenAddr           string
	CheckoutProxyURL     string
	PaymentProxyURL      string
	StripePublishableKey string
	BrowserLocale        string
	PINLocale            string
	BrowserPlatform      string
	BrowserFingerprint   string
	BrowserDeviceID      string
	MidtransClientID     string
	Runtime              map[string]string
	CheckoutPlan         map[string]string
	Billing              map[string]string
}

func ConfigFromEnv() Config {
	return Config{
		ListenAddr:           firstNonEmpty(os.Getenv("GOPAY_PAYMENT_LISTEN_ADDR"), os.Getenv("GPT_GOPAY_PAYMENT_LISTEN_ADDR"), ":50054"),
		CheckoutProxyURL:     strings.TrimSpace(os.Getenv("GOPAY_CHECKOUT_PROXY_URL")),
		PaymentProxyURL:      strings.TrimSpace(os.Getenv("GOPAY_PAYMENT_PROXY_URL")),
		StripePublishableKey: firstNonEmpty(os.Getenv("GOPAY_STRIPE_PUBLISHABLE_KEY"), defaultStripePublishableKey),
		BrowserLocale:        firstNonEmpty(os.Getenv("GOPAY_BROWSER_LOCALE"), defaultBrowserLocale),
		PINLocale:            firstNonEmpty(os.Getenv("GOPAY_PIN_LOCALE"), defaultPINLocale),
		BrowserPlatform:      firstNonEmpty(os.Getenv("GOPAY_BROWSER_PLATFORM"), defaultBrowserPlatform),
		BrowserFingerprint:   firstNonEmpty(os.Getenv("GOPAY_BROWSER_FINGERPRINT"), os.Getenv("GOPAY_PAYMENT_BROWSER_FINGERPRINT")),
		BrowserDeviceID:      firstNonEmpty(os.Getenv("GOPAY_BROWSER_DEVICE_ID"), os.Getenv("GOPAY_PAYMENT_DEVICE_ID")),
		MidtransClientID:     firstNonEmpty(os.Getenv("GOPAY_MIDTRANS_CLIENT_ID"), defaultMidtransClientID),
		Runtime: map[string]string{
			"version":                         firstNonEmpty(os.Getenv("GOPAY_STRIPE_RUNTIME_VERSION"), "fed52f3bc6"),
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
