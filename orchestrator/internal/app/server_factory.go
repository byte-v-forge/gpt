package app

import "orchestrator/internal/activities"

func newActivityServer(cfg orchestratorConfig, deps *orchestratorDependencies) *activities.Server {
	return activities.NewServer(activityConfig(cfg, deps))
}

func activityConfig(cfg orchestratorConfig, deps *orchestratorDependencies) activities.Config {
	return activities.Config{
		DB:                                deps.db,
		JobStore:                          deps.jobStore,
		AccountClient:                     deps.accountClient,
		BrowserClient:                     deps.browserClient,
		PaymentClient:                     deps.paymentClient,
		GoPayClient:                       deps.gopayClient,
		SmsClient:                         deps.smsClient,
		EmailClient:                       deps.emailClient,
		MailboxRegisterClient:             deps.mailboxRegisterClient,
		OTPAddr:                           cfg.GoPayOTPServiceAddr,
		OTPTimeout:                        cfg.GoPayOTPTimeout,
		RegistrationOTPTimeout:            cfg.RegistrationOTPWait,
		GoPayAppStepBodyLimit:             cfg.GoPayAppStepBodyLimit,
		GoPayAppLinkPaymentTimeout:        cfg.GoPayAppLinkPaymentTimeout,
		GoPayAppUnlinkTimeout:             cfg.GoPayAppUnlinkTimeout,
		ChangePhoneMaxFailures:            cfg.ChangePhoneMaxFailures,
		ChangePhoneDisabled:               cfg.ChangePhoneDisabled,
		ChangePhoneOTPRetryAttempts:       cfg.ChangePhoneOTPRetryAttempts,
		ChangePhoneGetNumberRetryDelay:    cfg.ChangePhoneGetNumberRetryDelay,
		ChangePhoneSMSCancelTimeout:       cfg.ChangePhoneSMSCancelTimeout,
		ChangePhoneSMSCancelRetryInterval: cfg.ChangePhoneSMSCancelRetryInterval,
	}
}
