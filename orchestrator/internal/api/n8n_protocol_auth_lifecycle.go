package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nProtocolAuthLifecycleConfig struct {
	Mode               string
	ResultSecretPrefix string
	StartStep          string
	WaitStep           string
	CompleteStep       string
	Failure            n8nAuthFailureConfig
}

func (s *Server) useN8NProtocolProxy(ctx context.Context, profile n8nDynamicProxyProfile, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	profile = profile.normalized()
	if profile.ProtocolMode == "" {
		return nil, fmt.Errorf("protocol mode is required")
	}
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolUseProxyActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, profile.ProtocolMode))
	result := n8nProtocolStepResult(jobID, accountID, n8nExecutionID, contracts.StepProtocolUseProxy)
	if err != nil {
		return result, s.markN8NAuthFailed(ctx, n8nAuthFailureConfig{}, jobID, contracts.StepProtocolUseProxy, err, out.GetData())
	}
	result.Success = true
	return result, nil
}

func (s *Server) startN8NProtocolAuth(ctx context.Context, cfg n8nProtocolAuthLifecycleConfig, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthStartActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, cfg.Mode))
	result, resultErr := s.n8nProtocolAuthProgressResult(ctx, cfg.ResultSecretPrefix, jobID, accountID, n8nExecutionID, cfg.StartStep, out)
	if err != nil {
		return result, s.markN8NAuthFailed(ctx, cfg.Failure, jobID, cfg.StartStep, err, out.GetData())
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) waitN8NProtocolAuth(ctx context.Context, cfg n8nProtocolAuthLifecycleConfig, jobID string, accountID string, n8nExecutionID string, flowID string, email string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthWaitActivity(ctx, &pb.ProtocolAuthWaitInput{JobId: jobID, AccountId: accountID, FlowId: strings.TrimSpace(flowID), Mode: cfg.Mode, Email: strings.TrimSpace(email), ProxyUrl: s.protocolProxyURL(ctx, jobID)})
	result, resultErr := s.n8nProtocolAuthProgressResult(ctx, cfg.ResultSecretPrefix, jobID, accountID, n8nExecutionID, cfg.WaitStep, out)
	if err != nil {
		return result, s.markN8NAuthFailed(ctx, cfg.Failure, jobID, cfg.WaitStep, err, out.GetData())
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) completeN8NProtocolAuth(ctx context.Context, cfg n8nProtocolAuthLifecycleConfig, jobID string, accountID string, n8nExecutionID string, flowID string, otpSource string, otpIssuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	flowID = strings.TrimSpace(flowID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthCompleteActivity(ctx, &pb.ProtocolAuthCompleteInput{JobId: jobID, AccountId: accountID, FlowId: flowID, Mode: cfg.Mode, OtpParam: contracts.JobParamRegistrationOTP, SubmittedAtParam: contracts.JobParamRegistrationOTPSubmittedAtUnix, OtpIssuedAfterUnix: otpIssuedAfterUnix, OtpSource: strings.TrimSpace(otpSource), ProxyUrl: s.protocolProxyURL(ctx, jobID)})
	result, resultErr := s.n8nProtocolAuthProgressResult(ctx, cfg.ResultSecretPrefix, jobID, accountID, n8nExecutionID, cfg.CompleteStep, &registerOutputProgress{accountID: accountID, flowID: flowID, result: out})
	if err != nil {
		return result, s.markN8NAuthFailed(ctx, cfg.Failure, jobID, cfg.CompleteStep, err, out.GetData())
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}
