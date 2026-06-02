package activities

import (
	"context"
	"orchestrator/internal/contracts"
)

func (s *Server) CodexOAuthStartBrowserActivity(ctx context.Context, input *CodexOAuthStartBrowserInput) (*CodexOAuthStartBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := &CodexOAuthStartBrowserOutput{PhoneLabel: label}
	data := codexOAuthBrowserData(label, input.GetPhone())
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthBrowserStart, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepCodexOAuthBrowserStart, "starting codex oauth browser", data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		flow, err := s.newCodexOAuthBrowserStartFlow(ctx, account, input.GetJobId(), label, input.GetPhone(), cfg, input.GetAllowAddPhone(), data)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		pkceKey := codexOAuthPKCESecretKey(input.GetJobId(), flow.browserFlow.flowID)
		if err := s.saveRuntimeSecret(ctx, pkceKey, flow.pkce.verifier); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		data.setPKCESecretKey(pkceKey)
		data.setPKCESecretWritten(true)
		if err := flow.startSession(); err != nil {
			_ = s.deleteRuntimeSecret(context.Background(), pkceKey)
			flow.failure = err.Error()
			flow.releasePhoneOnFailure()
			data.setError(err)
			return data.messageData(), err
		}
		output.Session = &CodexOAuthBrowserSession{
			FlowId:        flow.browserFlow.flowID,
			SessionId:     flow.browserFlow.getSessionID(),
			State:         flow.state,
			PkceSecretKey: pkceKey,
		}
		data.setFlowID(output.GetSession().GetFlowId())
		data.setBrowserSessionStarted(true)
		if err := flow.openAuthorizeURL(); err != nil {
			flow.stopSession()
			_ = s.deleteRuntimeSecret(context.Background(), pkceKey)
			flow.releasePhoneOnFailure()
			data.setError(err)
			return data.messageData(), err
		}
		output.Success = true
		output.Data = data.messageData()
		return data.messageData(), nil
	})
	if output.Data == nil {
		output.Data = data.messageData()
	}
	return output, err
}
