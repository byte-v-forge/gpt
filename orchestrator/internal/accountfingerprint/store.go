package accountfingerprint

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/browserfingerprint"
	"github.com/byte-v-forge/common-lib/hashx"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"orchestrator/db"
)

const (
	stableLocale         = "en-US"
	stableAcceptLanguage = "en-US,en;q=0.9"
	stableLanguage       = "en-US"
)

type GenerateParams struct {
	CountryCode string
	Region      string
}

type Profile struct {
	AccountID              string
	CountryCode            string
	Region                 string
	BrowserProfileTemplate string
	BrowserFamily          string
	BrowserMajorVersion    string
	OSFamily               string
	TLSProfileFamily       string
	TLSFingerprintVariant  string
	Locale                 string
	Timezone               string
	UserAgent              string
	AcceptLanguage         string
	Language               string
	DeviceID               string
	CreatedAt              int64
	UpdatedAt              int64
}

func (p Profile) Fingerprint() browserfingerprint.Fingerprint {
	p = normalizeStableProfile(p)
	candidate := stableCandidateForProfile(p)
	fingerprint := browserfingerprint.BuildChromium(candidate, p.Locale, p.DeviceID)
	fingerprint.DeviceID = strings.TrimSpace(p.DeviceID)
	fingerprint.TLSProfileName = strings.TrimSpace(p.TLSProfileFamily)
	if tlsProfile, ok := browserfingerprint.TLSProfile(fingerprint.TLSProfileName); ok {
		fingerprint.TLSProfile = tlsProfile
	}
	fingerprint.UserAgent = strings.TrimSpace(p.UserAgent)
	fingerprint.AcceptLanguage = strings.TrimSpace(p.AcceptLanguage)
	fingerprint.Language = strings.TrimSpace(p.Language)
	return fingerprint
}

type Store struct {
	db *gorm.DB
}

func NewStore(database *gorm.DB) *Store {
	return &Store{db: database}
}

func (s *Store) Get(ctx context.Context, accountID string) (Profile, bool, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return Profile{}, false, fmt.Errorf("account_id is required")
	}
	var row db.AccountBrowserFingerprint
	err := s.db.WithContext(ctx).First(&row, "account_id = ?", accountID).Error
	if err == nil {
		profile := profileFromRow(row)
		s.backfillStableFields(ctx, row, profile)
		return profile, true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return Profile{}, false, nil
	}
	return Profile{}, false, err
}

func (s *Store) Generate(ctx context.Context, accountID string, params GenerateParams) (Profile, bool, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return Profile{}, false, fmt.Errorf("account_id is required")
	}
	if profile, ok, err := s.Get(ctx, accountID); err != nil || ok {
		return profile, false, err
	}
	profile := s.Preview(accountID, params)
	now := time.Now().Unix()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	row := rowFromProfile(profile)
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return Profile{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		stored, _, err := s.Get(ctx, accountID)
		return stored, false, err
	}
	return profile, true, nil
}

func (s *Store) Preview(accountID string, params GenerateParams) Profile {
	countryCode, region := normalizeGeo(params)
	locale := stableLocale
	timezone := timezoneForGeo(countryCode, region)
	accountID = strings.TrimSpace(accountID)
	candidate := selectCandidate(accountID)
	deviceID := stableDeviceID(accountID, candidate)
	fingerprint := browserfingerprint.BuildChromium(candidate, locale, deviceID)
	return normalizeStableProfile(Profile{
		AccountID:              accountID,
		CountryCode:            countryCode,
		Region:                 region,
		BrowserProfileTemplate: selectorLabel(candidate),
		BrowserFamily:          "Chrome",
		BrowserMajorVersion:    candidate.MajorVersion,
		OSFamily:               browserfingerprint.OSAlias(candidate),
		TLSProfileFamily:       fingerprint.TLSProfileName,
		TLSFingerprintVariant:  stableTLSFingerprintVariant(accountID, candidate),
		Locale:                 locale,
		Timezone:               timezone,
		UserAgent:              fingerprint.UserAgent,
		AcceptLanguage:         fingerprint.AcceptLanguage,
		Language:               fingerprint.Language,
		DeviceID:               fingerprint.DeviceID,
	})
}

func selectCandidate(accountID string) browserfingerprint.ChromiumCandidate {
	candidates := gptChromiumCandidates()
	if len(candidates) == 0 {
		return browserfingerprint.ChromiumCandidate{}
	}
	return candidates[stableIndex(accountID, len(candidates))]
}

func gptChromiumCandidates() []browserfingerprint.ChromiumCandidate {
	return []browserfingerprint.ChromiumCandidate{
		{ProfileName: "chrome_146", MajorVersion: "146", OSToken: "Macintosh; Intel Mac OS X 14_6_1", Platform: "macOS"},
	}
}

func stableIndex(seed string, size int) int {
	if size <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(seed)))
	return int(h.Sum32() % uint32(size))
}

