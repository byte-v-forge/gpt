package protocol

import (
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/browserfingerprint"
	"github.com/byte-v-forge/common-lib/browserhttp"
	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/httpjson"
)

func NewBrowserHTTPClient(timeout time.Duration, proxyRawURL string, tlsProfileName ...string) (httpjson.Doer, error) {
	return browserhttp.New(browserhttp.Config{
		Timeout:                 timeout,
		ProxyURL:                proxyRawURL,
		TLSProfileName:          ResolveTLSProfileName(selectedTLSProfileName(tlsProfileName...)),
		RandomTLSExtensionOrder: envx.Bool("GOPAY_TLS_RANDOM_EXTENSION_ORDER", false),
		DisableHTTP3:            envx.Bool("GOPAY_TLS_DISABLE_HTTP3", true),
		ForceHTTP1:              envx.Bool("GOPAY_TLS_FORCE_HTTP1", false),
		HeaderOrder:             goPayHeaderOrder,
	})
}

func selectedTLSProfileName(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

var defaultAndroidTLSProfileNames = []string{
	"okhttp4_android_10",
	"okhttp4_android_11",
	"okhttp4_android_12",
	"okhttp4_android_13",
	"zalando_android_mobile",
	"nike_android_mobile",
	"confirmed_android",
	"mesh_android",
}

func SelectTLSProfileName() string {
	if profileName := envx.String("GOPAY_TLS_PROFILE"); profileName != "" && !strings.EqualFold(profileName, "random") {
		if canonical := browserfingerprint.CanonicalTLSProfileName(profileName); canonical != "" {
			return canonical
		}
	}
	if profilesFromEnv := tlsProfilesFromEnv(); len(profilesFromEnv) > 0 {
		return browserfingerprint.RandomTLSProfileName(profilesFromEnv)
	}
	return browserfingerprint.RandomTLSProfileName(defaultAndroidTLSProfileNames)
}

func ResolveTLSProfileName(profileName string) string {
	profileName = strings.TrimSpace(profileName)
	if profileName != "" && !strings.EqualFold(profileName, "random") {
		if canonical := browserfingerprint.CanonicalTLSProfileName(profileName); canonical != "" {
			return canonical
		}
	}
	return SelectTLSProfileName()
}

func tlsProfilesFromEnv() []string {
	return browserfingerprint.CanonicalTLSProfileNames(envx.List("GOPAY_TLS_PROFILES"))
}

func goPayHeaderOrder(headers http.Header, host string) ([]string, []string) {
	if !isGoPaySignedHeader(headers) {
		return nil, nil
	}
	return gopaySignedHeaderOrder(headers, host), []string{":method", ":authority", ":scheme", ":path"}
}

func isGoPaySignedHeader(headers http.Header) bool {
	return headerValue(headers, "x-e1") != "" && strings.EqualFold(headerValue(headers, "x-appid"), "com.gojek.gopay")
}

func headerValue(headers http.Header, key string) string {
	for existing, values := range headers {
		if !strings.EqualFold(existing, key) {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
		return ""
	}
	return ""
}

func gopaySignedHeaderOrder(headers http.Header, host string) []string {
	if strings.TrimSpace(host) == "" {
		host = headerValue(headers, "host")
	}
	if strings.EqualFold(host, "accounts.goto-products.com") {
		return []string{
			"accept-encoding",
			"key",
			"x-cvsdk-version",
			"authorization",
			"verification-token",
			"is-token-required",
			"gojek-service-area",
			"x-request-id",
			"country-code",
			"x-appversion",
			"content-length",
			"x-m1",
			"gojek-country-code",
			"x-uniqueid",
			"x-phonemake",
			"x-help-version",
			"x-e1",
			"user-agent",
			"x-deviceos",
			"x-user-type",
			"x-appid",
			"gojek-timezone",
			"content-type",
			"x-authsdk-version",
			"x-apptype",
			"x-user-locale",
			"x-devicetoken",
			"x-e2",
			"accept-language",
			"host",
			"transaction-id",
			"x-phonemodel",
			"x-platform",
		}
	}
	return []string{
		"accept-encoding",
		"country-code",
		"gojek-country-code",
		"gojek-service-area",
		"x-appversion",
		"x-help-version",
		"x-location",
		"x-location-accuracy",
		"x-uniqueid",
		"x-phonemake",
		"x-phonemodel",
		"x-deviceos",
		"x-user-type",
		"x-appid",
		"gojek-timezone",
		"x-apptype",
		"x-user-locale",
		"accept-language",
		"x-platform",
		"user-agent",
		"content-type",
		"x-m1",
		"x-e2",
		"x-authsdk-version",
		"x-cvsdk-version",
		"authorization",
		"x-request-id",
		"transaction-id",
		"verification-token",
		"is-token-required",
		"x-e1",
	}
}
