package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/contracts"
	"orchestrator/internal/gptaccount"
	"strings"

	"orchestrator/pb"
)

func (s *Server) CodexOAuthCompleteProtocolActivity(ctx context.Context, input *CodexOAuthCompleteBrowserInput) (*CodexOAuthCompleteBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := &CodexOAuthCompleteBrowserOutput{}
	data := codexOAuthProtocolData(label)
	data.setAuthSecretWritten(false)
	data.setAccountAuthWritten(false)
	data.setCallbackURLCaptured(false)
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthProtocolComplete, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepCodexOAuthProtocolComplete, "completing codex oauth protocol", data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		state, err := s.loadCodexOAuthProtocolState(ctx, input.GetJobId(), input.GetSession())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		client, err := s.newAccountGptClient(ctx, input.GetAccountId(), cfg, state)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, state.Stage)
		callbackURL, err := s.codexOAuthProtocolCallbackURL(ctx, client, state, data)
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if state.Stage == "add_phone" && !state.PhonePresent {
			if markErr := s.markCodexOAuthNeedPhone(ctx, gptaccount.ID(account), label, data); markErr != nil {
				data.setAccountPhoneNeedWriteError(markErr)
			}
		}
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		verifier := s.loadRuntimeSecret(ctx, state.PkceSecretKey)
		if strings.TrimSpace(verifier) == "" {
			err := fmt.Errorf("codex oauth pkce verifier is missing")
			data.setError(err)
			return data.messageData(), err
		}
		code, returnedState, err := codexOAuthCodeFromCallback(callbackURL)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		if returnedState != state.OauthState {
			err := fmt.Errorf("codex oauth state mismatch")
			data.setError(err)
			return data.messageData(), err
		}
		tokenCfg := cfg
		if strings.TrimSpace(tokenCfg.ProtocolProxyURL) != "" {
			tokenCfg.TokenProxyURL = tokenCfg.ProtocolProxyURL
		}
		tokens, err := exchangeCodexOAuthTokenWithProfile(ctx, tokenCfg, code, verifier, client.profile)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		authJSON, err := buildCodexAuthJSON(tokens)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		flow := &codexOAuthBrowserFlow{server: s, ctx: ctx, account: account, jobID: input.GetJobId(), label: label, cfg: cfg, data: data, authJSON: authJSON, markPhoneConfirmed: input.GetMarkPhoneConfirmedOnSuccess() || state.PhonePresent}
		if state.PhonePresent {
			flow.phoneAdded = true
		}
		if err := flow.persistAuthorization(); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		output.Success = true
		output.AuthSecretKey = flow.secretKey
		output.Data = data.messageData()
		return data.messageData(), nil
	})
	if output.Data == nil {
		output.Data = data.messageData()
	}
	return output, err
}

func (s *Server) codexOAuthProtocolCallbackURL(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, data *codexOAuthStepData) (string, error) {
	if state == nil {
		return "", fmt.Errorf("codex oauth protocol state missing")
	}
	if codexOAuthProtocolIsCallbackURL(state.LastUrl, state.RedirectUri) {
		data.setCallbackURLCaptured(true)
		return state.LastUrl, nil
	}
	candidates := []string{state.LastContinueUrl, state.AuthorizeUrl}
	if codexOAuthProtocolCanCompleteStage(state.Stage) && strings.TrimSpace(state.LastContinueUrl) != "" {
		candidates = []string{state.LastContinueUrl}
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
		if codexOAuthProtocolIsCallbackURL(state.LastUrl, state.RedirectUri) {
			data.setCallbackURLCaptured(true)
			return state.LastUrl, nil
		}
	}
	if state.Stage == "add_phone" && !state.PhonePresent {
		data.setAddPhoneRequired(true)
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

func (s *Server) CodexOAuthStopProtocolActivity(ctx context.Context, input *CodexOAuthStopBrowserInput) error {
	if input.GetSession() == nil {
		return nil
	}
	if err := s.deleteCodexOAuthProtocolState(ctx, input.GetJobId(), input.GetSession()); err != nil {
		return err
	}
	return s.deleteRuntimeSecret(ctx, input.GetSession().GetPkceSecretKey())
}
