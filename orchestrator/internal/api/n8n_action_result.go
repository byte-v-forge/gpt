package api

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
	"orchestrator/internal/protowrap"
	"orchestrator/pb"
)

type n8nActionSuccessConfig struct {
	Action  string
	Started bool
}

type n8nActionFailureStoreConfig struct {
	Action              string
	Started             bool
	DefaultMessage      string
	FailureStepFallback string
	Status              string
	Recoverable         bool
	Retryable           bool
}

func (cfg n8nActionSuccessConfig) withAction(profile contracts.ActionProfile) n8nActionSuccessConfig {
	cfg.Action = profile.ActionID
	return cfg
}

func (cfg n8nActionFailureStoreConfig) withAction(profile contracts.ActionProfile) n8nActionFailureStoreConfig {
	cfg.Action = profile.ActionID
	cfg.DefaultMessage = firstNonEmpty(cfg.DefaultMessage, profile.FailureMessageOrDefault())
	return cfg
}

type n8nActionFailureRecord struct {
	Step          string
	Status        string
	Recoverable   bool
	Retryable     bool
	ErrorMessage  string
	ResultMessage proto.Message
}

type n8nActionScope struct {
	JobID          string
	AccountID      string
	N8NExecutionID string
}

func n8nActionScopeFrom(jobID string, accountID string, n8nExecutionID string) n8nActionScope {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	return n8nActionScope{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID}
}

func (scope n8nActionScope) stepResult(step string, success bool) *pb.N8NActionStepResult {
	return &pb.N8NActionStepResult{
		JobId:          scope.JobID,
		AccountId:      scope.AccountID,
		N8NExecutionId: scope.N8NExecutionID,
		Step:           strings.TrimSpace(step),
		Success:        success,
	}
}

func (scope n8nActionScope) stepResultMessage(step string, success bool, data proto.Message) *pb.N8NActionStepResult {
	return &pb.N8NActionStepResult{
		JobId:          scope.JobID,
		AccountId:      scope.AccountID,
		N8NExecutionId: scope.N8NExecutionID,
		Step:           strings.TrimSpace(step),
		Success:        success,
		Data:           n8nActionStepData(data),
	}
}

func n8nActionStep(jobID string, accountID string, n8nExecutionID string, step string, success bool) *pb.N8NActionStepResult {
	return n8nActionScopeFrom(jobID, accountID, n8nExecutionID).stepResult(step, success)
}

func n8nActionStepData(data proto.Message) *pb.N8NActionStepData {
	out := &pb.N8NActionStepData{}
	if !protowrap.SetMessage(out, data) {
		return nil
	}
	return out
}

func n8nActionSuccess(result *pb.N8NActionSuccessResult) *pb.N8NActionSuccessResult {
	if result == nil {
		result = &pb.N8NActionSuccessResult{}
	}
	result.Success = true
	result.JobId = strings.TrimSpace(result.GetJobId())
	result.ParentJobId = strings.TrimSpace(result.GetParentJobId())
	result.AccountId = strings.TrimSpace(result.GetAccountId())
	result.ActivationId = strings.TrimSpace(result.GetActivationId())
	result.Label = strings.TrimSpace(result.GetLabel())
	result.N8NExecutionId = strings.TrimSpace(result.GetN8NExecutionId())
	return result
}

func n8nJobSuccess(jobID string) *pb.N8NActionSuccessResult {
	return n8nActionSuccess(&pb.N8NActionSuccessResult{JobId: jobID})
}

func n8nActionCompleteOutcomeMessage(jobID string, accountID string, n8nExecutionID string, action string, started bool, success bool, errorMessage string, result proto.Message) *pb.N8NActionCompleteResult {
	scope := n8nActionScopeFrom(jobID, accountID, n8nExecutionID)
	return &pb.N8NActionCompleteResult{
		JobId:          scope.JobID,
		AccountId:      scope.AccountID,
		N8NExecutionId: scope.N8NExecutionID,
		Action:         strings.TrimSpace(action),
		Started:        started,
		Success:        success,
		ErrorMessage:   strings.TrimSpace(errorMessage),
		Result:         n8nActionCompleteData(result),
	}
}

func n8nActionCompleteData(result proto.Message) *pb.N8NActionCompleteData {
	out := &pb.N8NActionCompleteData{}
	if !protowrap.SetMessage(out, result) {
		return nil
	}
	return out
}

func (s *Server) failStoredN8NActionMessage(ctx context.Context, cfg n8nActionFailureStoreConfig, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, result proto.Message) (any, error) {
	scope := n8nActionScopeFrom(jobID, accountID, n8nExecutionID)
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage == "" {
		errorMessage = strings.TrimSpace(cfg.DefaultMessage)
	}
	if errorMessage == "" {
		errorMessage = "action failed"
	}
	if step = strings.TrimSpace(step); step == "" {
		step = s.n8nActionFailureStep(ctx, scope.JobID, cfg.FailureStepFallback)
	}
	if step == "" {
		step = strings.TrimSpace(cfg.Action)
	}
	status := strings.TrimSpace(cfg.Status)
	if status == "" {
		status = jobstatus.FailedRetryable
	}
	if err := s.storeN8NActionFailure(ctx, scope.JobID, n8nActionFailureRecord{
		Step:          step,
		Status:        status,
		Recoverable:   cfg.Recoverable,
		Retryable:     cfg.Retryable,
		ErrorMessage:  errorMessage,
		ResultMessage: result,
	}); err != nil {
		return nil, err
	}
	return n8nActionCompleteOutcomeMessage(scope.JobID, scope.AccountID, scope.N8NExecutionID, cfg.Action, cfg.Started, false, errorMessage, result), nil
}

func (s *Server) failBoundN8NActionMessage(ctx context.Context, cfg n8nActionFailureStoreConfig, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data proto.Message) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	return s.failStoredN8NActionMessage(ctx, cfg, jobID, accountID, n8nExecutionID, step, errorMessage, data)
}

func (s *Server) storeN8NActionSuccessMessage(ctx context.Context, jobID string, result proto.Message) error {
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{
		JobId:  strings.TrimSpace(jobID),
		Result: jobDataMessage(result),
	})
}

func (s *Server) storeN8NActionFailure(ctx context.Context, jobID string, record n8nActionFailureRecord) error {
	errorMessage := strings.TrimSpace(record.ErrorMessage)
	if errorMessage == "" {
		errorMessage = "action failed"
	}
	status := strings.TrimSpace(record.Status)
	if status == "" {
		status = jobstatus.FailedRetryable
	}
	return s.activities.MarkJobFailedActivity(ctx, pb.JobFailureInput{
		JobId:        strings.TrimSpace(jobID),
		StepName:     strings.TrimSpace(record.Step),
		Status:       status,
		Recoverable:  record.Recoverable,
		Retryable:    record.Retryable,
		ErrorMessage: errorMessage,
		Result:       jobDataMessage(record.ResultMessage),
	})
}

func (s *Server) markN8NActionFailure(ctx context.Context, jobID string, record n8nActionFailureRecord, err error) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(record.ErrorMessage) == "" {
		record.ErrorMessage = err.Error()
	}
	if markErr := s.storeN8NActionFailure(ctx, jobID, record); markErr != nil {
		return errors.Join(err, markErr)
	}
	return err
}

func (s *Server) n8nActionFailureStep(ctx context.Context, jobID string, fallback string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return strings.TrimSpace(fallback)
}
