package activities

import (
	"time"
)

const (
	browserAuthStageQueued            = "queued"
	browserAuthStageStarting          = "starting"
	browserAuthStageEmailEntry        = "email_entry"
	browserAuthStageCredentialEntry   = "credential_entry"
	browserAuthStageOTPRequestClick   = "otp_request_click"
	browserAuthStageOTPRequestClicked = "otp_request_clicked"
	browserAuthStageWaitingForOTP     = "waiting_for_otp"
	browserAuthStageOTPSubmit         = "otp_submit"
	browserAuthStageSessionCapture    = "session_capture"
	browserAuthStageSucceeded         = "succeeded"
	browserAuthStageFailed            = "failed"
	browserAuthStageCancelled         = "cancelled"
)

type BrowserAuthConfig struct {
	ProxyRef       string
	Locale         string
	AcceptLanguage string
	Timezone       string
	UserAgent      string
	SecCHUA        string
	SecCHPlatform  string
	DeviceID       string
	TLSProfileName string
	WindowWidth    int
	WindowHeight   int
	SessionTTL     time.Duration
	CommandTimeout time.Duration
	BlockImages    bool
	GeoIP          bool
	Humanize       string
}

func (c BrowserAuthConfig) withDefaults() BrowserAuthConfig {
	if c.ProxyRef == "" {
		c.ProxyRef = "register"
	}
	if c.Locale == "" {
		c.Locale = "en-US"
	}
	if c.AcceptLanguage == "" {
		c.AcceptLanguage = "en-US,en;q=0.9"
	}
	if c.WindowWidth < 800 {
		c.WindowWidth = 1365
	}
	if c.WindowHeight < 600 {
		c.WindowHeight = 768
	}
	if c.SessionTTL <= 0 {
		c.SessionTTL = 30 * time.Minute
	}
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = 120 * time.Second
	}
	if c.Humanize == "" {
		c.Humanize = "true"
	}
	return c
}
