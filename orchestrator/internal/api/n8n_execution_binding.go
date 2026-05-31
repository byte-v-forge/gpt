package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) bindN8NExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) bindN8NExecutionAction(ctx context.Context, jobID string, executionID string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	executionID = strings.TrimSpace(executionID)
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if err := s.bindN8NExecution(ctx, jobID, executionID); err != nil {
		return nil, err
	}
	return n8nActionSuccess(&pb.N8NActionSuccessResult{JobId: jobID, N8NExecutionId: executionID}), nil
}
