package app

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/byte-v-forge/gpt/gopay/protocol"
)

const CustomerBaseURL = "https://customer.gopayapi.com"

type Config struct {
	Token        string
	ProxyURL     string
	Timeout      time.Duration
	HMACKey      string
	XE1Override  string
	XE1Marker    string
	HTTPClient   *http.Client
	Device       DeviceFingerprint
	DeviceConfig DeviceConfig
	Logger       protocol.Logger
}

func ConfigFromEnv(token string) Config {
	return Config{
		Token:        token,
		ProxyURL:     os.Getenv("GOPAY_PROXY_URL"),
		HMACKey:      os.Getenv("GOPAY_HMAC_KEY"),
		XE1Override:  os.Getenv("GOPAY_X_E1"),
		XE1Marker:    os.Getenv("GOPAY_X_E1_MARKER"),
		DeviceConfig: DeviceConfigFromEnv(),
	}
}

type Client struct {
	token  string
	device DeviceFingerprint
	http   *protocol.Client
	signer Signer
}

func NewClient(cfg Config) (*Client, error) {
	device := cfg.Device
	if device.AppID == "" {
		var err error
		device, err = NewDeviceFingerprint(cfg.DeviceConfig)
		if err != nil {
			return nil, err
		}
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		var err error
		httpClient, err = protocol.NewHTTPClient(cfg.Timeout, cfg.ProxyURL)
		if err != nil {
			return nil, err
		}
	}
	base, err := protocol.NewClient("", protocol.WithHTTPClient(httpClient), protocol.WithRetry(protocol.RetryPolicy{Attempts: 1}), protocol.WithLogger(cfg.Logger))
	if err != nil {
		return nil, err
	}
	return &Client{
		token:  strings.TrimSpace(cfg.Token),
		device: device,
		http:   base,
		signer: Signer{
			HMACKey:     cfg.HMACKey,
			XE1Override: cfg.XE1Override,
			XE1Marker:   cfg.XE1Marker,
		},
	}, nil
}

func (c *Client) Device() DeviceFingerprint {
	return c.device
}

func (c *Client) Get(ctx context.Context, rawURL string, expected ...int) (*protocol.Response, error) {
	return c.request(ctx, http.MethodGet, rawURL, nil, nil, expected...)
}

func (c *Client) Post(ctx context.Context, rawURL string, body any, expected ...int) (*protocol.Response, error) {
	return c.request(ctx, http.MethodPost, rawURL, body, nil, expected...)
}

func (c *Client) Patch(ctx context.Context, rawURL string, body any, expected ...int) (*protocol.Response, error) {
	return c.request(ctx, http.MethodPatch, rawURL, body, nil, expected...)
}

func (c *Client) Put(ctx context.Context, rawURL string, body any, expected ...int) (*protocol.Response, error) {
	return c.request(ctx, http.MethodPut, rawURL, body, nil, expected...)
}

func (c *Client) Delete(ctx context.Context, rawURL string, body any, expected ...int) (*protocol.Response, error) {
	return c.request(ctx, http.MethodDelete, rawURL, body, nil, expected...)
}

func (c *Client) Request(ctx context.Context, method string, rawURL string, body any, extra http.Header, expected ...int) (*protocol.Response, error) {
	return c.request(ctx, method, rawURL, body, extra, expected...)
}

func (c *Client) request(ctx context.Context, method string, rawURL string, body any, extra http.Header, expected ...int) (*protocol.Response, error) {
	bodyRaw, err := protocol.CompactJSON(body)
	if err != nil {
		return nil, err
	}
	headers, err := c.headers(method, rawURL, bodyRaw, extra)
	if err != nil {
		return nil, err
	}
	return c.http.Do(ctx, protocol.Request{
		Method:       method,
		Path:         rawURL,
		Body:         bodyRaw,
		Headers:      headers,
		Operation:    "gopay-app",
		ExpectStatus: expected,
	})
}

