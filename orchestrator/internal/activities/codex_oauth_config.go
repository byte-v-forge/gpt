package activities

import (
	"strings"
)

const (
	defaultCodexOAuthClientID                = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultCodexOAuthRedirectURI             = "http://localhost:1455/auth/callback"
	defaultCodexOAuthAuthURL                 = "https://auth.openai.com/oauth/authorize"
	defaultCodexOAuthTokenURL                = "https://auth.openai.com/oauth/token"
	defaultCodexOAuthScope                   = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	defaultCodexOAuthPhoneLabel              = "codex-oauth-add-phone"
	defaultCodexOAuthPhoneProfileKey         = "openai-th"
	defaultCodexOAuthPhoneCountryISO2        = "TH"
	defaultCodexOAuthPhoneCountryCallingCode = "66"
	defaultCodexOAuthPhoneMaxPriceUSD        = "0.068"
	defaultCodexOAuthPhoneMaxReuseCount      = 3
	defaultCodexOAuthPhoneWaitSeconds        = 60
	defaultCodexOAuthPhoneMinReuseRemaining  = 180
)

type CodexOAuthConfig struct {
	ClientID                      string
	RedirectURI                   string
	AuthURL                       string
	TokenURL                      string
	TokenProxyURL                 string
	Scope                         string
	PhoneLabel                    string
	PhoneProfileKey               string
	PhoneMaxReuseCount            int
	PhoneCountryISO2              string
	PhoneCountryCallingCode       string
	PhoneMaxPriceUSD              string
	PhoneFirstWaitSeconds         int32
	PhoneResendWaitSeconds        int32
	PhoneMinReuseRemainingSeconds int32
}

func (c CodexOAuthConfig) withDefaults() CodexOAuthConfig {
	if strings.TrimSpace(c.ClientID) == "" {
		c.ClientID = defaultCodexOAuthClientID
	}
	if strings.TrimSpace(c.RedirectURI) == "" {
		c.RedirectURI = defaultCodexOAuthRedirectURI
	}
	if strings.TrimSpace(c.AuthURL) == "" {
		c.AuthURL = defaultCodexOAuthAuthURL
	}
	if strings.TrimSpace(c.TokenURL) == "" {
		c.TokenURL = defaultCodexOAuthTokenURL
	}
	c.TokenProxyURL = strings.TrimSpace(c.TokenProxyURL)
	if strings.TrimSpace(c.Scope) == "" {
		c.Scope = defaultCodexOAuthScope
	}
	if strings.TrimSpace(c.PhoneLabel) == "" {
		c.PhoneLabel = defaultCodexOAuthPhoneLabel
	}
	if strings.TrimSpace(c.PhoneProfileKey) == "" {
		c.PhoneProfileKey = defaultCodexOAuthPhoneProfileKey
	}
	if c.PhoneMaxReuseCount <= 0 {
		c.PhoneMaxReuseCount = defaultCodexOAuthPhoneMaxReuseCount
	}
	if strings.TrimSpace(c.PhoneCountryISO2) == "" {
		c.PhoneCountryISO2 = defaultCodexOAuthPhoneCountryISO2
	}
	c.PhoneCountryISO2 = strings.ToUpper(strings.TrimSpace(c.PhoneCountryISO2))
	if strings.TrimSpace(c.PhoneCountryCallingCode) == "" {
		c.PhoneCountryCallingCode = defaultCodexOAuthPhoneCountryCallingCode
	}
	c.PhoneCountryCallingCode = strings.TrimPrefix(strings.TrimSpace(c.PhoneCountryCallingCode), "+")
	if strings.TrimSpace(c.PhoneMaxPriceUSD) == "" {
		c.PhoneMaxPriceUSD = defaultCodexOAuthPhoneMaxPriceUSD
	}
	if c.PhoneFirstWaitSeconds <= 0 {
		c.PhoneFirstWaitSeconds = defaultCodexOAuthPhoneWaitSeconds
	}
	if c.PhoneResendWaitSeconds <= 0 {
		c.PhoneResendWaitSeconds = defaultCodexOAuthPhoneWaitSeconds
	}
	if c.PhoneMinReuseRemainingSeconds <= 0 {
		c.PhoneMinReuseRemainingSeconds = defaultCodexOAuthPhoneMinReuseRemaining
	}
	return c
}

func (c CodexOAuthConfig) label(override string) string {
	if label := strings.TrimSpace(override); label != "" {
		return label
	}
	return c.withDefaults().PhoneLabel
}
