package activities

import (
	"context"
	"orchestrator/internal/contracts"
	"orchestrator/internal/gptaccount"

	"orchestrator/internal/channelotpwait"
	"orchestrator/pb"
)

func (s *Server) CodexOAuthStartProtocolActivity(ctx context.Context, input *CodexOAuthStartBrowserInput) (*CodexOAuthStartBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := &CodexOAuthStartBrowserOutput{PhoneLabel: label}
	data := codexOAuthProtocolData(label)
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthProtocolStart, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepCodexOAuthProtocolStart, "starting codex oauth protocol", data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		pkce, err := newCodexOAuthPKCE()
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		state, err := newCodexOAuthProtocolState(input.GetJobId(), cfg, pkce)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		if err := s.saveRuntimeSecret(ctx, state.PkceSecretKey, pkce.verifier); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		client, err := s.newAccountGptClient(ctx, input.GetAccountId(), cfg, state)
		if err != nil {
			_ = s.deleteRuntimeSecret(context.Background(), state.PkceSecretKey)
			data.setError(err)
			return data.messageData(), err
		}
		stage, err := runCodexOAuthProtocolURL(ctx, client, state, state.AuthorizeUrl, "https://chatgpt.com/", data)
		if err == nil {
			_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
			stage = codexOAuthProtocolStageFromDump(state, stage)
			state.Stage = stage
			data.setStage(stage)
			if stage == "add_phone" {
				if markErr := s.markCodexOAuthNeedPhone(ctx, gptaccount.ID(account), label, data); markErr != nil {
					data.setAccountPhoneNeedWriteError(markErr)
				}
			}
		}
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
			_ = s.deleteRuntimeSecret(context.Background(), state.PkceSecretKey)
			_ = s.deleteCodexOAuthProtocolState(context.Background(), input.GetJobId(), &CodexOAuthBrowserSession{FlowId: state.FlowId})
			data.setError(err)
			return data.messageData(), err
		}
		output.Session = &CodexOAuthBrowserSession{FlowId: state.FlowId, State: state.OauthState, PkceSecretKey: state.PkceSecretKey}
		data.setFlowID(state.FlowId)
		data.setProtocolSessionStarted(true)
		data.setDeviceIDPresent(state.DeviceId != "")
		output.Success = true
		output.Data = data.messageData()
		return data.messageData(), nil
	})
	if output.Data == nil {
		output.Data = data.messageData()
	}
	return output, err
}

func (s *Server) CodexOAuthDetectProtocolStageActivity(ctx context.Context, input *CodexOAuthBrowserStepInput) (*CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthProtocolStageStep(ctx, input, contracts.StepCodexOAuthProtocolDetect, "detecting codex oauth protocol stage", func(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, account *pb.Account, data *codexOAuthStepData) (string, int64, error) {
		stage := state.Stage
		if stage == "" && state.LastUrl != "" {
			stage = codexOAuthProtocolStageFromURL(state.LastUrl, "")
		}
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		stage = codexOAuthProtocolStageFromDump(state, stage)
		return stage, state.EmailOtpIssuedAfterUnix, nil
	})
}

func (s *Server) CodexOAuthSubmitProtocolEmailActivity(ctx context.Context, input *CodexOAuthBrowserStepInput) (*CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthProtocolStageStep(ctx, input, contracts.StepCodexOAuthProtocolEmail, "submitting codex oauth protocol email", func(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, account *pb.Account, data *codexOAuthStepData) (string, int64, error) {
		sentinel := codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue")
		resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/authorize/continue", "https://auth.openai.com/log-in", map[string]any{
			"username":    map[string]any{"value": gptaccount.Email(account), "kind": "email"},
			"screen_hint": "login",
		}, sentinel)
		issuedAfter := codexOAuthProtocolResponseSentAtUnix(client, resp)
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "email", data)
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		return codexOAuthProtocolStageFromDump(state, stage), issuedAfter, err
	})
}

