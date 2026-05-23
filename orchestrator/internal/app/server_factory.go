package app

import "orchestrator/internal/activities"

func newActivityServer(cfg orchestratorConfig, deps *orchestratorDependencies) *activities.Server {
	return activities.NewServer(activityConfig(cfg, deps))
}

func activityConfig(cfg orchestratorConfig, deps *orchestratorDependencies) activities.Config {
	return activities.Config{
		DB:                      deps.db,
		JobStore:                deps.jobStore,
		AccountClient:           deps.accountClient,
		BrowserAutomationClient: deps.browserAutomationClient,
		PaymentClient:           deps.paymentClient,
		OTPRelay:                deps.otpRelay,
		GoPayClient:             deps.gopayClient,
		SmsClient:               deps.smsClient,
		MailboxClient:           deps.mailboxClient,
		BrowserAuth: activities.BrowserAuthConfig{
			ProxyRef:       cfg.BrowserAuthProxyRef,
			Locale:         cfg.BrowserAuthLocale,
			AcceptLanguage: cfg.BrowserAuthAcceptLanguage,
			Timezone:       cfg.BrowserAuthTimezone,
			UserAgent:      cfg.BrowserAuthUserAgent,
			WindowWidth:    cfg.BrowserAuthWindowWidth,
			WindowHeight:   cfg.BrowserAuthWindowHeight,
			SessionTTL:     cfg.BrowserAuthSessionTTL,
			CommandTimeout: cfg.BrowserAuthCommandTimeout,
			BlockImages:    cfg.BrowserAuthBlockImages,
			GeoIP:          cfg.BrowserAuthGeoIP,
			Humanize:       cfg.BrowserAuthHumanize,
		},
		CodexOAuth: activities.CodexOAuthConfig{
			ClientID:                      cfg.CodexOAuthClientID,
			RedirectURI:                   cfg.CodexOAuthRedirectURI,
			AuthURL:                       cfg.CodexOAuthAuthURL,
			TokenURL:                      cfg.CodexOAuthTokenURL,
			Scope:                         cfg.CodexOAuthScope,
			PhoneLabel:                    cfg.CodexOAuthPhoneLabel,
			PhoneProfileKey:               cfg.CodexOAuthPhoneProfileKey,
			PhoneMaxReuseCount:            cfg.CodexOAuthPhoneMaxReuseCount,
			PhoneCountryISO2:              cfg.CodexOAuthPhoneCountryISO2,
			PhoneCountryCallingCode:       cfg.CodexOAuthPhoneCountryCallingCode,
			PhoneMaxPriceUSD:              cfg.CodexOAuthPhoneMaxPriceUSD,
			PhoneFirstWaitSeconds:         cfg.CodexOAuthPhoneFirstWaitSeconds,
			PhoneResendWaitSeconds:        cfg.CodexOAuthPhoneResendWaitSeconds,
			PhoneMinReuseRemainingSeconds: cfg.CodexOAuthPhoneMinReuseRemainingSeconds,
		},
		OTPTimeout:                     cfg.GoPayOTPTimeout,
		RegistrationOTPTimeout:         cfg.RegistrationOTPWait,
		GoPayAppStepBodyLimit:          cfg.GoPayAppStepBodyLimit,
		GoPayAppLinkPaymentTimeout:     cfg.GoPayAppLinkPaymentTimeout,
		GoPayAppUnlinkTimeout:          cfg.GoPayAppUnlinkTimeout,
		ChangePhoneMaxFailures:         cfg.ChangePhoneMaxFailures,
		ChangePhoneDisabled:            cfg.ChangePhoneDisabled,
		ChangePhoneOTPRetryAttempts:    cfg.ChangePhoneOTPRetryAttempts,
		ChangePhoneGetNumberRetryDelay: cfg.ChangePhoneGetNumberRetryDelay,
	}
}
