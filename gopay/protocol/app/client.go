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
		headers = gotoAuthHeaders(c.device, xM1, hasBody)
	} else if host == "api.gojekapi.com" && (gojekActivityPaths[path] || gojekAppHeaderPaths[path]) {
		headers = appHeaders(c.device, xM1, hasBody)
	} else if host == "customer.gopayapi.com" && (isGopayCustomerLinkPath(path) || isGopayCustomerAppHeaderPath(path) || (method == http.MethodGet && gopayCustomerSlimGetPaths[path])) {
		headers = appHeaders(c.device, xM1, hasBody)
	} else {
		setHeader(headers, "User-uuid", c.device.UserUUID)
		setHeader(headers, "X-DeviceToken", c.device.DeviceToken)
		setHeader(headers, "X-Location", c.device.Location)
		setHeader(headers, "X-Location-Accuracy", c.device.LocationAccuracy)
		setHeader(headers, "Gojek-Country-Code", c.device.GojekCountryCode)
		setHeader(headers, "X-Dark-Mode", "false")
	}
	if path == "/api/v1/users/pin/tokens" {
		setHeader(headers, "Sdk-Version", c.device.AppVersion)
		setHeader(headers, "X-Biometric", "")
		setHeader(headers, "X-Verification", "PIN")
	}
	if c.token != "" {
		if strings.HasPrefix(c.token, "Bearer ") {
			setHeader(headers, "Authorization", c.token)
		} else {
			setHeader(headers, "Authorization", "Bearer "+c.token)
		}
	}
	for key, values := range extra {
		setHeaderValues(headers, key, values)
	}
	signToken := headers.Get("Authorization")
	if signToken == "" {
		signToken = c.token
	}
	signature, err := c.signer.Sign(method, rawURL, body, signToken, c.device, xM1)
	if err != nil {
		return nil, err
	}
	setHeader(headers, "X-E1", signature.XE1)
	setHeader(headers, "X-E3", signature.BodyMD5)
	return headers, nil
}

func setHeader(headers http.Header, key, value string) {
	deleteHeader(headers, key)
	headers[key] = []string{value}
}

func setHeaderValues(headers http.Header, key string, values []string) {
	deleteHeader(headers, key)
	headers[key] = append([]string(nil), values...)
}

func deleteHeader(headers http.Header, key string) {
	for existing := range headers {
		if strings.EqualFold(existing, key) {
			delete(headers, existing)
		}
	}
}

func setBaseHeaders(headers http.Header, device DeviceFingerprint, xM1 string, hasBody bool) {
	setHeader(headers, "X-AppVersion", device.AppVersion)
	setHeader(headers, "X-AppId", device.AppID)
	setHeader(headers, "X-AppType", device.AppType)
	setHeader(headers, "Accept", "application/json")
	setHeader(headers, "User-Agent", device.UserAgent)
	setHeader(headers, "D1", device.D1)
	setHeader(headers, "X-Session-ID", device.SessionID)
	setHeader(headers, "X-Platform", device.Platform)
	setHeader(headers, "X-UniqueId", device.UniqueID)
	setHeader(headers, "X-User-Type", device.UserType)
	setHeader(headers, "X-DeviceOS", device.DeviceOS)
	setHeader(headers, "X-PhoneMake", device.PhoneMake)
	setHeader(headers, "X-PushTokenType", "FCM")
	setHeader(headers, "X-PhoneModel", device.PhoneModel)
	setHeader(headers, "Accept-Language", defaultAcceptLanguage)
	setHeader(headers, "X-User-Locale", defaultUserLocale)
	setHeader(headers, "X-M1", xM1)
	setHeader(headers, "X-E2", device.XE2)
	setHeader(headers, "AdjTs", device.AdjTS)
	if hasBody {
		setHeader(headers, "Content-Type", "application/json")
	}
}

