package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nCodexOAuthResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id,omitempty"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Action         string         `json:"action"`
	Success        bool           `json:"success"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
}

func (s *Server) StartN8NCodexOAuth(ctx context.Context, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.CodexOAuthResponse{ErrorMessage: "account_id is required"}, "", fmt.Errorf("account_id is required")
	}
	jobID := uuid.NewString()
	params := codexOAuthJobParams(accountID, req.GetLabel())
	params["engine"] = "n8n"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionCodexOAuth, params); err != nil {
		return &pb.CodexOAuthResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.CodexOAuthResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) BindN8NCodexOAuthExecution(ctx context.Context, jobID string, n8nExecutionID string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if err := s.bindN8NCodexOAuthExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	return map[string]any{"job_id": jobID, "n8n_execution_id": n8nExecutionID, "success": true}, nil
}

func (s *Server) CodexOAuthStartBrowser(ctx context.Context, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error) {
	out, err := s.activities.CodexOAuthStartBrowserActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthDetectBrowserStage(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthDetectBrowserStageActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthSubmitEmail(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthSubmitEmailActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthSubmitPassword(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthSubmitPasswordActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthSubmitEmailOTP(ctx context.Context, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthSubmitEmailOTPActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthAddPhoneBrowser(ctx context.Context, req *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error) {
	out, err := s.activities.CodexOAuthAddPhoneBrowserActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthCompleteBrowser(ctx context.Context, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error) {
	out, err := s.activities.CodexOAuthCompleteBrowserActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthStopBrowser(ctx context.Context, req *pb.CodexOAuthStopBrowserInput) (any, error) {
	if err := s.activities.CodexOAuthStopBrowserActivity(ctx, *req); err != nil {
		return nil, err
	}
	return map[string]any{"job_id": strings.TrimSpace(req.GetJobId()), "success": true}, nil
}

func (s *Server) CompleteN8NCodexOAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, result map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]any{}
	}
	result["account_id"] = accountID
	result["n8n_execution_id"] = n8nExecutionID
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	return &n8nCodexOAuthResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuth, Success: true, Result: result}, nil
}

func (s *Server) FailN8NCodexOAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "codex oauth failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	step = strings.TrimSpace(step)
	if step == "" {
		step = s.codexOAuthFailureStep(ctx, jobID)
	}
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRecoverable, true, false, err, data); markErr != nil {
		return nil, markErr
	}
	return &n8nCodexOAuthResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuth, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) bindN8NCodexOAuthExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) codexOAuthFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepCodexOAuthBrowserStart
}
