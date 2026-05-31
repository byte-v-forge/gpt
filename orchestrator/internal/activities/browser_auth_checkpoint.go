package activities

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"

	"orchestrator/internal/channelotpwait"
)

const (
	browserAuthOTPKindParam         = "browser_auth_otp_kind"
	browserAuthOTPKindRegisterEmail = "register_email"
	browserAuthOTPKindLoginEmail    = "login_email"
)

func (f *browserAuthFlow) runUntilCheckpoint(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	f.setStatus(browserAuthStageStarting, "starting browser session")
	if err := f.startSession(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	f.promoteFlowIDToSessionID()
	if f.mode == browserAuthModeRegister {
		return f.runRegisterUntilCheckpoint(client, cfg)
	}
	return f.runLoginUntilCheckpoint(client, cfg)
}

func (f *browserAuthFlow) runRegisterUntilCheckpoint(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	if f.cancelled() {
		return fmt.Errorf("browser auth cancelled")
	}
	if err := f.openRegisterEntry(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	f.setStatus(browserAuthStageEmailEntry, "submitting register email")
	if _, err := f.submitRegisterEmail(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting register password")
	passwordEntry, err := f.openRegisterPasswordEntry(client, cfg)
	if err != nil {
		f.fail(err)
		return err
	}
	if passwordEntry == "existing_login_password" {
		return f.runExistingLoginFromRegisterUntilCheckpoint(client, cfg)
	}
	otpIssuedAfter, err := f.submitRegisterPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return err
	}
	f.markWaitingForOTP(browserAuthOTPKindRegisterEmail, unixSecondsFromMillis(otpIssuedAfter))
	return nil
}

func (f *browserAuthFlow) runExistingLoginFromRegisterUntilCheckpoint(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	f.setStatus(browserAuthStageCredentialEntry, "logging in existing account")
	state, otpIssuedAfter, err := f.submitLoginPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return err
	}
	if state == "otp_required" {
		f.markWaitingForOTP(browserAuthOTPKindLoginEmail, unixSecondsFromMillis(otpIssuedAfter))
		return nil
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	if err := f.captureResult(client, cfg, true); err != nil {
		f.fail(err)
		return err
	}
	return nil
}

func (f *browserAuthFlow) runLoginUntilCheckpoint(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	if f.cancelled() {
		return fmt.Errorf("browser auth cancelled")
	}
	if err := f.openLoginEntry(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		if err := f.captureResult(client, cfg, true); err != nil {
			f.fail(err)
			return err
		}
		return nil
	}
	f.setStatus(browserAuthStageEmailEntry, "submitting login email")
	if _, err := f.submitLoginEmail(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting login password")
	if err := f.openLoginPasswordEntry(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	state, otpIssuedAfter, err := f.submitLoginPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return err
	}
	if state == "otp_required" {
		f.markWaitingForOTP(browserAuthOTPKindLoginEmail, unixSecondsFromMillis(otpIssuedAfter))
		return nil
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		f.fail(err)
		return err
	}
	if err := f.captureResult(client, cfg, true); err != nil {
		f.fail(err)
		return err
	}
	return nil
}

func (f *browserAuthFlow) completeFromCheckpoint(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp, otpKind string) error {
	code := channelotpwait.NormalizeCode(otp)
	if code == "" {
		return fmt.Errorf("otp is required")
	}
	f.setTaskScope("complete-" + strconv.FormatInt(time.Now().UnixMilli(), 10))
	switch strings.TrimSpace(otpKind) {
	case browserAuthOTPKindLoginEmail:
		f.setStatus(browserAuthStageOTPSubmit, "submitting login OTP")
		if _, err := f.submitLoginOTP(client, cfg, code); err != nil {
			return err
		}
	default:
		if f.mode == browserAuthModeLogin {
			f.setStatus(browserAuthStageOTPSubmit, "submitting login OTP")
			if _, err := f.submitLoginOTP(client, cfg, code); err != nil {
				return err
			}
			break
		}
		f.setStatus(browserAuthStageOTPSubmit, "submitting register OTP")
		if _, err := f.submitRegisterOTP(client, cfg, code); err != nil {
			return err
		}
		if _, err := f.completeRegisterProfile(client, cfg); err != nil {
			return err
		}
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		return err
	}
	return f.captureResult(client, cfg, true)
}

func (f *browserAuthFlow) promoteFlowIDToSessionID() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sessionID != "" {
		f.flowID = f.sessionID
	}
}

func (f *browserAuthFlow) getOTPKind() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.otpKind
}

func (f *browserAuthFlow) isDone() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done
}