func stableDeviceID(accountID string, candidate browserfingerprint.ChromiumCandidate) string {
	seed := fmt.Sprintf("byte-v-forge:gpt-account-fingerprint:device:%s:%s:%s:%s", strings.TrimSpace(accountID), candidate.ProfileName, candidate.MajorVersion, browserfingerprint.OSAlias(candidate))
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String()
}

func stableTLSFingerprintVariant(accountID string, candidate browserfingerprint.ChromiumCandidate) string {
	suffix := hashx.ShortSHA256(strings.Join([]string{"byte-v-forge:gpt-account-fingerprint:tls", strings.TrimSpace(accountID), candidate.ProfileName, browserfingerprint.OSAlias(candidate)}, "\x00"), 12)
	return strings.Trim(strings.Join([]string{candidate.ProfileName, browserfingerprint.OSAlias(candidate), suffix}, ":"), ":")
}

func selectorLabel(candidate browserfingerprint.ChromiumCandidate) string {
	return strings.Trim(strings.Join([]string{candidate.ProfileName, browserfingerprint.OSAlias(candidate)}, "_"), "_")
}

func normalizeGeo(params GenerateParams) (string, string) {
	countryCode := normalizeCountryCode(params.CountryCode)
	if countryCode == "" {
		countryCode = "US"
	}
	region := normalizeRegion(countryCode, params.Region)
	if region == "" {
		region = defaultRegion(countryCode)
	}
	return countryCode, region
}

func normalizeCountryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) > 2 {
		return value[:2]
	}
	return value
}

func normalizeRegion(countryCode string, value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || strings.Contains(value, "-") {
		return value
	}
	if len(value) <= 3 {
		countryCode = normalizeCountryCode(countryCode)
		if countryCode != "" {
			return countryCode + "-" + value
		}
	}
	return value
}

func defaultRegion(countryCode string) string {
	switch normalizeCountryCode(countryCode) {
	case "ID":
		return "ID-JK"
	case "TH":
		return "TH-10"
	case "SG":
		return "SG-01"
	case "JP":
		return "JP-13"
	case "US":
		return "US-NY"
	default:
		return normalizeCountryCode(countryCode)
	}
}

func timezoneForGeo(countryCode string, region string) string {
	region = normalizeRegion(countryCode, region)
	switch normalizeCountryCode(countryCode) {
	case "ID":
		return "Asia/Jakarta"
	case "TH":
		return "Asia/Bangkok"
	case "SG":
		return "Asia/Singapore"
	case "JP":
		return "Asia/Tokyo"
	case "US":
		switch region {
		case "US-CA", "CA", "US-WEST":
			return "America/Los_Angeles"
		case "US-TX", "TX", "US-CENTRAL":
			return "America/Chicago"
		default:
			return "America/New_York"
		}
	default:
		return "America/New_York"
	}
}

func normalizeStableProfile(profile Profile) Profile {
	profile.CountryCode = normalizeCountryCode(profile.CountryCode)
	profile.Region = normalizeRegion(profile.CountryCode, profile.Region)
	if profile.CountryCode == "" && strings.Contains(profile.Region, "-") {
		profile.CountryCode, _, _ = strings.Cut(profile.Region, "-")
		profile.CountryCode = normalizeCountryCode(profile.CountryCode)
	}
	if profile.CountryCode == "" || profile.Region == "" {
		if countryCode, region, ok := GeoFromTimezone(profile.Timezone); ok {
			if profile.CountryCode == "" {
				profile.CountryCode = countryCode
			}
			if profile.Region == "" {
				profile.Region = region
			}
		}
	}
	if profile.CountryCode != "" && profile.Region == "" {
		profile.Region = defaultRegion(profile.CountryCode)
	}
	profile.Locale = stableLocale
	profile.AcceptLanguage = stableAcceptLanguage
	profile.Language = stableLanguage
	if strings.TrimSpace(profile.AccountID) != "" {
		candidate := stableCandidateForProfile(profile)
		deviceID := strings.TrimSpace(profile.DeviceID)
		if deviceID == "" {
			deviceID = stableDeviceID(profile.AccountID, candidate)
		}
		fingerprint := browserfingerprint.BuildChromium(candidate, stableLocale, deviceID)
		profile.BrowserProfileTemplate = selectorLabel(candidate)
		profile.BrowserFamily = "Chrome"
		profile.BrowserMajorVersion = candidate.MajorVersion
		profile.OSFamily = browserfingerprint.OSAlias(candidate)
		profile.TLSProfileFamily = fingerprint.TLSProfileName
		profile.TLSFingerprintVariant = stableTLSFingerprintVariant(profile.AccountID, candidate)
		profile.UserAgent = fingerprint.UserAgent
		profile.DeviceID = fingerprint.DeviceID
	}
	return profile
}

