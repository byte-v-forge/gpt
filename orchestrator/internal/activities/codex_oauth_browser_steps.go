package activities

import (
	"context"
	"orchestrator/internal/contracts"
	"orchestrator/internal/gptaccount"
)

func (s *Server) CodexOAuthDetectBrowserStageActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthBrowserStageStep(ctx, input, contracts.StepCodexOAuthBrowserDetect, "detecting codex oauth stage", func(flow *codexOAuthBrowserFlow, accountEmail string) (string, int64, error) {
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, 0, err
	})
}

func (s *Server) CodexOAuthSubmitEmailActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthBrowserStageStep(ctx, input, contracts.StepCodexOAuthBrowserEmail, "submitting codex oauth email", func(flow *codexOAuthBrowserFlow, accountEmail string) (string, int64, error) {
		issuedAfter, err := flow.browserFlow.submitCodexOAuthEmail(flow.server.browserAutomationClient, flow.server.browserAuthConfig, accountEmail)
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, issuedAfter, err
	})
}

func (s *Server) CodexOAuthSubmitPasswordActivity(ctx context.Context, input CodexOAuthBrowserStepInput) (CodexOAuthBrowserStageOutput, error) {
	return s.codexOAuthBrowserStageStep(ctx, input, contracts.StepCodexOAuthBrowserPassword, "submitting codex oauth password", func(flow *codexOAuthBrowserFlow, _ string) (string, int64, error) {
		issuedAfter, err := flow.browserFlow.submitCodexOAuthPassword(flow.server.browserAutomationClient, flow.server.browserAuthConfig, flow.browserFlow.password)
		if err != nil {
			return "", issuedAfter, err
		}
		stage, err := flow.browserFlow.detectCodexOAuthStage(flow.server.browserAutomationClient, flow.server.browserAuthConfig)
		return stage, issuedAfter, err
	})
}

func (s *Server) CodexOAuthSubmitEmailOTPActivity(ctx context.Context, input CodexOAuthSubmitEmailOTPInput) (CodexOAuthBrowserStageOutput, error) {
	stepInput := CodexOAuthBrowserStepInput{JobId: input.GetJobId(), AccountId: input.GetAccountId(), Label: input.GetLabel(), Session: input.GetSession()}
	return s.codexOAuthBrowserStageStep(ctx, stepInput, contracts.StepCodexOAuthBrowserEmailOTP, "submitting codex oauth email otp", func(flow *codexOAuthBrowserFlow, _ string) (string, int64, error) {
		otp, err := s.consumeStoredOTP(ctx, input.GetJobId(), contracts.JobParamRegistrationOTP, contracts.JobParamRegistrationOTPSubmittedAtUnix, input.GetIssuedAfterUnix())
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
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := CodexOAuthBrowserStageOutput{}
	data := codexOAuthBrowserData(label, nil)
	step := s.activityStep(ctx, input.GetJobId(), stepName, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, heartbeat, data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		flow, err := s.newCodexOAuthBrowserSessionFlow(ctx, account, input.GetJobId(), label, nil, cfg, false, false, input.GetSession(), data, stepName)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		stage, issuedAfter, err := fn(flow, gptaccount.Email(account))
		if stage != "" {
			data.setStage(stage)
			output.Stage = stage
		}
		if stage == "add_phone" {
			if markErr := s.markCodexOAuthNeedPhone(ctx, gptaccount.ID(account), label, data); markErr != nil {
				data.setAccountPhoneNeedWriteError(markErr)
			}
		}
		if issuedAfter > 0 {
			data.setEmailOTPIssuedAfter(issuedAfter)
			output.EmailOtpIssuedAfterUnix = issuedAfter
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
