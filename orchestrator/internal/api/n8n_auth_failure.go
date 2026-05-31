package api

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nAuthFailureConfig struct {
	Status      func(error) string
	Recoverable func(error) bool
	Retryable   func(error) bool
}

type n8nAuthFailConfig struct {
	Action              string
	Mode                string
	DefaultMessage      string
	FailureStepFallback string
	BrowserCancel       bool
	ProtocolFlowParam   string
	Failure             n8nAuthFailureConfig
}

func (cfg n8nAuthFailConfig) withAction(profile contracts.ActionProfile) n8nAuthFailConfig {
	cfg.Action = profile.ActionID
	cfg.DefaultMessage = firstNonEmpty(cfg.DefaultMessage, profile.FailureMessageOrDefault())
	return cfg
}

func n8nRegisterAuthFailure() n8nAuthFailureConfig {
	return n8nAuthFailureConfig{Status: registerProtocolFailureStatus, Recoverable: registerProtocolRecoverable, Retryable: registerProtocolRetryable}
}

func (s *Server) failN8NAuth(ctx context.Context, cfg n8nAuthFailConfig, jobID string, accountID string, n8nExecutionID string, flowID string, errorMessage string, data *pb.N8NAuthFailureData) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage == "" {
		errorMessage = strings.TrimSpace(cfg.DefaultMessage)
	}
	if errorMessage == "" {
		errorMessage = "auth failed"
	}
	data = n8nAuthFailureData(n8nActionScopeFrom(jobID, accountID, n8nExecutionID), flowID, cfg.Mode, errorMessage, data)
	err := fmt.Errorf("%s", errorMessage)
	result, markErr := s.failStoredN8NActionMessage(ctx, n8nActionFailureStoreConfig{
		Action:              cfg.Action,
		Started:             true,
		FailureStepFallback: cfg.FailureStepFallback,
		Status:              cfg.Failure.status(err),
		Recoverable:         cfg.Failure.recoverable(err),
		Retryable:           cfg.Failure.retryable(err),
	}, jobID, accountID, n8nExecutionID, "", errorMessage, data)
	if markErr != nil {
		return nil, markErr
	}
	if cfg.BrowserCancel {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: strings.TrimSpace(flowID), Mode: cfg.Mode})
	} else if strings.TrimSpace(cfg.ProtocolFlowParam) != "" {
		_ = s.cancelN8NProtocolAuthState(ctx, jobID, firstNonEmpty(flowID, data.GetFlowId()), cfg.ProtocolFlowParam, cfg.Mode)
	}
	return result, nil
}

func (s *Server) markN8NAuthFailed(ctx context.Context, cfg n8nAuthFailureConfig, jobID string, step string, err error, data proto.Message) error {
	return s.markN8NActionFailure(ctx, jobID, n8nActionFailureRecord{
		Step:          step,
		Status:        cfg.status(err),
		Recoverable:   cfg.recoverable(err),
		Retryable:     cfg.retryable(err),
		ResultMessage: data,
	}, err)
}

func (cfg n8nAuthFailureConfig) status(err error) string {
	if cfg.Status != nil {
		return cfg.Status(err)
	}
	return jobstatus.FailedRetryable
}

func (cfg n8nAuthFailureConfig) recoverable(err error) bool {
	if cfg.Recoverable != nil {
		return cfg.Recoverable(err)
	}
	return false
}

func (cfg n8nAuthFailureConfig) retryable(err error) bool {
	if cfg.Retryable != nil {
		return cfg.Retryable(err)
	}
	return true
}

func (s *Server) cancelN8NProtocolAuthState(ctx context.Context, jobID string, flowID string, flowParam string, mode string) error {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		params, err := s.jobStore.Params(ctx, jobID)
		if err == nil {
			flowID = strings.TrimSpace(params[strings.TrimSpace(flowParam)])
		}
	}
	if flowID == "" {
		return nil
	}
	return s.activities.ProtocolAuthCancelActivity(ctx, pb.ProtocolAuthCancelInput{JobId: jobID, FlowId: flowID, Mode: mode})
}

func n8nAuthFailureData(scope n8nActionScope, flowID string, mode string, errorMessage string, data *pb.N8NAuthFailureData) *pb.N8NAuthFailureData {
	if data == nil {
		data = &pb.N8NAuthFailureData{}
	}
	data.AccountId = strings.TrimSpace(scope.AccountID)
	data.N8NExecutionId = strings.TrimSpace(scope.N8NExecutionID)
	data.FlowId = firstNonEmpty(data.GetFlowId(), flowID)
	data.Mode = firstNonEmpty(data.GetMode(), mode)
	data.ErrorMessage = firstNonEmpty(errorMessage, data.GetErrorMessage())
	if strings.TrimSpace(data.GetReason()) == "" {
		data.Reason = firstNonEmpty(data.GetStage(), "auth_failed")
	}
	return data
}
