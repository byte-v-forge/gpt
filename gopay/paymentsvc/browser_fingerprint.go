package paymentsvc

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/google/uuid"
)

type browserFingerprint struct {
	DeviceID       string
	TLSProfileName string
	TLSProfile     profiles.ClientProfile
	UserAgent      string
	SecCHUA        string
	SecCHPlatform  string
	AcceptLanguage string
	OAILanguage    string
}

type browserFingerprintCandidate struct {
	profileName string
	major       string
	osToken     string
	platform    string
}

var defaultPaymentBrowserFingerprints = []browserFingerprintCandidate{
	{profileName: "chrome_146", major: "146", osToken: "Windows NT 10.0; Win64; x64", platform: "Windows"},
	{profileName: "chrome_146", major: "146", osToken: "Macintosh; Intel Mac OS X 14_6_1", platform: "macOS"},
	{profileName: "chrome_144", major: "144", osToken: "Windows NT 10.0; Win64; x64", platform: "Windows"},
	{profileName: "chrome_144", major: "144", osToken: "Macintosh; Intel Mac OS X 14_5", platform: "macOS"},
	{profileName: "chrome_133", major: "133", osToken: "Windows NT 10.0; Win64; x64", platform: "Windows"},
	{profileName: "chrome_133", major: "133", osToken: "Macintosh; Intel Mac OS X 13_7_2", platform: "macOS"},
	{profileName: "chrome_131", major: "131", osToken: "Windows NT 10.0; Win64; x64", platform: "Windows"},
	{profileName: "chrome_131", major: "131", osToken: "Macintosh; Intel Mac OS X 13_6_7", platform: "macOS"},
}

func randomPaymentBrowserFingerprint(locale string) browserFingerprint {
	candidates := paymentBrowserFingerprintCandidates()
	if len(candidates) == 0 {
		candidates = defaultPaymentBrowserFingerprints
	}
	candidate := candidates[randomInt(len(candidates))]
	profile, ok := profiles.MappedTLSClients[candidate.profileName]
	if !ok {
		candidate = defaultPaymentBrowserFingerprints[0]
		profile = profiles.Chrome_146
	}
	acceptLanguage, oaiLanguage := browserLanguages(locale)
	return browserFingerprint{
		DeviceID:       uuid.NewString(),
		TLSProfileName: candidate.profileName,
		TLSProfile:     profile,
		UserAgent:      fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36", candidate.osToken, candidate.major),
		SecCHUA:        fmt.Sprintf(`"Google Chrome";v="%s", "Not.A/Brand";v="8", "Chromium";v="%s"`, candidate.major, candidate.major),
		SecCHPlatform:  fmt.Sprintf(`"%s"`, candidate.platform),
		AcceptLanguage: acceptLanguage,
		OAILanguage:    oaiLanguage,
	}
}

func browserLanguages(locale string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "zh", "zh-cn", "zh_cn":
		return "zh-CN,zh;q=0.9,en;q=0.8", "zh-CN"
	case "id", "id-id", "id_id":
		return "id-ID,id;q=0.9,en;q=0.8", "id-ID"
	case "en-id", "en_id":
		return "en-ID,en;q=0.9", "en-ID"
	default:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
			return "zh-CN,zh;q=0.9,en;q=0.8", "zh-CN"
		}
		return "en-US,en;q=0.9", "en-US"
	}
}

func paymentBrowserFingerprintCandidates() []browserFingerprintCandidate {
	profileNames := configuredPaymentTLSProfiles()
	if len(profileNames) == 0 {
		return append([]browserFingerprintCandidate(nil), defaultPaymentBrowserFingerprints...)
	}
	var out []browserFingerprintCandidate
	for _, profileName := range profileNames {
		profileName = canonicalPaymentTLSProfile(profileName)
		if profileName == "" {
			continue
		}
		matched := false
		for _, candidate := range defaultPaymentBrowserFingerprints {
			if strings.EqualFold(candidate.profileName, profileName) {
				out = append(out, candidate)
				matched = true
			}
		}
		if !matched {
			major := chromeMajorFromProfile(profileName)
			out = append(out, browserFingerprintCandidate{profileName: profileName, major: major, osToken: "Windows NT 10.0; Win64; x64", platform: "Windows"})
		}
	}
	return out
}

func configuredPaymentTLSProfiles() []string {
	if pinned := strings.TrimSpace(os.Getenv("GOPAY_PAYMENT_TLS_PROFILE")); pinned != "" && !strings.EqualFold(pinned, "random") {
		return []string{pinned}
	}
	raw := strings.TrimSpace(os.Getenv("GOPAY_PAYMENT_TLS_PROFILES"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func canonicalPaymentTLSProfile(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for candidate := range profiles.MappedTLSClients {
		if strings.EqualFold(candidate, name) {
			return candidate
		}
	}
	return ""
}

func chromeMajorFromProfile(profileName string) string {
	parts := strings.Split(profileName, "_")
	for _, part := range parts {
		if len(part) == 3 && strings.Trim(part, "0123456789") == "" {
			return part
		}
	}
	return "146"
}

func (fp browserFingerprint) withFallback(locale string) browserFingerprint {
	if fp.UserAgent != "" && fp.TLSProfileName != "" {
		return fp
	}
	return randomPaymentBrowserFingerprint(locale)
}

func (fp browserFingerprint) applyBrowserHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	if fp.UserAgent != "" {
		headers.Set("User-Agent", fp.UserAgent)
	}
	if fp.AcceptLanguage != "" {
		headers.Set("Accept-Language", fp.AcceptLanguage)
	}
	if fp.SecCHUA != "" {
		headers.Set("sec-ch-ua", fp.SecCHUA)
		headers.Set("sec-ch-ua-mobile", "?0")
	}
	if fp.SecCHPlatform != "" {
		headers.Set("sec-ch-ua-platform", fp.SecCHPlatform)
	}
}

func (fp browserFingerprint) newAttemptHeaders() http.Header {
	headers := http.Header{}
	fp.applyBrowserHeaders(headers)
	if fp.DeviceID != "" {
		headers.Set("x-device-id", fp.DeviceID)
	}
	headers.Set("x-correlation-id", uuid.NewString())
	headers.Set("x-request-id", uuid.NewString())
	return headers
}

func randomInt(size int) int {
	if size <= 1 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(size))
	}
	return int(n.Int64())
}
