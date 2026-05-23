package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/pb"
)

type codexOAuthBrowserResult struct {
	authSecretKey     string
	phoneReuseCount   int32
	phoneReuseLimit   int32
	addPhoneConfirmed bool
	addPhoneRequired  bool
}

func (s *Server) runCodexOAuthBrowser(ctx context.Context, account *pb.Account, jobID, label string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, allowAddPhone bool, markPhoneConfirmed bool, data map[string]any) (codexOAuthBrowserResult, error) {
	if s.browserAutomationClient == nil {
		return codexOAuthBrowserResult{}, fmt.Errorf("browser automation client is not configured")
	}
	if allowAddPhone {
		if err := ensureCodexOAuthPhoneLeaseUsable(phone, cfg); err != nil {
			return codexOAuthBrowserResult{}, err
		}
	}
	pkce, err := newCodexOAuthPKCE()
	if err != nil {
		return codexOAuthBrowserResult{}, err
	}
	state, err := randomURLToken(32)
	if err != nil {
		return codexOAuthBrowserResult{}, err
	}
	authorizeURL := buildCodexOAuthAuthorizeURL(cfg, pkce, state)
	flow := newBrowserAuthFlow("codex_oauth_add_phone", jobID, account)
	if err := flow.startSession(s.browserAutomationClient, s.browserAuthConfig); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	defer flow.stopSession(s.browserAutomationClient)

	phoneUsed := false
	success := false
	failureMessage := "codex oauth browser did not complete"
	defer func() {
		if !success {
			_ = s.releaseCodexPhone(ctx, phone, account.GetAccountId(), jobID, label, phoneUsed, failureMessage)
		}
	}()

	if err := flow.openCodexOAuthEntry(s.browserAutomationClient, s.browserAuthConfig, authorizeURL); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if err := flow.ensureCodexOAuthLoggedIn(ctx, s, account, jobID, data); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	stage, err := flow.detectCodexOAuthStage(s.browserAutomationClient, s.browserAuthConfig)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	addPhoneConfirmed := false
	addPhoneRequired := stage == "add_phone"
	if stage == "add_phone" {
		if !allowAddPhone {
			data["add_phone_required"] = true
			failureMessage = "codex_oauth_add_phone_required"
			return codexOAuthBrowserResult{addPhoneRequired: true}, fmt.Errorf("codex_oauth_add_phone_required")
		}
		phoneUsed = true
		if err := flow.completeCodexOAuthAddPhone(ctx, s, jobID, phone, cfg, data); err != nil {
			failureMessage = err.Error()
			return codexOAuthBrowserResult{}, err
		}
		data["add_phone_confirmed"] = true
		data["add_phone_required"] = true
		addPhoneConfirmed = true
		if err := s.markCodexPhoneSuccess(ctx, phone, account.GetAccountId(), jobID, label); err != nil {
			failureMessage = err.Error()
			return codexOAuthBrowserResult{}, err
		}
	} else {
		data["add_phone_confirmed"] = false
		data["add_phone_required"] = false
		if phone != nil && strings.TrimSpace(phone.GetActivationId()) != "" {
			if err := s.releaseCodexPhone(ctx, phone, account.GetAccountId(), jobID, label, false, "add phone not required"); err != nil {
				return codexOAuthBrowserResult{}, err
			}
		}
	}
	callbackURL, err := flow.completeCodexOAuthConsentAndCallback(s.browserAutomationClient, s.browserAuthConfig)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	data["callback_url_captured"] = true
	code, returnedState, err := codexOAuthCodeFromCallback(callbackURL)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if returnedState != state {
		failureMessage = "codex oauth state mismatch"
		return codexOAuthBrowserResult{}, fmt.Errorf("codex oauth state mismatch")
	}
	tokens, err := exchangeCodexOAuthToken(ctx, cfg, code, pkce.verifier)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	authJSON, err := buildCodexAuthJSON(tokens)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if err := s.updateAccount(ctx, &pb.Account{
		AccountId:              account.GetAccountId(),
		CodexAuthJson:          string(authJSON),
		CodexAuthUpdatedAtUnix: time.Now().Unix(),
	}); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, fmt.Errorf("save codex auth json to account db: %w", err)
	}
	data["account_auth_written"] = true
	if markPhoneConfirmed {
		if err := s.updateAccount(ctx, &pb.Account{
			AccountId:               account.GetAccountId(),
			CodexPhoneConfirmed:     boolPtr(true),
			CodexPhoneLabel:         label,
			CodexPhoneUpdatedAtUnix: time.Now().Unix(),
		}); err != nil {
			failureMessage = err.Error()
			return codexOAuthBrowserResult{}, fmt.Errorf("save codex phone state to account db: %w", err)
		}
		data["account_phone_confirmed_written"] = true
	}
	secretKey := codexOAuthAuthSecretPrefix + account.GetAccountId()
	if err := s.saveRuntimeSecret(ctx, secretKey, string(authJSON)); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	data["auth_secret_key"] = secretKey
	data["auth_secret_written"] = true
	success = true
	reuseCount := int32(0)
	reuseLimit := int32(0)
	if phone != nil {
		reuseCount = phone.GetReuseCount()
		reuseLimit = phone.GetReuseLimit()
	}
	return codexOAuthBrowserResult{
		authSecretKey:     secretKey,
		phoneReuseCount:   reuseCount,
		phoneReuseLimit:   reuseLimit,
		addPhoneConfirmed: addPhoneConfirmed,
		addPhoneRequired:  addPhoneRequired,
	}, nil
}