func (s *Server) CodexOAuthSubmitProtocolPasswordActivity(ctx context.Context, input *CodexOAuthBrowserStepInput) (*CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthProtocolStageStep(ctx, input, contracts.StepCodexOAuthProtocolPassword, "submitting codex oauth protocol password", func(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, account *pb.Account, data *codexOAuthStepData) (string, int64, error) {
		password, err := s.getAccountPassword(ctx, gptaccount.ID(account))
		if err != nil {
			return "", 0, err
		}
		sentinel := codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue")
		resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/password/verify", "https://auth.openai.com/log-in/password", map[string]any{"password": password}, sentinel)
		issuedAfter := codexOAuthProtocolResponseSentAtUnix(client, resp)
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "password", data)
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		return codexOAuthProtocolStageFromDump(state, stage), issuedAfter, err
	})
}

func codexOAuthProtocolResponseSentAtUnix(client *GptClient, resp *codexOAuthProtocolHTTPResponse) int64 {
	if resp != nil && resp.SentAtUnixMs > 0 {
		return unixSecondsFromMillis(resp.SentAtUnixMs)
	}
	if client != nil && client.lastRequestSentAtUnixMs > 0 {
		return unixSecondsFromMillis(client.lastRequestSentAtUnixMs)
	}
	return 0
}

func (s *Server) CodexOAuthSubmitProtocolEmailOTPActivity(ctx context.Context, input *CodexOAuthSubmitEmailOTPInput) (*CodexOAuthBrowserStageOutput, error) {
	stepInput := CodexOAuthBrowserStepInput{JobId: input.GetJobId(), AccountId: input.GetAccountId(), Label: input.GetLabel(), Session: input.GetSession()}
	return s.codexOAuthProtocolStageStep(ctx, &stepInput, contracts.StepCodexOAuthProtocolEmailOTP, "submitting codex oauth protocol email otp", func(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, _ *pb.Account, data *codexOAuthStepData) (string, int64, error) {
		otp, err := s.consumeStoredOTP(ctx, input.GetJobId(), contracts.JobParamRegistrationOTP, contracts.JobParamRegistrationOTPSubmittedAtUnix, input.GetIssuedAfterUnix())
		if err != nil {
			return "", input.GetIssuedAfterUnix(), err
		}
		resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/email-otp/validate", "https://auth.openai.com/email-verification", map[string]any{"code": channelotpwait.NormalizeCode(otp)})
		if err != nil {
			return "", input.GetIssuedAfterUnix(), err
		}
		stage, err := advanceCodexOAuthProtocolJSON(ctx, client, state, resp, "email_otp", data)
		if err == nil && (stage == "" || stage == "email_otp") {
			if next, navErr := runCodexOAuthProtocolURL(ctx, client, state, state.AuthorizeUrl, "https://auth.openai.com/email-verification", data); navErr == nil && next != "" {
				stage = next
			}
		}
		_ = fetchCodexOAuthClientAuthSessionDump(ctx, client, state, data, stage)
		return codexOAuthProtocolStageFromDump(state, stage), input.GetIssuedAfterUnix(), err
	})
}

func (s *Server) codexOAuthProtocolStageStep(ctx context.Context, input *CodexOAuthBrowserStepInput, stepName, heartbeat string, fn func(context.Context, *GptClient, *pb.CodexOAuthProtocolState, *pb.Account, *codexOAuthStepData) (string, int64, error)) (*CodexOAuthBrowserStageOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := &CodexOAuthBrowserStageOutput{}
	data := codexOAuthProtocolData(label)
	step := s.activityStep(ctx, input.GetJobId(), stepName, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, heartbeat, data.messageData())
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
		stage, issuedAfter, err := fn(ctx, client, state, account, data)
		if stage != "" {
			state.Stage = stage
			data.setStage(stage)
			output.Stage = stage
		}
		if stage == "add_phone" {
			if markErr := s.markCodexOAuthNeedPhone(ctx, gptaccount.ID(account), label, data); markErr != nil {
				data.setAccountPhoneNeedWriteError(markErr)
			}
		}
		if issuedAfter > 0 {
			state.EmailOtpIssuedAfterUnix = issuedAfter
			data.setEmailOTPIssuedAfter(issuedAfter)
			output.EmailOtpIssuedAfterUnix = issuedAfter
		}
		if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
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
