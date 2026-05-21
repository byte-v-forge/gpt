package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/google/uuid"
)

const (
	defaultAppVersion      = "2.7.0"
	defaultAppID           = "com.gojek.gopay"
	defaultAppBuild        = "2070"
	defaultGojekCountry    = "ID"
	defaultAuthSDKVersion  = "1.0.0"
	defaultCVSDKVersion    = "1.0.0"
	defaultAcceptLanguage  = "en-ID"
	defaultTimezone        = "Asia/Jakarta"
	defaultUserLocale      = "en_ID"
	defaultLocation        = "-6.2000000,106.8000000"
	defaultLocationAcc     = "0.010999999552965164"
	defaultPlatform        = "Android"
	defaultUserType        = "customer"
	defaultApplicationType = "GOPAY"
)

type DeviceConfig struct {
	StaticIdentity   bool
	AppVersion       string
	AppID            string
	AppBuild         string
	AndroidVersion   string
	UniqueID         string
	SessionID        string
	TransactionID    string
	UserAgent        string
	D1               string
	XE2              string
	AdjTS            string
	AppsFlyerID      string
	WidevineID       string
	Screen           string
	WiFiMAC          string
	WiFiSSID         string
	M1Signature      string
	UserUUID         string
	DeviceToken      string
	Location         string
	LocationAccuracy string
	GojekCountryCode string
}

func DeviceConfigFromEnv() DeviceConfig {
	return DeviceConfig{
		StaticIdentity:   envBool("GOPAY_STATIC_DEVICE_IDENTITY", false),
		AppVersion:       os.Getenv("GOPAY_APP_VERSION"),
		AppID:            os.Getenv("GOPAY_APP_ID"),
		AppBuild:         os.Getenv("GOPAY_APP_BUILD"),
		AndroidVersion:   os.Getenv("GOPAY_ANDROID_VERSION"),
		UniqueID:         os.Getenv("GOPAY_UNIQUE_ID"),
		SessionID:        os.Getenv("GOPAY_SESSION_ID"),
		TransactionID:    os.Getenv("GOPAY_TRANSACTION_ID"),
		UserAgent:        os.Getenv("GOPAY_USER_AGENT"),
		D1:               os.Getenv("GOPAY_D1"),
		XE2:              os.Getenv("GOPAY_X_E2"),
		AdjTS:            os.Getenv("GOPAY_ADJTS"),
		AppsFlyerID:      os.Getenv("GOPAY_APPSFLYER_ID"),
		WidevineID:       os.Getenv("GOPAY_WIDEVINE_ID"),
		Screen:           os.Getenv("GOPAY_SCREEN"),
		WiFiMAC:          os.Getenv("GOPAY_WIFI_MAC"),
		WiFiSSID:         os.Getenv("GOPAY_WIFI_SSID"),
		M1Signature:      os.Getenv("GOPAY_M1_SIGNATURE"),
		UserUUID:         os.Getenv("GOPAY_USER_UUID"),
		DeviceToken:      os.Getenv("GOPAY_DEVICE_TOKEN"),
		Location:         os.Getenv("GOPAY_LOCATION"),
		LocationAccuracy: os.Getenv("GOPAY_LOCATION_ACCURACY"),
		GojekCountryCode: os.Getenv("GOPAY_GOJEK_COUNTRY_CODE"),
	}
}

type DeviceFingerprint struct {
	AppType          string
	AppVersion       string
	AppID            string
	Platform         string
	UniqueID         string
	PhoneMake        string
	PhoneModel       string
	DeviceOS         string
	UserType         string
	SessionID        string
	TransactionID    string
	UserAgent        string
	D1               string
	XE2              string
	AdjTS            string
	AppsFlyerID      string
	WidevineID       string
	Screen           string
	WiFiMAC          string
	WiFiSSID         string
	M1ConnectionID   string
	M1Signature      string
	M1DeviceUUID     string
	UserUUID         string
	DeviceToken      string
	Location         string
	LocationAccuracy string
	GojekCountryCode string
}