func appHeaders(device DeviceFingerprint, xM1 string, hasBody bool) http.Header {
	headers := http.Header{}
	setHeader(headers, "Accept-Encoding", "gzip")
	setHeader(headers, "Gojek-Service-Area", "1")
	setHeader(headers, "Country-Code", device.GojekCountryCode)
	setHeader(headers, "X-AppVersion", device.AppVersion)
	setHeader(headers, "X-M1", xM1)
	setHeader(headers, "Gojek-Country-Code", device.GojekCountryCode)
	setHeader(headers, "X-Request-ID", newTimeUUIDString())
	setHeader(headers, "X-UniqueId", device.UniqueID)
	setHeader(headers, "X-PhoneMake", device.PhoneMake)
	setHeader(headers, "X-Help-Version", device.AppVersion)
	setHeader(headers, "X-Location", device.Location)
	setHeader(headers, "X-Location-Accuracy", device.LocationAccuracy)
	setHeader(headers, "X-DeviceOS", device.DeviceOS)
	setHeader(headers, "X-User-Type", device.UserType)
	setHeader(headers, "User-Agent", device.UserAgent)
	setHeader(headers, "X-AppId", device.AppID)
	setHeader(headers, "Gojek-Timezone", envDefault("GOPAY_TIMEZONE", defaultTimezone))
	setHeader(headers, "X-AuthSDK-Version", envDefault("GOPAY_AUTHSDK_VERSION", defaultAuthSDKVersion))
	setHeader(headers, "X-AppType", device.AppType)
	setHeader(headers, "X-User-Locale", envDefault("GOPAY_USER_LOCALE", defaultUserLocale))
	setHeader(headers, "X-DeviceToken", device.DeviceToken)
	setHeader(headers, "X-E2", device.XE2)
	setHeader(headers, "X-CVSDK-Version", envDefault("GOPAY_CVSDK_VERSION", defaultCVSDKVersion))
	setHeader(headers, "Accept-Language", envDefault("GOPAY_ACCEPT_LANGUAGE", defaultAcceptLanguage))
	setHeader(headers, "Transaction-ID", device.TransactionID)
	setHeader(headers, "X-PhoneModel", device.PhoneModel)
	setHeader(headers, "X-Platform", device.Platform)
	if hasBody {
		setHeader(headers, "Content-Type", "application/json")
	}
	return headers
}

func gotoAuthHeaders(device DeviceFingerprint, xM1 string, hasBody bool) http.Header {
	headers := http.Header{}
	setHeader(headers, "Accept-Encoding", "gzip")
	setHeader(headers, "X-CVSDK-Version", envDefault("GOPAY_CVSDK_VERSION", defaultCVSDKVersion))
	setHeader(headers, "Gojek-Service-Area", "1")
	setHeader(headers, "X-Request-ID", newTimeUUIDString())
	setHeader(headers, "Country-Code", device.GojekCountryCode)
	setHeader(headers, "X-AppVersion", device.AppVersion)
	setHeader(headers, "X-M1", xM1)
	setHeader(headers, "Gojek-Country-Code", device.GojekCountryCode)
	setHeader(headers, "X-UniqueId", device.UniqueID)
	setHeader(headers, "X-PhoneMake", device.PhoneMake)
	setHeader(headers, "X-Help-Version", device.AppVersion)
	setHeader(headers, "User-Agent", device.UserAgent)
	setHeader(headers, "X-DeviceOS", device.DeviceOS)
	setHeader(headers, "X-User-Type", device.UserType)
	setHeader(headers, "X-AppId", device.AppID)
	setHeader(headers, "Gojek-Timezone", envDefault("GOPAY_TIMEZONE", defaultTimezone))
	setHeader(headers, "X-AuthSDK-Version", envDefault("GOPAY_AUTHSDK_VERSION", defaultAuthSDKVersion))
	setHeader(headers, "X-AppType", device.AppType)
	setHeader(headers, "X-User-Locale", envDefault("GOPAY_USER_LOCALE", defaultUserLocale))
	setHeader(headers, "X-DeviceToken", device.DeviceToken)
	setHeader(headers, "X-E2", device.XE2)
	setHeader(headers, "Accept-Language", envDefault("GOPAY_ACCEPT_LANGUAGE", defaultAcceptLanguage))
	setHeader(headers, "Transaction-ID", device.TransactionID)
	setHeader(headers, "X-PhoneModel", device.PhoneModel)
	setHeader(headers, "X-Platform", device.Platform)
	if hasBody {
		setHeader(headers, "Content-Type", "application/json")
	}
	return headers
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func newTimeUUIDString() string {
	value, err := uuid.NewUUID()
	if err == nil {
		return value.String()
	}
	return uuid.NewString()
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
