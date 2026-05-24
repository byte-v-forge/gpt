package activities

import "context"

func (s *Server) CodexOAuthDetectBrowserStageActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthBrowserStageStep(ctx, input, stepCodexOAuthBrowserDetect, "detecting codex oauth stage", func(flow *codexOAuthBrowserFlow, accountEmail string) (string, int64, error) {
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, 0, err
	})
}

func (s *Server) CodexOAuthSubmitEmailActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthBrowserStageStep(ctx, input, stepCodexOAuthBrowserEmail, "submitting codex oauth email", func(flow *codexOAuthBrowserFlow, accountEmail string) (string, int64, error) {
		issuedAfter, err := flow.browserFlow.submitCodexOAuthEmail(flow.server.browserAutomationClient, flow.server.browserAuthConfig, accountEmail)
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, issuedAfter, err
	})
}

func (s *Server) CodexOAuthSubmitPasswordActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthBrowserStageStep(ctx, input, stepCodexOAuthBrowserPassword, "submitting codex oauth password", func(flow *codexOAuthBrowserFlow, _ string) (string, int64, error) {
		issuedAfter, err := flow.browserFlow.submitCodexOAuthPassword(flow.server.browserAutomationClient, flow.server.browserAuthConfig, flow.account.GetPassword())
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, issuedAfter, err
	})
}

func (s *Server) CodexOAuthSubmitEmailOTPActivity(ctx context.Context, input CodexOAuthSubmitEmailOTPInput) (CodexOAuthBrowserStageOutput, error) {
	stepInput := CodexOAuthBrowserStepInput{JobId: input.GetJobId(), AccountId: input.GetAccountId(), Label: input.GetLabel(), Session: input.GetSession()}
	return s.codexOAuthBrowserStageStep(ctx, stepInput, stepCodexOAuthBrowserEmailOTP, "submitting codex oauth email otp", func(flow *codexOAuthBrowserFlow, accountEmail string) (string, int64, error) {
		otp, err := s.waitCodexOAuthEmailOTP(ctx, input.GetJobId(), accountEmail, input.GetIssuedAfterUnix())
		if err != nil {
			return "", input.GetIssuedAfterUnix(), err
		}
		if err := flow.browserFlow.submitCodexOAuthOTP(flow.server.browserAutomationClient, flow.server.browserAuthConfig, otp); err != nil {
			return "", input.GetIssuedAfterUnix(), err
		}
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, input.GetIssuedAfterUnix(), err
	})
}

func (s *Server) codexOAuthBrowserStageStep(ctx context.Context, input CodexOAuthBrowserStepInput, stepName, heartbeat string, fn func(*codexOAuthBrowserFlow, string) (string, int64, error)) (CodexOAuthBrowserStageOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthBrowserStageOutput{}
	data := codexOAuthBrowserData(label, nil)
	step := s.activityStep(ctx, input.GetJobId(), stepName, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, heartbeat, data)
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		flow, err := s.newCodexOAuthBrowserSessionFlow(ctx, account, input.GetJobId(), label, nil, cfg, false, false, input.GetSession(), data, stepName)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		stage, issuedAfter, err := fn(flow, account.GetEmail())
		if stage != "" {
			data["login_stage"] = stage
			output.Stage = stage
		}
		if stage == "add_phone" {
			if markErr := s.markCodexOAuthNeedPhone(ctx, account.GetAccountId(), label, data); markErr != nil {
				data["account_phone_need_write_error"] = markErr.Error()
			}
		}
		if issuedAfter > 0 {
			data["email_otp_issued_after_unix"] = issuedAfter
			output.EmailOtpIssuedAfterUnix = issuedAfter
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
