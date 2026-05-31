package activities

import (
	"strings"

	"orchestrator/pb"
)

type codexOAuthStepData struct {
	message *pb.ActivityCodexOAuthStepData
}

func newCodexOAuthStepData(label string, phone *CodexOAuthPhoneLease) *codexOAuthStepData {
	data := &codexOAuthStepData{message: &pb.ActivityCodexOAuthStepData{}}
	data.setLabel(label)
	data.setPhoneLease(phone)
	return data
}

func (data *codexOAuthStepData) messageData() *pb.ActivityCodexOAuthStepData {
	if data == nil {
		return nil
	}
	return data.message
}

func (data *codexOAuthStepData) setSentinelError(message string) {
	if data != nil && data.message != nil {
		data.message.SentinelError = strings.TrimSpace(message)
	}
}

func (data *codexOAuthStepData) setSentinelFlow(flow string) {
	if data != nil && data.message != nil {
		data.message.SentinelFlow = strings.TrimSpace(flow)
	}
}

func (data *codexOAuthStepData) setSentinelTokenPresent(present bool) {
	if data != nil && data.message != nil {
		data.message.SentinelTokenPresent = boolPtr(present)
	}
}

func (data *codexOAuthStepData) setSentinelTPresent(present bool) {
	if data != nil && data.message != nil {
		data.message.SentinelTPresent = boolPtr(present)
	}
}

func (data *codexOAuthStepData) setWorkspaceSelectError(message string) {
	if data != nil && data.message != nil {
		data.message.WorkspaceSelectError = strings.TrimSpace(message)
	}
}

func (data *codexOAuthStepData) setWorkspaceSelectStatus(status int) {
	if data != nil && data.message != nil {
		data.message.WorkspaceSelectStatus = int32(status)
	}
}

func (data *codexOAuthStepData) setWorkspaceSelected(selected bool) {
	if data != nil && data.message != nil {
		data.message.WorkspaceSelected = boolPtr(selected)
	}
}

func (data *codexOAuthStepData) setChooseAccountError(message string) {
	if data != nil && data.message != nil {
		data.message.ChooseAccountError = strings.TrimSpace(message)
	}
}

func (data *codexOAuthStepData) setChooseAccountStatus(status int) {
	if data != nil && data.message != nil {
		data.message.ChooseAccountStatus = int32(status)
	}
}

func (data *codexOAuthStepData) setChooseAccountSelected(selected bool) {
	if data != nil && data.message != nil {
		data.message.ChooseAccountSelected = boolPtr(selected)
	}
}

func (data *codexOAuthStepData) setLabel(label string) {
	if data != nil {
		data.message.Label = strings.TrimSpace(label)
	}
}

func (data *codexOAuthStepData) setPhoneLease(phone *CodexOAuthPhoneLease) {
	if data == nil || phone == nil {
		return
	}
	data.message.ProfileKey = phone.GetProfileKey()
	data.message.PhoneReused = boolPtr(phone.GetReused())
	data.message.PhoneReuseCount = phone.GetReuseCount()
	data.message.PhoneReuseLimit = phone.GetReuseLimit()
	data.message.PhoneExpiresAtUnix = phone.GetExpiresAtUnix()
	data.message.PhoneActivationId = phone.GetActivationId()
	data.message.PhoneCountryIso2 = phone.GetCountryIso2()
	data.message.PhoneCountryCode = phone.GetCountryCallingCode()
	data.message.PhoneMask = maskPhone(phone.GetPhoneE164(), phone.GetPhoneNational())
	if strings.TrimSpace(phone.GetCountryIso2()) != "" {
		data.message.VerificationChannel = "sms"
	}
}

func (data *codexOAuthStepData) setPhoneAcquireRequest(cfg CodexOAuthConfig, label string, reuseLimit int32) {
	if data == nil {
		return
	}
	data.message.ProfileKey = cfg.PhoneProfileKey
	data.message.CountryIso2 = cfg.PhoneCountryISO2
	data.message.CountryCallingCode = cfg.PhoneCountryCallingCode
	data.message.MaxReuseCount = reuseLimit
	data.setLabel(label)
	data.message.VerificationChannel = "sms"
}

func (data *codexOAuthStepData) setPhoneAcquired(lease *CodexOAuthPhoneLease) {
	if data == nil || lease == nil {
		return
	}
	data.message.ActivationId = lease.GetActivationId()
	data.message.PhoneReused = boolPtr(lease.GetReused())
	data.message.PhoneReuseCount = lease.GetReuseCount()
	data.message.PhoneReuseLimit = lease.GetReuseLimit()
	data.message.PhoneExpiresAtUnix = lease.GetExpiresAtUnix()
	data.message.PhoneMask = maskPhone(lease.GetPhoneE164(), lease.GetPhoneNational())
}

func (data *codexOAuthStepData) setReleaseSkipped(reason string) {
	if data == nil {
		return
	}
	data.message.Released = boolPtr(false)
	data.message.Reason = strings.TrimSpace(reason)
}

func (data *codexOAuthStepData) setReleaseRequested(activationID string, label string) {
	if data == nil {
		return
	}
	data.message.ActivationId = strings.TrimSpace(activationID)
	data.setLabel(label)
	data.message.Released = boolPtr(true)
}

func (data *codexOAuthStepData) setPKCESecretKey(key string) {
	if data != nil {
		data.message.PkceSecretKey = strings.TrimSpace(key)
	}
}

func (data *codexOAuthStepData) setPKCESecretWritten(written bool) {
	if data != nil {
		data.message.PkceSecretWritten = boolPtr(written)
	}
}

func (data *codexOAuthStepData) setBrowserSessionStarted(started bool) {
	if data != nil {
		data.message.BrowserSessionStarted = boolPtr(started)
	}
}

func (data *codexOAuthStepData) setAuthSecretKey(key string) {
	if data != nil {
		data.message.AuthSecretKey = strings.TrimSpace(key)
	}
}

func (data *codexOAuthStepData) setAccountPhoneStatus(status string) {
	if data != nil {
		data.message.AccountPhoneStatus = strings.TrimSpace(status)
	}
}

func (data *codexOAuthStepData) setAccountPhoneNeedWritten(written bool) {
	if data != nil {
		data.message.AccountPhoneNeedWritten = boolPtr(written)
	}
}

func (data *codexOAuthStepData) setAccountPhoneConfirmedWritten(written bool) {
	if data != nil {
		data.message.AccountPhoneConfirmedWritten = boolPtr(written)
	}
}

func (data *codexOAuthStepData) setAddPhonePendingStage(stage string) {
	if data != nil {
		data.message.AddPhonePendingStage = strings.TrimSpace(stage)
	}
}

func (data *codexOAuthStepData) setPhoneResendClickError(err error) {
	if data != nil && err != nil {
		data.message.PhoneResendClickError = err.Error()
	}
}