func (c *Client) headers(method string, rawURL string, body []byte, extra http.Header) (http.Header, error) {
	parsed, _ := url.Parse(rawURL)
	path := parsed.Path
	host := strings.ToLower(parsed.Host)
	xM1 := c.device.XM1()
	hasBody := len(body) > 0
	headers := http.Header{}
	setBaseHeaders(headers, c.device, xM1, hasBody)

	if host == "accounts.goto-products.com" {
		headers = appHeaders(c.device, xM1, hasBody)
		headers.Set("X-CVSDK-Version", envDefault("GOPAY_CVSDK_VERSION", defaultCVSDKVersion))
	} else if host == "api.gojekapi.com" && (gojekActivityPaths[path] || gojekAppHeaderPaths[path]) {
		headers = appHeaders(c.device, xM1, hasBody)
	} else if host == "customer.gopayapi.com" && (isGopayCustomerLinkPath(path) || isGopayCustomerAppHeaderPath(path) || (method == http.MethodGet && gopayCustomerSlimGetPaths[path])) {
		headers = appHeaders(c.device, xM1, hasBody)
	} else {
		headers.Set("User-uuid", c.device.UserUUID)
		headers.Set("X-DeviceToken", c.device.DeviceToken)
		headers.Set("X-Location", c.device.Location)
		headers.Set("X-Location-Accuracy", c.device.LocationAccuracy)
		headers.Set("Gojek-Country-Code", c.device.GojekCountryCode)
		headers.Set("X-Dark-Mode", "false")
	}
	if path == "/api/v1/users/pin/tokens" {
		headers.Set("Sdk-Version", c.device.AppVersion)
		headers.Set("X-Biometric", "")
		headers.Set("X-Verification", "PIN")
	}
	if c.token != "" {
		if strings.HasPrefix(c.token, "Bearer ") {
			headers.Set("Authorization", c.token)
		} else {
			headers.Set("Authorization", "Bearer "+c.token)
		}
	}
	for key, values := range extra {
		headers.Del(key)
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	signToken := headers.Get("Authorization")
	if signToken == "" {
		signToken = c.token
	}
	signature, err := c.signer.Sign(method, rawURL, body, signToken, c.device, xM1)
	if err != nil {
		return nil, err
	}
	headers.Set("X-E1", signature.XE1)
	headers.Set("X-E3", signature.BodyMD5)
	return headers, nil
}

func setBaseHeaders(headers http.Header, device DeviceFingerprint, xM1 string, hasBody bool) {
	headers.Set("X-AppVersion", device.AppVersion)
	headers.Set("X-AppId", device.AppID)
	headers.Set("X-AppType", device.AppType)
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", device.UserAgent)
	headers.Set("D1", device.D1)
	headers.Set("X-Session-ID", device.SessionID)
	headers.Set("X-Platform", device.Platform)
	headers.Set("X-UniqueId", device.UniqueID)
	headers.Set("X-User-Type", device.UserType)
	headers.Set("X-DeviceOS", device.DeviceOS)
	headers.Set("X-PhoneMake", device.PhoneMake)
	headers.Set("X-PushTokenType", "FCM")
	headers.Set("X-PhoneModel", device.PhoneModel)
	headers.Set("Accept-Language", defaultAcceptLanguage)
	headers.Set("X-User-Locale", defaultUserLocale)
	headers.Set("X-M1", xM1)
	headers.Set("X-E2", device.XE2)
	headers.Set("AdjTs", device.AdjTS)
	if hasBody {
		headers.Set("Content-Type", "application/json")
	}
}

func appHeaders(device DeviceFingerprint, xM1 string, hasBody bool) http.Header {
	headers := http.Header{}
	headers.Set("Accept-Encoding", "gzip")
	headers.Set("Gojek-Service-Area", "1")
	headers.Set("Country-Code", device.GojekCountryCode)
	headers.Set("X-AppVersion", device.AppVersion)
	headers.Set("X-M1", xM1)
	headers.Set("Gojek-Country-Code", device.GojekCountryCode)
	headers.Set("X-Request-ID", uuid.NewString())
	headers.Set("X-UniqueId", device.UniqueID)
	headers.Set("X-PhoneMake", device.PhoneMake)
	headers.Set("X-Help-Version", device.AppVersion)
	headers.Set("X-Location", device.Location)
	headers.Set("X-Location-Accuracy", device.LocationAccuracy)
	headers.Set("X-DeviceOS", device.DeviceOS)
	headers.Set("X-User-Type", device.UserType)
	headers.Set("User-Agent", device.UserAgent)
	headers.Set("X-AppId", device.AppID)
	headers.Set("Gojek-Timezone", envDefault("GOPAY_TIMEZONE", defaultTimezone))
	headers.Set("X-AuthSDK-Version", envDefault("GOPAY_AUTHSDK_VERSION", defaultAuthSDKVersion))
	headers.Set("X-AppType", device.AppType)
	headers.Set("X-User-Locale", envDefault("GOPAY_USER_LOCALE", defaultUserLocale))
	headers.Set("X-DeviceToken", device.DeviceToken)
	headers.Set("X-E2", device.XE2)
	headers.Set("X-CVSDK-Version", envDefault("GOPAY_CVSDK_VERSION", defaultCVSDKVersion))
	headers.Set("Accept-Language", envDefault("GOPAY_ACCEPT_LANGUAGE", defaultAcceptLanguage))
	headers.Set("Transaction-ID", device.TransactionID)
	headers.Set("X-PhoneModel", device.PhoneModel)
	headers.Set("X-Platform", device.Platform)
	if hasBody {
		headers.Set("Content-Type", "application/json")
	}
	return headers
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

var gopayCustomerSlimGetPaths = map[string]bool{
	"/v1/users/profile":            true,
	"/v1/payment-options/balances": true,
	"/v1/payment-options/profiles": true,
	"/v1/user/wallet-card/balance": true,
}

var gopayCustomerAppHeaderPaths = map[string]bool{
	"/v1/users/profile":                               true,
	"/v1/qris/payments":                               true,
	"/v2/customer/payment-options/checkout/list":      true,
	"/v1/customer/payment-options/settings/last-used": true,
	"/v1/promotions/evaluate":                         true,
	"/api/v1/festival-envelopes/claim":                true,
	"/api/v1/users/deactivate":                        true,
	"/api/v1/users/deactivate/check":                  true,
	"/api/v1/users/pin/challenges":                    true,
	"/api/v1/users/pin/tokens":                        true,
	"/api/v1/users/pin/tokens/nb":                     true,
	"/api/v1/users/pins/allowed":                      true,
	"/api/v2/users/pins/setup/tokens":                 true,
	"/cvs/v1/methods":                                 true,
	"/cvs/v1/initiate":                                true,
	"/cvs/v1/verify":                                  true,
}

var gojekActivityPaths = map[string]bool{
	"/v5/customers": true,
	"/v2/otp/retry": true,
	"/v5/customers/verificationUpdateProfile": true,
	"/gojek/v2/customer":                      true,
}

var gojekAppHeaderPaths = map[string]bool{
	"/courier/v1/token":    true,
	"/v7/customers/signup": true,
}

func isGopayCustomerLinkPath(path string) bool {
	return path == "/v1/linkedapps" || strings.HasPrefix(path, "/v1/links/")
}

func isGopayCustomerAppHeaderPath(path string) bool {
	if gopayCustomerAppHeaderPaths[path] {
		return true
	}
	if path == "/v1/festivals" || strings.HasPrefix(path, "/v1/festivals/") {
		return true
	}
	if strings.HasPrefix(path, "/customers/v1/payments/") {
		return true
	}
	if strings.HasPrefix(path, "/v3/payments/") && strings.HasSuffix(path, "/capture") {
		return true
	}
	if strings.HasPrefix(path, "/api/v2/challenges/") && (strings.HasSuffix(path, "/pin-page") || strings.HasSuffix(path, "/pin-page/nb")) {
		return true
	}
	return false
}
