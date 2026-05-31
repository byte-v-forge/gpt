package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nBrowserAuthLifecycleConfig struct {
	Mode               string
	ResultSecretPrefix string
	StartStep          string
	CompleteStep       string
	ResultLabel        string
	Failure            n8nAuthFailureConfig
}

func (s *Server) startN8NBrowserAuth(ctx context.Context, cfg n8nBrowserAuthLifecycleConfig, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.BrowserAuthStartActivity(ctx, pb.BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: cfg.Mode})
	result := s.n8nBrowserAuthStartResult(ctx, cfg.ResultSecretPrefix, jobID, accountID, n8nExecutionID, cfg.StartStep, out)
	if err != nil {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: out.GetBrowserSessionId(), Mode: cfg.Mode})
		return result, s.markN8NAuthFailed(ctx, cfg.Failure, jobID, cfg.StartStep, err, out.GetData())
	}
	if result.GetResultReady() && strings.TrimSpace(result.GetResultRef()) == "" {
		return result, fmt.Errorf("failed to persist %s result", n8nAuthResultLabel(cfg.ResultLabel))
	}
	if errMessage := strings.TrimSpace(result.GetResultSecretError()); errMessage != "" {
		return result, fmt.Errorf("failed to persist %s result: %s", n8nAuthResultLabel(cfg.ResultLabel), errMessage)
	}
	return result, nil
}

func (s *Server) completeN8NBrowserAuth(ctx context.Context, cfg n8nBrowserAuthLifecycleConfig, jobID string, accountID string, n8nExecutionID string, flowID string, otpSource string, otpIssuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	flowID = strings.TrimSpace(flowID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.BrowserAuthCompleteActivity(ctx, pb.BrowserAuthCompleteInput{JobId: jobID, AccountId: accountID, BrowserSessionId: flowID, Mode: cfg.Mode, OtpParam: contracts.JobParamRegistrationOTP, SubmittedAtParam: contracts.JobParamRegistrationOTPSubmittedAtUnix, OtpIssuedAfterUnix: otpIssuedAfterUnix, OtpSource: strings.TrimSpace(otpSource)})
	result, resultErr := s.n8nBrowserAuthOutputResult(ctx, cfg.ResultSecretPrefix, jobID, accountID, n8nExecutionID, cfg.CompleteStep, flowID, &out)
	if err != nil {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: flowID, Mode: cfg.Mode})
		return result, s.markN8NAuthFailed(ctx, cfg.Failure, jobID, cfg.CompleteStep, err, out.GetData())
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}
