package api

import (
	"context"
	"strings"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nCodexOAuthFlowSteps struct {
	Start          func(context.Context, *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error)
	Detect         func(context.Context, *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	SubmitEmail    func(context.Context, *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	SubmitPassword func(context.Context, *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	SubmitEmailOTP func(context.Context, *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error)
	AddPhone       func(context.Context, *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error)
	Complete       func(context.Context, *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error)
	Stop           func(context.Context, *pb.CodexOAuthStopBrowserInput) error
}

func (s *Server) n8nCodexOAuthBrowserSteps() n8nCodexOAuthFlowSteps {
	return n8nCodexOAuthFlowSteps{
		Start:          s.activities.CodexOAuthStartBrowserActivity,
		Detect:         s.activities.CodexOAuthDetectBrowserStageActivity,
		SubmitEmail:    s.activities.CodexOAuthSubmitEmailActivity,
		SubmitPassword: s.activities.CodexOAuthSubmitPasswordActivity,
		SubmitEmailOTP: s.activities.CodexOAuthSubmitEmailOTPActivity,
		AddPhone:       s.activities.CodexOAuthAddPhoneBrowserActivity,
		Complete:       s.activities.CodexOAuthCompleteBrowserActivity,
		Stop:           s.activities.CodexOAuthStopBrowserActivity,
	}
}

func (s *Server) n8nCodexOAuthProtocolSteps() n8nCodexOAuthFlowSteps {
	return n8nCodexOAuthFlowSteps{
		Start:          s.activities.CodexOAuthStartProtocolActivity,
		Detect:         s.activities.CodexOAuthDetectProtocolStageActivity,
		SubmitEmail:    s.activities.CodexOAuthSubmitProtocolEmailActivity,
		SubmitPassword: s.activities.CodexOAuthSubmitProtocolPasswordActivity,
		SubmitEmailOTP: s.activities.CodexOAuthSubmitProtocolEmailOTPActivity,
		AddPhone:       s.activities.CodexOAuthAddPhoneProtocolActivity,
		Complete:       s.activities.CodexOAuthCompleteProtocolActivity,
		Stop:           s.activities.CodexOAuthStopProtocolActivity,
	}
}

func (s *Server) CodexOAuthStartFlow(ctx context.Context, actionID string, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.Start(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthDetectFlowStage(ctx context.Context, actionID string, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.Detect(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthSubmitFlowEmail(ctx context.Context, actionID string, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.SubmitEmail(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthSubmitFlowPassword(ctx context.Context, actionID string, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.SubmitPassword(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthAwaitFlowEmailOTP(ctx context.Context, actionID string, req *pb.N8NAuthOTPWaitRequest) (*pb.N8NChannelOTPWaitResult, error) {
	profile, err := n8nCodexOAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	cfg := s.channelOTPWaitConfig(channelotpwait.ChannelEmail, profile.OTP)
	target := strings.TrimSpace(req.GetTarget())
	if target == "" {
		target, err = s.n8nAuthAccountEmail(ctx, req.GetAccountId())
		if err != nil {
			return nil, err
		}
	}
	return s.awaitN8NChannelOTP(ctx, n8nChannelOTPWaitRequest{
		Action:           actionID,
		JobID:            req.GetJobId(),
		AccountID:        req.GetAccountId(),
		N8NExecutionID:   req.GetN8NExecutionId(),
		FlowID:           req.GetFlowId(),
		Channel:          channelotpwait.ChannelEmail,
		Target:           target,
		StepName:         cfg.StepName,
		TimeoutSeconds:   req.GetTimeoutSeconds(),
		OTPIssuedAfter:   req.GetOtpIssuedAfterUnix(),
		ResumeURL:        req.GetResumeUrl(),
		OTPParam:         contracts.JobParamRegistrationOTP,
		SubmittedAtParam: contracts.JobParamRegistrationOTPSubmittedAtUnix,
	}, cfg)
}

func (s *Server) CodexOAuthSubmitFlowEmailOTP(ctx context.Context, actionID string, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.SubmitEmailOTP(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthAddPhoneFlow(ctx context.Context, actionID string, req *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.AddPhone(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthCompleteFlow(ctx context.Context, actionID string, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	out, err := steps.Complete(ctx, req)
	return out, err
}

func (s *Server) CodexOAuthStopFlow(ctx context.Context, actionID string, req *pb.CodexOAuthStopBrowserInput) (any, error) {
	steps, err := s.n8nCodexOAuthFlowStepsForAction(actionID)
	if err != nil {
		return nil, err
	}
	if err := steps.Stop(ctx, req); err != nil {
		return nil, err
	}
	return n8nJobSuccess(req.GetJobId()), nil
}

func (s *Server) n8nCodexOAuthFlowStepsForAction(actionID string) (n8nCodexOAuthFlowSteps, error) {
	definition, ok := n8nCodexOAuthProfileDefinitionForAction(actionID)
	if !ok {
		return n8nCodexOAuthFlowSteps{}, unsupportedN8NAuthActionError("codex oauth flow", contracts.NormalizeActionID(actionID))
	}
	switch definition.Flow {
	case n8nCodexOAuthBrowserFlow:
		return s.n8nCodexOAuthBrowserSteps(), nil
	case n8nCodexOAuthProtocolFlow:
		return s.n8nCodexOAuthProtocolSteps(), nil
	default:
		return n8nCodexOAuthFlowSteps{}, unsupportedN8NAuthActionError("codex oauth flow", contracts.NormalizeActionID(actionID))
	}
}
