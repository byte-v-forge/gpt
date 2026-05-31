package activities

import (
	"orchestrator/internal/gptaccount"
	"strings"
)

func (f *codexOAuthBrowserFlow) openAuthorizeURL() error {
	if err := f.browserFlow.openCodexOAuthEntry(f.server.browserAutomationClient, f.server.browserAuthConfig, f.authorizeURL); err != nil {
		return f.fail(err)
	}
	return nil
}

func (f *codexOAuthBrowserFlow) ensureLoggedIn() error {
	if err := f.browserFlow.ensureCodexOAuthLoggedIn(f.ctx, f.server, f.account, f.jobID, f.data); err != nil {
		return f.fail(err)
	}
	return nil
}

func (f *codexOAuthBrowserFlow) handleAddPhoneStage() (codexOAuthBrowserResult, error) {
	stage, err := f.browserFlow.detectCodexOAuthStage(f.server.browserAutomationClient, f.server.browserAuthConfig)
	if err != nil {
		return codexOAuthBrowserResult{}, f.fail(err)
	}
	f.phoneNeeded = stage == "add_phone"
	if !f.phoneNeeded {
		return codexOAuthBrowserResult{}, f.releaseUnusedPhoneLease()
	}
	if err := f.server.markCodexOAuthNeedPhone(f.ctx, gptaccount.ID(f.account), f.label, f.data); err != nil {
		f.data.setAccountPhoneNeedWriteError(err)
	}
	if !f.allowAddPhone {
		f.data.setAddPhoneRequired(true)
		f.failure = "codex_oauth_add_phone_required"
		return codexOAuthBrowserResult{addPhoneRequired: true}, codexOAuthAddPhoneRequiredError()
	}
	if err := f.addPhoneToCodexOAuthAccount(); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	return codexOAuthBrowserResult{}, nil
}

func (f *codexOAuthBrowserFlow) addPhoneToCodexOAuthAccount() error {
	otpReceived, err := f.browserFlow.completeCodexOAuthAddPhone(f.ctx, f.server, f.jobID, f.phone, f.cfg, f.data)
	f.phoneUsed = otpReceived
	if err != nil {
		return f.fail(err)
	}
	f.data.setAddPhoneRequired(true)
	stage, err := f.browserFlow.detectCodexOAuthStage(f.server.browserAutomationClient, f.server.browserAuthConfig)
	if err != nil {
		return f.fail(err)
	}
	f.data.setPostAddPhoneStage(stage)
	f.phoneAdded = stage == "consent" || stage == "callback"
	f.data.setAddPhoneConfirmed(f.phoneAdded)
	if !f.phoneAdded {
		f.data.setAddPhonePendingStage(stage)
		if err := f.server.markCodexOAuthNeedPhone(f.ctx, gptaccount.ID(f.account), f.label, f.data); err != nil {
			f.data.setAccountPhoneNeedWriteError(err)
		}
	}
	if err := f.server.markCodexPhoneSuccess(f.ctx, f.phone, gptaccount.ID(f.account), f.jobID, f.label); err != nil {
		return f.fail(err)
	}
	if f.phoneAdded {
		if err := f.server.markCodexOAuthPhoneConfirmed(f.ctx, gptaccount.ID(f.account), f.label, f.data); err != nil {
			return f.fail(err)
		}
	}
	return nil
}

func (f *codexOAuthBrowserFlow) releaseUnusedPhoneLease() error {
	f.data.setAddPhoneConfirmed(false)
	f.data.setAddPhoneRequired(false)
	if f.phone == nil || strings.TrimSpace(f.phone.GetActivationId()) == "" {
		return nil
	}
	return f.server.releaseCodexPhone(f.ctx, f.phone, gptaccount.ID(f.account), f.jobID, f.label, false, "add phone not required")
}
