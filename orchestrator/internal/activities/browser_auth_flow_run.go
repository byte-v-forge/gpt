package activities

import (
	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func (f *browserAuthFlow) run(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) {
	defer f.finish()
	f.setStatus(browserAuthStageStarting, "starting browser session")
	if err := f.startSession(client, cfg); err != nil {
		f.fail(err)
		return
	}
	defer f.stopSession(client)
	if f.mode == browserAuthModeRegister {
		f.runRegister(client, cfg)
		return
	}
	f.runLogin(client, cfg)
}

func (f *browserAuthFlow) runRegister(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) {
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}
	if err := f.openRegisterEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}

	f.setStatus(browserAuthStageEmailEntry, "submitting register email")
	if _, err := f.submitRegisterEmail(client, cfg); err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting register password")
	passwordEntry, err := f.openRegisterPasswordEntry(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	if passwordEntry == "existing_login_password" {
		if err := f.loginExistingAccountFromRegister(client, cfg); err != nil {
			f.fail(err)
		}
		return
	}
	otpIssuedAfter, err := f.submitRegisterPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	f.markOTPRequestClickedAt(unixSecondsFromMillis(otpIssuedAfter))
	otp, err := f.waitForOTP()
	if err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageOTPSubmit, "submitting register OTP")
	if _, err := f.submitRegisterOTP(client, cfg, otp); err != nil {
		f.fail(err)
		return
	}
	if _, err := f.completeRegisterProfile(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if err := f.captureResult(client, cfg, true); err != nil {
		f.fail(err)
		return
	}
}

func (f *browserAuthFlow) runLogin(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) {
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}
	if err := f.openLoginEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		if err := f.captureResult(client, cfg, true); err != nil {
			f.fail(err)
		}
		return
	}
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}

	f.setStatus(browserAuthStageEmailEntry, "submitting login email")
	if _, err := f.submitLoginEmail(client, cfg); err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting login password")
	if err := f.openLoginPasswordEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	state, otpIssuedAfter, err := f.submitLoginPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	if state == "otp_required" {
		f.markOTPRequestClickedAt(unixSecondsFromMillis(otpIssuedAfter))
		otp, err := f.waitForOTP()
		if err != nil {
			f.fail(err)
			return
		}
		f.setStatus(browserAuthStageOTPSubmit, "submitting login OTP")
		if _, err := f.submitLoginOTP(client, cfg, otp); err != nil {
			f.fail(err)
			return
		}
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if err := f.captureResult(client, cfg, true); err != nil {
		f.fail(err)
		return
	}
}
