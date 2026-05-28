package paymentsvc

import (
	"fmt"
	"net/http"

	"github.com/byte-v-forge/common-lib/browserfingerprint"
	"github.com/byte-v-forge/common-lib/fingerprinthttp"
	"github.com/byte-v-forge/common-lib/stringx"
	"github.com/google/uuid"
)

type browserFingerprint struct {
	browserfingerprint.Fingerprint
	OAILanguage string
}

type browserFingerprintCandidate = browserfingerprint.ChromiumCandidate

var defaultPaymentBrowserFingerprints = browserfingerprint.DefaultChromiumCandidates()

func stablePaymentBrowserFingerprint(locale, selector, deviceID string) browserFingerprint {
	candidate := selectPaymentBrowserFingerprintCandidate(defaultPaymentBrowserFingerprints, selector)
	if candidate.ProfileName == "" {
		candidate = defaultPaymentBrowserFingerprints[0]
	}
	return buildPaymentBrowserFingerprint(candidate, locale, stringx.FirstNonEmpty(deviceID, stablePaymentDeviceID(candidate)))
}

func randomPaymentBrowserFingerprint(locale string) browserFingerprint {
	candidate := defaultPaymentBrowserFingerprints[browserfingerprint.RandomIndex(len(defaultPaymentBrowserFingerprints))]
	return buildPaymentBrowserFingerprint(candidate, locale, uuid.NewString())
}

func browserFingerprintFromProfile(profile requestProfile) browserFingerprint {
	profile = profile.withDefaults(defaultRequestProfile(profile.Name))
	candidate := selectPaymentBrowserFingerprintCandidate(defaultPaymentBrowserFingerprints, profile.TLSProfile)
	if candidate.ProfileName == "" {
		candidate = defaultPaymentBrowserFingerprints[0]
	}
	fp := buildPaymentBrowserFingerprint(candidate, profile.Locale, stringx.FirstNonEmpty(profile.DeviceID, stableRequestProfileDeviceID(profile, candidate)))
	if profile.UserAgent != "" {
		fp.UserAgent = profile.UserAgent
	}
	if profile.SecCHUA != "" {
		fp.SecCHUA = profile.SecCHUA
	}
	if profile.SecCHPlatform != "" {
		fp.SecCHPlatform = profile.SecCHPlatform
	}
	if profile.AcceptLanguage != "" {
		fp.AcceptLanguage = profile.AcceptLanguage
	}
	if profile.OAILanguage != "" {
		fp.OAILanguage = profile.OAILanguage
	}
	return fp
}

func buildPaymentBrowserFingerprint(candidate browserFingerprintCandidate, locale, deviceID string) browserFingerprint {
	fp := browserfingerprint.BuildChromium(candidate, locale, deviceID)
	return browserFingerprint{Fingerprint: fp, OAILanguage: fp.Language}
}

func selectPaymentBrowserFingerprintCandidate(candidates []browserFingerprintCandidate, selector string) browserFingerprintCandidate {
	candidate, _ := browserfingerprint.SelectChromiumCandidate(candidates, selector)
	return candidate
}

func stablePaymentDeviceID(candidate browserFingerprintCandidate) string {
	seed := fmt.Sprintf("byte-v-forge:gopay-payment:%s:%s:%s", candidate.ProfileName, candidate.MajorVersion, browserfingerprint.OSAlias(candidate))
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String()
}

func stableRequestProfileDeviceID(profile requestProfile, candidate browserFingerprintCandidate) string {
	seed := fmt.Sprintf(
		"byte-v-forge:gopay:%s:%s:%s:%s:%s:%s",
		profile.Name,
		profile.ProxyURL,
		candidate.ProfileName,
		candidate.MajorVersion,
		browserfingerprint.OSAlias(candidate),
		profile.UserAgent,
	)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String()
}

func (fp browserFingerprint) withFallback(locale string) browserFingerprint {
	if fp.UserAgent != "" && fp.TLSProfileName != "" {
		return fp
	}
	return stablePaymentBrowserFingerprint(locale, "", "")
}

func (fp browserFingerprint) applyBrowserHeaders(headers http.Header) {
	fp.Fingerprint.ApplyBrowserHeaders(headers)
}

func (fp browserFingerprint) httpProfile(proxyURL string) fingerprinthttp.Profile {
	fp = fp.withFallback(defaultBrowserLocale)
	return fingerprinthttp.Profile{
		ProxyURL:       proxyURL,
		TLSProfileName: fp.TLSProfileName,
		UserAgent:      fp.UserAgent,
		SecCHUA:        fp.SecCHUA,
		SecCHPlatform:  fp.SecCHPlatform,
		AcceptLanguage: fp.AcceptLanguage,
		Language:       fp.OAILanguage,
		DeviceID:       fp.DeviceID,
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
