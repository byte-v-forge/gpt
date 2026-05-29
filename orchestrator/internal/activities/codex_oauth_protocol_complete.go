package activities

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) CodexOAuthCompleteProtocolActivity(ctx context.Context, input CodexOAuthCompleteBrowserInput) (CodexOAuthCompleteBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := CodexOAuthCompleteBrowserOutput{}
	data := codexOAuthProtocolData(label)
	data["auth_secret_written"] = false
	data["account_auth_written"] = false
	data["callback_url_captured"] = false
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthProtocolComplete, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthProtocolComplete, "completing codex oauth protocol", data)
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		state, err := s.loadCodexOAuthProtocolState(ctx, input.GetJobId(), input.GetSession())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		client, err := s.newAccountGptClient(ctx, input.GetAccountId(), cfg, state)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, state.Stage)
		callbackURL, err := s.codexOAuthProtocolCallbackURL(ctx, client, state, data)
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if state.Stage == "add_phone" && !state.PhonePresent {
			if markErr := s.markCodexOAuthNeedPhone(ctx, account.GetAccountId(), label, data); markErr != nil {
				data["account_phone_need_write_error"] = markErr.Error()
			}
		}
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		verifier := s.loadRuntimeSecret(ctx, state.PKCESecretKey)
		if strings.TrimSpace(verifier) == "" {
			err := fmt.Errorf("codex oauth pkce verifier is missing")
			data["error_message"] = err.Error()
			return data, err
		}
		code, returnedState, err := codexOAuthCodeFromCallback(callbackURL)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if returnedState != state.OAuthState {
			err := fmt.Errorf("codex oauth state mismatch")
			data["error_message"] = err.Error()
			return data, err
		}
		tokenCfg := cfg
		if strings.TrimSpace(tokenCfg.ProtocolProxyURL) != "" {
			tokenCfg.TokenProxyURL = tokenCfg.ProtocolProxyURL
		}
		tokens, err := exchangeCodexOAuthTokenWithProfile(ctx, tokenCfg, code, verifier, client.profile)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		authJSON, err := buildCodexAuthJSON(tokens)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		flow := &codexOAuthBrowserFlow{server: s, ctx: ctx, account: account, jobID: input.GetJobId(), label: label, cfg: cfg, data: data, authJSON: authJSON, markPhoneConfirmed: input.GetMarkPhoneConfirmedOnSuccess() || state.PhonePresent}
		if state.PhonePresent {
			flow.phoneAdded = true
		}
		if err := flow.persistAuthorization(); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		output.Success = true
		output.AuthSecretKey = flow.secretKey
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}

func (s *Server) codexOAuthProtocolCallbackURL(ctx context.Context, client *GptClient, state *codexOAuthProtocolState, data map[string]any) (string, error) {
	if state == nil {
		return "", fmt.Errorf("codex oauth protocol state missing")
	}
	if codexOAuthProtocolIsCallbackURL(state.LastURL, state.RedirectURI) {
		data["callback_url_captured"] = true
		return state.LastURL, nil
	}
	candidates := []string{state.LastContinueURL, state.AuthorizeURL}
	if codexOAuthProtocolCanCompleteStage(state.Stage) && strings.TrimSpace(state.LastContinueURL) != "" {
		candidates = []string{state.LastContinueURL}
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		stage, err := runCodexOAuthProtocolURL(ctx, client, state, candidate, codexOAuthProtocolRefererForStage(state.Stage), data)
		if err != nil {
			return "", err
		}
		state.Stage = stage
		if codexOAuthProtocolIsCallbackURL(state.LastURL, state.RedirectURI) {
			data["callback_url_captured"] = true
			return state.LastURL, nil
		}
	}
	if state.Stage == "add_phone" && !state.PhonePresent {
		data["add_phone_required"] = true
		return "", codexOAuthAddPhoneRequiredError()
	}
	return "", fmt.Errorf("codex oauth callback stage not ready: %s", state.Stage)
}

func codexOAuthProtocolCanCompleteStage(stage string) bool {
	switch stage {
	case "consent", "callback":
		return true
	default:
		return false
	}
}

func (s *Server) CodexOAuthStopProtocolActivity(ctx context.Context, input CodexOAuthStopBrowserInput) error {
	if input.GetSession() == nil {
		return nil
	}
	if err := s.deleteCodexOAuthProtocolState(ctx, input.GetJobId(), input.GetSession()); err != nil {
		return err
	}
	return s.deleteRuntimeSecret(ctx, input.GetSession().GetPkceSecretKey())
}
