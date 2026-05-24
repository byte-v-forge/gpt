package activities

import (
	"fmt"
	"time"

	"orchestrator/pb"
)

func (f *codexOAuthBrowserFlow) completeAuthorization() error {
	callbackURL, err := f.browserFlow.completeCodexOAuthConsentAndCallback(f.server.browserAutomationClient, f.server.browserAuthConfig)
	if err != nil {
		return f.fail(err)
	}
	f.data["callback_url_captured"] = true
	code, returnedState, err := codexOAuthCodeFromCallback(callbackURL)
	if err != nil {
		return f.fail(err)
	}
	if returnedState != f.state {
		return f.fail(fmt.Errorf("codex oauth state mismatch"))
	}
	tokens, err := exchangeCodexOAuthToken(f.ctx, f.cfg, code, f.pkce.verifier)
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
	if err := f.writeAccountAuthJSON(); err != nil {
		return err
	}
	if f.markPhoneConfirmed {
		if err := f.writeAccountPhoneConfirmation(); err != nil {
			return err
		}
	}
	return f.writeRuntimeAuthSecret()
}

func (f *codexOAuthBrowserFlow) writeAccountAuthJSON() error {
	if err := f.server.updateAccount(f.ctx, &pb.Account{
		AccountId:              f.account.GetAccountId(),
		CodexAuthJson:          string(f.authJSON),
		CodexAuthUpdatedAtUnix: time.Now().Unix(),
	}); err != nil {
		return f.fail(fmt.Errorf("save codex auth json to account db: %w", err))
	}
	f.data["account_auth_written"] = true
	return nil
}

func (f *codexOAuthBrowserFlow) writeAccountPhoneConfirmation() error {
	if err := f.server.markCodexOAuthPhoneConfirmed(f.ctx, f.account.GetAccountId(), f.label, f.data); err != nil {
		return f.fail(fmt.Errorf("save codex phone state to account db: %w", err))
	}
	return nil
}

func (f *codexOAuthBrowserFlow) writeRuntimeAuthSecret() error {
	secretKey := codexOAuthAuthSecretPrefix + f.account.GetAccountId()
	if err := f.server.saveRuntimeSecret(f.ctx, secretKey, string(f.authJSON)); err != nil {
		return f.fail(err)
	}
	f.secretKey = secretKey
	f.data["auth_secret_key"] = secretKey
	f.data["auth_secret_written"] = true
	return nil
}
