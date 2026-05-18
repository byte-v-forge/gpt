package app

import "time"

const (
	defaultChangePhoneMaxFailures            = 3
	defaultChangePhoneOTPRetryAttempts       = 1
	defaultChangePhoneGetNumberRetryDelay    = 5 * time.Second
	defaultChangePhoneSMSCancelTimeout       = 130 * time.Second
	defaultChangePhoneSMSCancelRetryInterval = 10 * time.Second
)