func NewDeviceFingerprint(cfg DeviceConfig) (DeviceFingerprint, error) {
	make := randomBrandWord()
	model := randomPhoneModel(make)
	android := firstNonEmpty(cfg.AndroidVersion, fmt.Sprint(randomIntRange(10, 14)))
	deviceOS := android
	if !strings.HasPrefix(strings.ToLower(deviceOS), "android") {
		deviceOS = "Android, " + deviceOS
	}
	appVersion := firstNonEmpty(cfg.AppVersion, defaultAppVersion)
	appID := firstNonEmpty(cfg.AppID, defaultAppID)
	appBuild := firstNonEmpty(cfg.AppBuild, defaultAppBuild)
	d1 := cfg.D1
	if d1 == "" || !cfg.StaticIdentity {
		var err error
		d1, err = randomD1()
		if err != nil {
			return DeviceFingerprint{}, err
		}
	}
	widevine := cfg.WidevineID
	if widevine == "" || !cfg.StaticIdentity {
		var err error
		widevine, err = randomBase64(32)
		if err != nil {
			return DeviceFingerprint{}, err
		}
	}
	uniqueID := cfg.UniqueID
	if uniqueID == "" || !cfg.StaticIdentity {
		uniqueID = randomHex(8)
	}
	sessionID := cfg.SessionID
	if sessionID == "" || !cfg.StaticIdentity {
		sessionID = uuid.NewString()
	}
	transactionID := cfg.TransactionID
	if transactionID == "" || !cfg.StaticIdentity {
		transactionID = uuid.NewString()
	}
	return DeviceFingerprint{
		AppType:          defaultApplicationType,
		AppVersion:       appVersion,
		AppID:            appID,
		Platform:         defaultPlatform,
		UniqueID:         uniqueID,
		PhoneMake:        make,
		PhoneModel:       model,
		DeviceOS:         deviceOS,
		UserType:         defaultUserType,
		SessionID:        sessionID,
		TransactionID:    transactionID,
		UserAgent:        firstNonEmpty(cfg.UserAgent, fmt.Sprintf("GoPay/%s (%s; build:%s; Android, %s)", appVersion, appID, appBuild, android)),
		D1:               d1,
		XE2:              cfg.XE2,
		AdjTS:            firstNonEmpty(cfg.AdjTS, "host:D"),
		AppsFlyerID:      firstNonEmpty(cfg.AppsFlyerID, fmt.Sprintf("%d-%d", unixMillis(), randomInt64Range(1_000_000_000_000_000_000, 9_000_000_000_000_000_000))),
		WidevineID:       widevine,
		Screen:           firstNonEmpty(cfg.Screen, randomScreen()),
		WiFiMAC:          firstNonEmpty(cfg.WiFiMAC, randomWiFiMAC()),
		WiFiSSID:         firstNonEmpty(cfg.WiFiSSID, randomBrandWord()+"_"+randomHex(6)),
		M1ConnectionID:   fmt.Sprint(randomIntRange(100000, 999999)),
		M1Signature:      firstNonEmpty(cfg.M1Signature, randomHex(16)),
		M1DeviceUUID:     uuid.NewString(),
		UserUUID:         cfg.UserUUID,
		DeviceToken:      cfg.DeviceToken,
		Location:         firstNonEmpty(cfg.Location, randomLocation()),
		LocationAccuracy: firstNonEmpty(cfg.LocationAccuracy, randomLocationAccuracy()),
		GojekCountryCode: firstNonEmpty(cfg.GojekCountryCode, defaultGojekCountry),
	}, nil
}

func (d DeviceFingerprint) XM1() string {
	return strings.Join([]string{
		"3:" + firstNonEmpty(d.AppsFlyerID, "0-0"),
		"4:" + firstNonEmpty(d.M1ConnectionID, "5939"),
		"5:" + d.PhoneMake + "|3200|2",
		"6:" + firstNonEmpty(d.WiFiMAC, "02:00:00:00:00:00"),
		"7:" + firstNonEmpty(d.WiFiSSID, "<unknown ssid>"),
		"8:" + firstNonEmpty(d.Screen, "1080x2148"),
		"9:passive,network,fused,gps",
		"10:1",
		"11:" + d.WidevineID,
		"15:" + d.M1Signature,
		"16:" + d.M1DeviceUUID,
	}, ",")
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
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

func randomD1() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(raw))
	for _, b := range raw {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, ":"), nil
}

func randomBase64(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func randomHex(size int) string {
	raw := make([]byte, size)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func randomBrandWord() string {
	consonants := "bcdfghjklmnpqrstvwxyz"
	vowels := "aeiou"
	count := randomIntRange(2, 4)
	var b strings.Builder
	for range count {
		b.WriteByte(consonants[randomIntRange(0, len(consonants)-1)])
		b.WriteByte(vowels[randomIntRange(0, len(vowels)-1)])
	}
	if randomIntRange(0, 99) < 35 {
		suffixes := "nrsx"
		b.WriteByte(suffixes[randomIntRange(0, len(suffixes)-1)])
	}
	word := b.String()
	return strings.ToUpper(word[:1]) + word[1:]
}

func randomPhoneModel(make string) string {
	prefix := strings.ToUpper(lettersOnly(make))
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	if len(prefix) < 2 {
		prefix = randomLetters(2)
	}
	families := "ACMNPRSVXZ"
	separator := "-"
	if randomIntRange(0, 1) == 1 {
		separator = " "
	}
	return fmt.Sprintf("%s, %s%c%s%d", make, prefix, families[randomIntRange(0, len(families)-1)], separator, randomIntRange(1000, 9999))
}

func randomScreen() string {
	width := randomIntRange(45, 90) * 16
	height := width * randomIntRange(195, 228) / 100
	if height < width+640 {
		height = width + 640
	}
	height = (height / 8) * 8
	if height > 3200 {
		height = 3200
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func randomWiFiMAC() string {
	raw := randomHex(5)
	parts := []string{"02"}
	for i := 0; i < len(raw); i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.Join(parts, ":")
}

func randomLocation() string {
	lat := -62_000_000 + randomIntRange(-500_000, 500_000)
	lon := 1_068_000_000 + randomIntRange(-500_000, 500_000)
	return fmt.Sprintf("%.7f,%.7f", float64(lat)/10_000_000, float64(lon)/10_000_000)
}

func randomLocationAccuracy() string {
	return fmt.Sprintf("0.0%d999999552965164", randomIntRange(10, 99))
}

func randomLetters(length int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var out strings.Builder
	for range length {
		out.WriteByte(alphabet[randomIntRange(0, len(alphabet)-1)])
	}
	return out.String()
}

func lettersOnly(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func randomIntRange(minimum, maximum int) int {
	if maximum <= minimum {
		return minimum
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maximum-minimum+1)))
	if err != nil {
		return minimum
	}
	return minimum + int(n.Int64())
}

func randomInt64Range(minimum, maximum int64) int64 {
	if maximum <= minimum {
		return minimum
	}
	n, err := rand.Int(rand.Reader, big.NewInt(maximum-minimum+1))
	if err != nil {
		return minimum
	}
	return minimum + n.Int64()
}
