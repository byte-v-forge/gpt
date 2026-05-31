package activities

import "strings"

func (data *codexOAuthStepData) setError(err error) {
	if data != nil && err != nil {
		data.message.ErrorMessage = err.Error()
	}
}

func (data *codexOAuthStepData) setDriver(driver string) {
	if data != nil {
		data.message.Driver = strings.TrimSpace(driver)
	}
}

func (data *codexOAuthStepData) setStage(stage string) {
	if data != nil {
		data.message.LoginStage = strings.TrimSpace(stage)
	}
}

func (data *codexOAuthStepData) setFlowID(flowID string) {
	if data != nil {
		data.message.FlowId = strings.TrimSpace(flowID)
	}
}

func (data *codexOAuthStepData) setProtocolSessionStarted(started bool) {
	if data != nil {
		data.message.ProtocolSessionStarted = boolPtr(started)
	}
}

func (data *codexOAuthStepData) setDeviceIDPresent(present bool) {
	if data != nil {
		data.message.DeviceIdPresent = boolPtr(present)
	}
}

func (data *codexOAuthStepData) setEmailOTPIssuedAfter(issuedAfter int64) {
	if data != nil {
		data.message.EmailOtpIssuedAfterUnix = issuedAfter
	}
}

func (data *codexOAuthStepData) setAccountPhoneNeedWriteError(err error) {
	if data != nil && err != nil {
		data.message.AccountPhoneNeedWriteError = err.Error()
	}
}

func (data *codexOAuthStepData) setAddPhoneRequired(required bool) {
	if data != nil {
		data.message.AddPhoneRequired = boolPtr(required)
	}
}

func (data *codexOAuthStepData) setAddPhoneConfirmed(confirmed bool) {
	if data != nil {
		data.message.AddPhoneConfirmed = boolPtr(confirmed)
	}
}

func (data *codexOAuthStepData) setAuthSecretWritten(written bool) {
	if data != nil {
		data.message.AuthSecretWritten = boolPtr(written)
	}
}

func (data *codexOAuthStepData) setAccountAuthWritten(written bool) {
	if data != nil {
		data.message.AccountAuthWritten = boolPtr(written)
	}
}

func (data *codexOAuthStepData) setCallbackURLCaptured(captured bool) {
	if data != nil {
		data.message.CallbackUrlCaptured = boolPtr(captured)
	}
}

func (data *codexOAuthStepData) setPhoneOTPIssuedAfter(issuedAfter int64) {
	if data != nil {
		data.message.PhoneOtpIssuedAfterUnix = issuedAfter
	}
}

func (data *codexOAuthStepData) setPhoneOTPResendIssuedAfter(issuedAfter int64) {
	if data != nil {
		data.message.PhoneOtpResendIssuedAfterUnix = issuedAfter
	}
}

func (data *codexOAuthStepData) setPhoneOTPReceived(received bool) {
	if data != nil {
		data.message.PhoneOtpReceived = boolPtr(received)
	}
}

func (data *codexOAuthStepData) setPhoneValidityConfirmed(confirmed bool) {
	if data != nil {
		data.message.PhoneValidityConfirmed = boolPtr(confirmed)
	}
}

func (data *codexOAuthStepData) setPhoneValidityFailure(failure string) {
	if data != nil {
		data.message.PhoneValidityFailure = strings.TrimSpace(failure)
	}
}

func (data *codexOAuthStepData) setSMSRequestAdditionalError(err error) {
	if data != nil && err != nil {
		data.message.SmsRequestAdditionalError = err.Error()
	}
}

func (data *codexOAuthStepData) setSMSMarkSentError(err error) {
	if data != nil && err != nil {
		data.message.SmsMarkSentError = err.Error()
	}
}

func (data *codexOAuthStepData) setSMSFirstWaitError(err error) {
	if data != nil && err != nil {
		data.message.SmsFirstWaitError = err.Error()
	}
}

func (data *codexOAuthStepData) setSMSResendRequestError(err error) {
	if data != nil && err != nil {
		data.message.SmsResendRequestError = err.Error()
	}
}

func (data *codexOAuthStepData) setPostAddPhoneStage(stage string) {
	if data != nil {
		data.message.PostAddPhoneStage = strings.TrimSpace(stage)
	}
}

func (data *codexOAuthStepData) setClientAuthSessionDumpError(message string) {
	if data != nil {
		data.message.ClientAuthSessionDumpError = strings.TrimSpace(message)
	}
}

func (data *codexOAuthStepData) setClientAuthSessionDumpStatus(status int) {
	if data != nil {
		data.message.ClientAuthSessionDumpStatus = int32(status)
	}
}

func (data *codexOAuthStepData) setClientAuthSessionDumpSeen(seen bool) {
	if data != nil {
		data.message.ClientAuthSessionDumpSeen = boolPtr(seen)
	}
}

func (data *codexOAuthStepData) setClientAuthSessionKeys(keys []string) {
	if data != nil {
		data.message.ClientAuthSessionKeys = keys
	}
}

func (data *codexOAuthStepData) setClientAuthOpenAIClientIDPresent(present bool) {
	if data != nil {
		data.message.ClientAuthOpenaiClientIdPresent = boolPtr(present)
	}
}

func (data *codexOAuthStepData) setClientAuthSessionIDPresent(present bool) {
	if data != nil {
		data.message.ClientAuthSessionIdPresent = boolPtr(present)
	}
}

func (data *codexOAuthStepData) setClientAuthEmailVerificationMode(mode string) {
	if data != nil {
		data.message.ClientAuthEmailVerificationMode = strings.TrimSpace(mode)
	}
}

func (data *codexOAuthStepData) setClientAuthEmailVerified(verified bool) {
	if data != nil {
		data.message.ClientAuthEmailVerified = boolPtr(verified)
	}
}

func (data *codexOAuthStepData) setClientAuthPhoneStateKnown(known bool) {
	if data != nil {
		data.message.ClientAuthPhoneStateKnown = boolPtr(known)
	}
}

func (data *codexOAuthStepData) setClientAuthPhonePresent(present bool) {
	if data != nil {
		data.message.ClientAuthPhonePresent = boolPtr(present)
	}
}

func (data *codexOAuthStepData) setClientAuthPhoneStatus(status string) {
	if data != nil {
		data.message.ClientAuthPhoneStatus = strings.TrimSpace(status)
	}
}

func (data *codexOAuthStepData) setClientAuthPhoneVerificationChannel(channel string) {
	if data != nil {
		data.message.ClientAuthPhoneVerificationChannel = strings.TrimSpace(channel)
	}
}

func (data *codexOAuthStepData) setClientAuthPhoneMask(mask string) {
	if data != nil {
		data.message.ClientAuthPhoneMask = strings.TrimSpace(mask)
	}
}

func (data *codexOAuthStepData) setAddPhoneRequiredSource(source string) {
	if data != nil {
		data.message.AddPhoneRequiredSource = strings.TrimSpace(source)
	}
}
