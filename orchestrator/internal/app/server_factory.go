package app

import "orchestrator/internal/activities"

func newActivityServer(cfg orchestratorConfig, deps *orchestratorDependencies) *activities.Server {
	return activities.NewServer(activityConfig(cfg, deps))
}

func activityConfig(cfg orchestratorConfig, deps *orchestratorDependencies) activities.Config {
	return activities.Config{
		DB:                      deps.db,
		OTPProjection:           deps.otpProjection,
		JobStore:                deps.jobStore,
		RuntimeSecrets:          deps.secrets,
		Fingerprints:            deps.fingerprints,
		AccountClient:           deps.accountClient,
		BrowserAutomationClient: deps.browserAutomationClient,
		PaymentClient:           deps.paymentClient,
		OTPRelay:                deps.otpRelay,
		GoPayClient:             deps.gopayClient,
		SmsClient:               deps.smsClient,
		SmsCatalogClient:        deps.smsCatalogClient,
		MailboxPollRequester:    deps.mailboxPollRequester,
		GPTSettings:             deps.gptSettings,
		ActionRegistry:          deps.actionRegistry,
		BrowserAuth:             activities.BrowserAuthConfig{},
		CodexOAuth: activities.CodexOAuthConfig{
			ClientID:                      cfg.CodexOAuthClientID,
			RedirectURI:                   cfg.CodexOAuthRedirectURI,
			AuthURL:                       cfg.CodexOAuthAuthURL,
			TokenURL:                      cfg.CodexOAuthTokenURL,
			TokenProxyURL:                 cfg.CodexOAuthTokenProxyURL,
			ProtocolProxyURL:              cfg.CodexOAuthProtocolProxyURL,
			ProtocolProxyRuntimeHTTPAddr:  cfg.CodexOAuthProtocolProxyRuntimeHTTPAddr,
			ProtocolTLSProfile:            cfg.CodexOAuthProtocolTLSProfile,
			ProtocolSessionDumpEnabled:    cfg.CodexOAuthProtocolSessionDumpEnabled,
			Scope:                         cfg.CodexOAuthScope,
			PhoneLabel:                    cfg.CodexOAuthPhoneLabel,
			PhoneProfileKey:               cfg.CodexOAuthPhoneProfileKey,
			PhoneMaxReuseCount:            cfg.CodexOAuthPhoneMaxReuseCount,
			PhoneCountryISO2:              cfg.CodexOAuthPhoneCountryISO2,
			PhoneCountryCallingCode:       cfg.CodexOAuthPhoneCountryCallingCode,
			PhoneWaitSeconds:              cfg.CodexOAuthPhoneWaitSeconds,
			PhoneMinReuseRemainingSeconds: cfg.CodexOAuthPhoneMinReuseRemainingSeconds,
		},
	}
}
