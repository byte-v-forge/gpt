package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) StartN8NCodexOAuthBatchAddPhone(ctx context.Context, req *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, error) {
	accountIDs := compactAccountIDs(req.GetAccountIds())
	if len(accountIDs) == 0 {
		return &pb.CodexOAuthBatchAddPhoneResponse{ErrorMessage: "account_ids is required"}, fmt.Errorf("account_ids is required")
	}
	jobID := uuid.NewString()
	params := codexOAuthBatchAddPhoneJobParams(accountIDs, req.GetLabel(), req.GetMaxReuseCount())
	params["engine"] = "n8n"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, "", actionCodexOAuthBatchAddPhone, params); err != nil {
		return &pb.CodexOAuthBatchAddPhoneResponse{JobId: jobID, ErrorMessage: err.Error()}, err
	}
	return &pb.CodexOAuthBatchAddPhoneResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) CreateN8NCodexOAuthBatchAddPhoneChild(ctx context.Context, parentJobID string, accountID string, label string, maxReuseCount int32, n8nExecutionID string) (any, error) {
	parentJobID = strings.TrimSpace(parentJobID)
	accountID = strings.TrimSpace(accountID)
	if parentJobID == "" || accountID == "" {
		return nil, fmt.Errorf("parent_job_id and account_id are required")
	}
	if err := s.bindN8NCodexOAuthBatchExecution(ctx, parentJobID, n8nExecutionID); err != nil {
		return nil, err
	}
	childJobID := uuid.NewString()
	params := codexOAuthAddPhoneJobParams(accountID, label, maxReuseCount)
	params["engine"] = "n8n"
	params["parent_job_id"] = parentJobID
	if _, err := s.jobStore.CreateWithID(ctx, childJobID, accountID, actionCodexOAuthAddPhone, params); err != nil {
		return nil, err
	}
	return map[string]any{
		"parent_job_id":    parentJobID,
		"job_id":           childJobID,
		"account_id":       accountID,
		"label":            strings.TrimSpace(label),
		"max_reuse_count":  maxReuseCount,
		"n8n_execution_id": n8nExecutionID,
		"success":          true,
	}, nil
}

func (s *Server) CompleteN8NCodexOAuthBatchAddPhone(ctx context.Context, jobID string, n8nExecutionID string, result map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	if err := s.bindN8NCodexOAuthBatchExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]any{}
	}
	result["n8n_execution_id"] = strings.TrimSpace(n8nExecutionID)
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	return &n8nCodexOAuthResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuthBatchAddPhone, Success: true, Result: result}, nil
}

func (s *Server) FailN8NCodexOAuthBatchAddPhone(ctx context.Context, jobID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	if err := s.bindN8NCodexOAuthBatchExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "codex oauth batch add phone failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	data["n8n_execution_id"] = strings.TrimSpace(n8nExecutionID)
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, "codex_oauth_batch_add_phone", jobstatus.FailedRetryable, false, true, err, data); markErr != nil {
		return nil, markErr
	}
	return &n8nCodexOAuthResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuthBatchAddPhone, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) bindN8NCodexOAuthBatchExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}
