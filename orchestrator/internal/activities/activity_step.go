package activities

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type activityStep struct {
	server      *Server
	ctx         context.Context
	jobID       string
	stepName    string
	recoverable bool
	retryable   bool
}

func (s *Server) startActivityStep(ctx context.Context, jobID, stepName string, recoverable bool, retryable bool) (activityStep, error) {
	step := activityStep{
		server:      s,
		ctx:         ctx,
		jobID:       jobID,
		stepName:    stepName,
		recoverable: recoverable,
		retryable:   retryable,
	}
	if err := s.jobStore.StartStep(ctx, jobID, stepName, recoverable, retryable); err != nil {
		return activityStep{}, err
	}
	return step, nil
}

func (s *Server) StartJobStepActivity(ctx context.Context, input JobStepStartInput) error {
	jobID := strings.TrimSpace(input.GetJobId())
	stepName := strings.TrimSpace(input.GetStepName())
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	if stepName == "" {
		return fmt.Errorf("step_name is required")
	}
	step, err := s.startActivityStep(ctx, jobID, stepName, input.GetRecoverable(), input.GetRetryable())
	if err != nil {
		return err
	}
	if detail := protoDataMap(input.GetDetail()); len(detail) > 0 {
		step.update(detail)
	}
	return nil
}

func (s *Server) CompleteJobStepActivity(ctx context.Context, input JobStepCompleteInput) error {
	jobID := strings.TrimSpace(input.GetJobId())
	stepName := strings.TrimSpace(input.GetStepName())
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	if stepName == "" {
		return fmt.Errorf("step_name is required")
	}
	step := s.activityStep(ctx, jobID, stepName, input.GetRecoverable(), input.GetRetryable())
	return step.complete(protoDataMap(input.GetResult()), nil)
}

func (s *Server) activityStep(ctx context.Context, jobID, stepName string, recoverable bool, retryable bool) activityStep {
	return activityStep{
		server:      s,
		ctx:         ctx,
		jobID:       jobID,
		stepName:    stepName,
		recoverable: recoverable,
		retryable:   retryable,
	}
}

func (s *Server) completeActivityStep(ctx context.Context, jobID, stepName string, recoverable bool, retryable bool, data map[string]any, stepErr error) error {
	return s.jobStore.CompleteStep(ctx, jobID, stepName, recoverable, retryable, data, stepErr)
}

func (s *Server) updateActivityStepData(ctx context.Context, jobID, stepName string, data map[string]any) {
	s.updateRunningStepData(ctx, jobID, stepName, data)
}

func (s *Server) progressActivityStep(ctx context.Context, jobID, stepName, message string, fields map[string]any) {
	s.recordActivityProgress(ctx, jobID, stepName, message, fields)
}

func (step activityStep) complete(data map[string]any, stepErr error) error {
	return step.server.completeActivityStep(step.ctx, step.jobID, step.stepName, step.recoverable, step.retryable, data, stepErr)
}

func (step activityStep) run(fn func() (any, error)) (any, error) {
	return step.server.runAtomicStep(step.ctx, step.jobID, step.stepName, step.recoverable, step.retryable, fn)
}

func (step activityStep) update(data map[string]any) {
	step.server.updateActivityStepData(step.ctx, step.jobID, step.stepName, data)
}

func (step activityStep) progress(message string, fields map[string]any) {
	step.server.progressActivityStep(step.ctx, step.jobID, step.stepName, message, fields)
}

func (step activityStep) progressEvery(last *time.Time, message string, fields map[string]any) {
	step.server.recordActivityProgressEvery(step.ctx, last, step.jobID, step.stepName, message, fields)
}
