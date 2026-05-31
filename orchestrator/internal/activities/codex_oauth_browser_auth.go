package activities

import (
	"fmt"
	"orchestrator/internal/gptaccount"
)

func (f *codexOAuthBrowserFlow) completeAuthorization() error {
	callbackURL, err := f.browserFlow.completeCodexOAuthConsentAndCallback(f.server.browserAutomationClient, f.server.browserAuthConfig)
	if err != nil {
		return f.fail(err)
	}
	f.data.setCallbackURLCaptured(true)
	code, returnedState, err := codexOAuthCodeFromCallback(callbackURL)
	if err != nil {
		return f.fail(err)
	}
	if returnedState != f.state {
		return f.fail(fmt.Errorf("codex oauth state mismatch"))
	}
	accountProfile, err := f.server.accountFingerprint(f.ctx, gptaccount.ID(f.account))
	if err != nil {
		return f.fail(err)
	}
	tokens, err := exchangeCodexOAuthTokenWithProfile(f.ctx, f.cfg, code, f.pkce.verifier, codexOAuthProtocolProfileFromAccount(accountProfile, f.cfg))
	if err != nil {
		return f.fail(err)
	}
	authJSON, err := buildCodexAuthJSON(tokens)
	if err != nil {
		return f.fail(err)
	}
	f.authJSON = authJSON
	return nil
}

func (f *codexOAuthBrowserFlow) persistAuthorization() error {
	if err := f.writeRuntimeAuthSecret(); err != nil {
		return err
	}
	if f.markPhoneConfirmed {
		if err := f.writeAccountPhoneConfirmation(); err != nil {
			return err
		}
	}
	return nil
}

func (f *codexOAuthBrowserFlow) writeAccountPhoneConfirmation() error {
	if err := f.server.markCodexOAuthPhoneConfirmed(f.ctx, gptaccount.ID(f.account), f.label, f.data); err != nil {
		return f.fail(fmt.Errorf("save codex phone state to account db: %w", err))
	}
	return nil
}

func (f *codexOAuthBrowserFlow) writeRuntimeAuthSecret() error {
	if err := f.server.saveCodexAuthJSON(f.ctx, gptaccount.ID(f.account), string(f.authJSON)); err != nil {
		return f.fail(err)
	}
	f.secretKey = codexOAuthAuthSecretKey(gptaccount.ID(f.account))
	f.data.setAuthSecretKey(f.secretKey)
	f.data.setAuthSecretWritten(true)
	return nil
}
