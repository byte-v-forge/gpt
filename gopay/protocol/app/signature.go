package app

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const emptyBodyMD5 = "d41d8cd98f00b204e9800998ecf8427e"

type Signer struct {
	HMACKey     string
	XE1Override string
	XE1Marker   string
	Now         func() time.Time
}

type Signature struct {
	XE1     string
	BodyMD5 string
}

func (s Signer) Sign(method string, rawURL string, body []byte, token string, device DeviceFingerprint, xM1 string) (Signature, error) {
	bodyMD5 := emptyBodyMD5
	if len(body) > 0 {
		sum := md5.Sum(body)
		bodyMD5 = hex.EncodeToString(sum[:])
	}
	if strings.TrimSpace(s.XE1Override) != "" {
		return Signature{XE1: strings.TrimSpace(s.XE1Override), BodyMD5: bodyMD5}, nil
	}
	hmacKey := strings.TrimSpace(s.HMACKey)
	if hmacKey == "" {
		return Signature{}, fmt.Errorf("GOPAY_HMAC_KEY is required")
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	field1, err := randomField1()
	if err != nil {
		return Signature{}, err
	}
	timestamp := fmt.Sprint(now.UnixMilli())
	path := signaturePath(rawURL)
	jwt := strings.TrimPrefix(token, "Bearer ")
	if xM1 == "" {
		xM1 = device.XM1()
	}
	parts := []string{
		device.AppType,
		device.PhoneModel + ":" + jwt,
		device.UniqueID + ":",
		bodyMD5 + ":" + path,
		strings.ToUpper(method) + ":" + timestamp,
		device.DeviceOS + ":" + device.AppVersion,
		xM1 + ":" + device.AppID,
		field1 + ":" + device.PhoneMake,
		device.Platform,
	}
	msg := strings.Join(parts, ";")
	mac := hmac.New(sha256.New, []byte(hmacKey))
	_, _ = mac.Write([]byte(msg))
	marker := strings.TrimSpace(s.XE1Marker)
	if marker == "" {
		marker = "D"
	}
	return Signature{
		XE1:     hex.EncodeToString(mac.Sum(nil)) + ":" + field1 + ":" + marker + ":" + timestamp,
		BodyMD5: bodyMD5,
	}, nil
}

func signaturePath(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	}
	return parsed.Host + parsed.RequestURI()
}

func randomField1() (string, error) {
	first := make([]byte, 32)
	second := make([]byte, 16)
	if _, err := rand.Read(first); err != nil {
		return "", err
	}
	if _, err := rand.Read(second); err != nil {
		return "", err
	}
	return hex.EncodeToString(first) + strings.Repeat("0", 64) + hex.EncodeToString(second), nil
}
