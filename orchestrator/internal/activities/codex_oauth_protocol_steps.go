package activities

import (
	"context"
	"time"

	"orchestrator/pb"
)

func (s *Server) CodexOAuthStartProtocolActivity(ctx context.Context, input CodexOAuthStartBrowserInput) (CodexOAuthStartBrowserOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthStartBrowserOutput{PhoneLabel: label}
	data := codexOAuthProtocolData(label)
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthProtocolStart, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthProtocolStart, "starting codex oauth protocol", data)
		defer stopHeartbeat()
		if _, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId()); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		pkce, err := newCodexOAuthPKCE()
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		state, err := newCodexOAuthProtocolState(input.GetJobId(), cfg, pkce)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if err := s.saveRuntimeSecret(ctx, state.PKCESecretKey, pkce.verifier); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		client, err := newCodexOAuthProtocolHTTPClient(cfg, &state)
		if err != nil {
			_ = s.deleteRuntimeSecret(context.Background(), state.PKCESecretKey)
			data["error_message"] = err.Error()
			return data, err
		}
		stage, err := runCodexOAuthProtocolURL(ctx, client, &state, state.AuthorizeURL, "https://chatgpt.com/", data)
		if err == nil {
			_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, &state, data, stage)
			stage = codexOAuthProtocolStageFromDump(&state, stage)
			state.Stage = stage
			data["login_stage"] = stage
		}
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), &state); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
			_ = s.deleteRuntimeSecret(context.Background(), state.PKCESecretKey)
			_ = s.deleteCodexOAuthProtocolState(context.Background(), input.GetJobId(), &CodexOAuthBrowserSession{FlowId: state.FlowID})
			data["error_message"] = err.Error()
			return data, err
		}
		output.Session = &CodexOAuthBrowserSession{FlowId: state.FlowID, State: state.OAuthState, PkceSecretKey: state.PKCESecretKey}
		data["flow_id"] = state.FlowID
		data["protocol_session_started"] = true
		data["device_id_present"] = state.DeviceID != ""
		output.Success = true
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}

func (s *Server) CodexOAuthDetectProtocolStageActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthProtocolStageStep(ctx, input, stepCodexOAuthProtocolDetect, "detecting codex oauth protocol stage", func(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, int64, error) {
		stage := state.Stage
		if stage == "" && state.LastURL != "" {
			stage = codexOAuthProtocolStageFromURL(state.LastURL, "")
		}
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		stage = codexOAuthProtocolStageFromDump(state, stage)
		return stage, state.EmailOTPIssuedAfterUnix, nil
	})
}

func (s *Server) CodexOAuthSubmitProtocolEmailActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthProtocolStageStep(ctx, input, stepCodexOAuthProtocolEmail, "submitting codex oauth protocol email", func(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, int64, error) {
		sentinel := codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue")
		resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/authorize/continue", "https://auth.openai.com/log-in", map[string]any{
			"username":    map[string]any{"value": account.GetEmail(), "kind": "email"},
			"screen_hint": "login",
		}, sentinel)
		if err != nil {
			return "", 0, err
		}
		stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "email", data)
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		return codexOAuthProtocolStageFromDump(state, stage), 0, err
	})
}

func (s *Server) CodexOAuthSubmitProtocolPasswordActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthProtocolStageStep(ctx, input, stepCodexOAuthProtocolPassword, "submitting codex oauth protocol password", func(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, int64, error) {
		issuedAfter := time.Now().Add(-time.Second).Unix()
		sentinel := codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue")
		resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/password/verify", "https://auth.openai.com/log-in/password", map[string]any{"password": account.GetPassword()}, sentinel)
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "password", data)
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		return codexOAuthProtocolStageFromDump(state, stage), issuedAfter, err
	})
}

func (s *Server) CodexOAuthSubmitProtocolEmailOTPActivity(ctx context.Context, input CodexOAuthSubmitEmailOTPInput) (CodexOAuthBrowserStageOutput, error) {
	stepInput := CodexOAuthBrowserStepInput{JobId: input.GetJobId(), AccountId: input.GetAccountId(), Label: input.GetLabel(), Session: input.GetSession()}
	return s.codexOAuthProtocolStageStep(ctx, stepInput, stepCodexOAuthProtocolEmailOTP, "submitting codex oauth protocol email otp", func(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, int64, error) {
		otp, err := s.waitCodexOAuthEmailOTP(ctx, input.GetJobId(), account.GetEmail(), input.GetIssuedAfterUnix())
		if err != nil {
			return "", input.GetIssuedAfterUnix(), err
		}
		resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/email-otp/validate", "https://auth.openai.com/email-verification", map[string]any{"code": normalizeOTP(otp)})
		if err != nil {
			return "", input.GetIssuedAfterUnix(), err
		}
		stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "email_otp", data)
		if err == nil && (stage == "" || stage == "email_otp") {
			if next, navErr := runCodexOAuthProtocolURL(ctx, client, state, state.AuthorizeURL, "https://auth.openai.com/email-verification", data); navErr == nil && next != "" {
				stage = next
			}
		}
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		return codexOAuthProtocolStageFromDump(state, stage), input.GetIssuedAfterUnix(), err
	})
}

func (s *Server) codexOAuthProtocolStageStep(ctx context.Context, input CodexOAuthBrowserStepInput, stepName, heartbeat string, fn func(context.Context, *codexOAuthProtocolHTTPClient, *codexOAuthProtocolState, *pb.Account, map[string]any) (string, int64, error)) (CodexOAuthBrowserStageOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthBrowserStageOutput{}
	data := codexOAuthProtocolData(label)
	step := s.activityStep(ctx, input.GetJobId(), stepName, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, heartbeat, data)
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
		client, err := newCodexOAuthProtocolHTTPClient(cfg, state)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		stage, issuedAfter, err := fn(ctx, client, state, account, data)
		if stage != "" {
			state.Stage = stage
			data["login_stage"] = stage
			output.Stage = stage
		}
		if issuedAfter > 0 {
			state.EmailOTPIssuedAfterUnix = issuedAfter
			data["email_otp_issued_after_unix"] = issuedAfter
			output.EmailOtpIssuedAfterUnix = issuedAfter
		}
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
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
