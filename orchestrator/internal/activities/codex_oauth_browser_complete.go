package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/contracts"
	"strings"
)

func (s *Server) CodexOAuthCompleteBrowserActivity(ctx context.Context, input CodexOAuthCompleteBrowserInput) (CodexOAuthCompleteBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := CodexOAuthCompleteBrowserOutput{}
	data := codexOAuthBrowserData(label, nil)
	data.setAuthSecretWritten(false)
	data.setAccountAuthWritten(false)
	data.setCallbackURLCaptured(false)
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthBrowserComplete, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepCodexOAuthBrowserComplete, "completing codex oauth browser", data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		flow, err := s.newCodexOAuthBrowserSessionFlow(ctx, account, input.GetJobId(), label, nil, cfg, false, input.GetMarkPhoneConfirmedOnSuccess(), input.GetSession(), data, contracts.StepCodexOAuthBrowserComplete)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		verifier := s.loadRuntimeSecret(ctx, input.GetSession().GetPkceSecretKey())
		if strings.TrimSpace(verifier) == "" {
			err := fmt.Errorf("codex oauth pkce verifier is missing")
			data.setError(err)
			return data.messageData(), err
		}
		flow.pkce = codexOAuthPKCE{verifier: verifier}
		if err := flow.completeAuthorization(); err != nil {
			data.setError(err)
			return data.messageData(), err
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