func GeoFromTimezone(timezone string) (string, string, bool) {
	switch strings.TrimSpace(timezone) {
	case "Asia/Tokyo":
		return "JP", "JP-13", true
	case "Asia/Jakarta":
		return "ID", "ID-JK", true
	case "Asia/Bangkok":
		return "TH", "TH-10", true
	case "Asia/Singapore":
		return "SG", "SG-01", true
	case "America/Los_Angeles":
		return "US", "US-CA", true
	case "America/Chicago":
		return "US", "US-TX", true
	case "America/New_York":
		return "US", "US-NY", true
	default:
		return "", "", false
	}
}

func stableCandidateForProfile(profile Profile) browserfingerprint.ChromiumCandidate {
	candidates := gptChromiumCandidates()
	if selector := strings.TrimSpace(profile.BrowserProfileTemplate); selector != "" {
		if candidate, ok := browserfingerprint.SelectChromiumCandidate(candidates, selector); ok && candidate.ProfileName != "" {
			return candidate
		}
	}
	if selector := strings.TrimSpace(strings.Join([]string{profile.TLSProfileFamily, profile.OSFamily}, "_")); selector != "_" {
		if candidate, ok := browserfingerprint.SelectChromiumCandidate(candidates, selector); ok && candidate.ProfileName != "" {
			return candidate
		}
	}
	return selectCandidate(profile.AccountID)
}

func (s *Store) backfillStableFields(ctx context.Context, row db.AccountBrowserFingerprint, profile Profile) {
	updates := map[string]any{}
	putUpdate(updates, "browser_profile_template", row.BrowserProfileTemplate, profile.BrowserProfileTemplate)
	putUpdate(updates, "country_code", row.CountryCode, profile.CountryCode)
	putUpdate(updates, "region", row.Region, profile.Region)
	putUpdate(updates, "browser_family", row.BrowserFamily, profile.BrowserFamily)
	putUpdate(updates, "browser_major_version", row.BrowserMajorVersion, profile.BrowserMajorVersion)
	putUpdate(updates, "os_family", row.OSFamily, profile.OSFamily)
	putUpdate(updates, "tls_profile_family", row.TLSProfileFamily, profile.TLSProfileFamily)
	putUpdate(updates, "tls_fingerprint_variant", row.TLSFingerprintVariant, profile.TLSFingerprintVariant)
	putUpdate(updates, "locale", row.Locale, profile.Locale)
	putUpdate(updates, "timezone", row.Timezone, profile.Timezone)
	putUpdate(updates, "user_agent", row.UserAgent, profile.UserAgent)
	putUpdate(updates, "accept_language", row.AcceptLanguage, profile.AcceptLanguage)
	putUpdate(updates, "language", row.Language, profile.Language)
	putUpdate(updates, "device_id", row.DeviceID, profile.DeviceID)
	if len(updates) == 0 {
		return
	}
	updates["updated_at"] = time.Now().Unix()
	_ = s.db.WithContext(ctx).Model(&db.AccountBrowserFingerprint{}).Where("account_id = ?", row.AccountID).Updates(updates).Error
}

func putUpdate(updates map[string]any, key string, current string, next string) {
	if current != next {
		updates[key] = next
	}
}

func profileFromRow(row db.AccountBrowserFingerprint) Profile {
	return normalizeStableProfile(Profile{
		AccountID:              row.AccountID,
		CountryCode:            row.CountryCode,
		Region:                 row.Region,
		BrowserProfileTemplate: row.BrowserProfileTemplate,
		BrowserFamily:          row.BrowserFamily,
		BrowserMajorVersion:    row.BrowserMajorVersion,
		OSFamily:               row.OSFamily,
		TLSProfileFamily:       row.TLSProfileFamily,
		TLSFingerprintVariant:  row.TLSFingerprintVariant,
		Locale:                 row.Locale,
		Timezone:               row.Timezone,
		UserAgent:              row.UserAgent,
		AcceptLanguage:         row.AcceptLanguage,
		Language:               row.Language,
		DeviceID:               row.DeviceID,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	})
}

func rowFromProfile(profile Profile) db.AccountBrowserFingerprint {
	profile = normalizeStableProfile(profile)
	return db.AccountBrowserFingerprint{
		AccountID:              profile.AccountID,
		CountryCode:            profile.CountryCode,
		Region:                 profile.Region,
		BrowserProfileTemplate: profile.BrowserProfileTemplate,
		BrowserFamily:          profile.BrowserFamily,
		BrowserMajorVersion:    profile.BrowserMajorVersion,
		OSFamily:               profile.OSFamily,
		TLSProfileFamily:       profile.TLSProfileFamily,
		TLSFingerprintVariant:  profile.TLSFingerprintVariant,
		Locale:                 profile.Locale,
		Timezone:               profile.Timezone,
		UserAgent:              profile.UserAgent,
		AcceptLanguage:         profile.AcceptLanguage,
		Language:               profile.Language,
		DeviceID:               profile.DeviceID,
		CreatedAt:              profile.CreatedAt,
		UpdatedAt:              profile.UpdatedAt,
	}
}
