package activities

import (
	"context"
)

func (s *Server) CodexOAuthStartBrowserActivity(ctx context.Context, input CodexOAuthStartBrowserInput) (CodexOAuthStartBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := CodexOAuthStartBrowserOutput{PhoneLabel: label}
	data := codexOAuthBrowserData(label, input.GetPhone())
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthBrowserStart, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthBrowserStart, "starting codex oauth browser", data)
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		flow, err := s.newCodexOAuthBrowserStartFlow(ctx, account, input.GetJobId(), label, input.GetPhone(), cfg, input.GetAllowAddPhone(), data)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		pkceKey := codexOAuthPKCESecretKey(input.GetJobId(), flow.browserFlow.flowID)
		if err := s.saveRuntimeSecret(ctx, pkceKey, flow.pkce.verifier); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		data["pkce_secret_key"] = pkceKey
		data["pkce_secret_written"] = true
		if err := flow.startSession(); err != nil {
			_ = s.deleteRuntimeSecret(context.Background(), pkceKey)
			flow.failure = err.Error()
			flow.releasePhoneOnFailure()
			data["error_message"] = err.Error()
			return data, err
		}
		output.Session = &CodexOAuthBrowserSession{
			FlowId:        flow.browserFlow.flowID,
			SessionId:     flow.browserFlow.getSessionID(),
			State:         flow.state,
			PkceSecretKey: pkceKey,
		}
		data["flow_id"] = output.GetSession().GetFlowId()
		data["browser_session_started"] = true
		if err := flow.openAuthorizeURL(); err != nil {
			flow.stopSession()
			_ = s.deleteRuntimeSecret(context.Background(), pkceKey)
			flow.releasePhoneOnFailure()
			data["error_message"] = err.Error()
			return data, err
		}
		output.Success = true
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}
