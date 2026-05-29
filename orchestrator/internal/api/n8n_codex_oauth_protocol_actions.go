package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) StartN8NCodexOAuthProtocol(ctx context.Context, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.CodexOAuthResponse{ErrorMessage: "account_id is required"}, "", fmt.Errorf("account_id is required")
	}
	jobID := uuid.NewString()
	params := codexOAuthJobParams(accountID, req.GetLabel())
	params["engine"] = "n8n"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionCodexOAuthProtocol, params); err != nil {
		return &pb.CodexOAuthResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.CodexOAuthResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) N8NCodexOAuthProtocolDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	return s.n8nDynamicProxySettings(ctx, jobID, accountID, n8nExecutionID, actionCodexOAuthProtocol, s.bindN8NCodexOAuthProtocolExecution)
}

func (s *Server) RecordN8NCodexOAuthProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error) {
	return s.recordN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, proxyURL, data, actionCodexOAuthProtocol, s.bindN8NCodexOAuthProtocolExecution)
}

func (s *Server) FailN8NCodexOAuthProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	return s.failN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, errorMessage, data, s.bindN8NCodexOAuthProtocolExecution)
}

func (s *Server) UseN8NCodexOAuthProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolUseProxyActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, codexOAuthProtocolMode))
	result := n8nProtocolStepResult(jobID, accountID, n8nExecutionID, stepProtocolUseProxy, out.GetData())
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProtocolUseProxy, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	result.Success = true
	return result, nil
}

func (s *Server) CodexOAuthStartProtocol(ctx context.Context, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error) {
	out, err := s.activities.CodexOAuthStartProtocolActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthDetectProtocolStage(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthDetectProtocolStageActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthSubmitProtocolEmail(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthSubmitProtocolEmailActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthSubmitProtocolPassword(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthSubmitProtocolPasswordActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthSubmitProtocolEmailOTP(ctx context.Context, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error) {
	out, err := s.activities.CodexOAuthSubmitProtocolEmailOTPActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthCompleteProtocol(ctx context.Context, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error) {
	out, err := s.activities.CodexOAuthCompleteProtocolActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthStopProtocol(ctx context.Context, req *pb.CodexOAuthStopBrowserInput) (any, error) {
	if err := s.activities.CodexOAuthStopProtocolActivity(ctx, *req); err != nil {
		return nil, err
	}
	return map[string]any{"job_id": strings.TrimSpace(req.GetJobId()), "success": true}, nil
}

func (s *Server) CompleteN8NCodexOAuthProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, result map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
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
	return &n8nCodexOAuthResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuthProtocol, Success: true, Result: result}, nil
}

func (s *Server) FailN8NCodexOAuthProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "codex oauth protocol failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	step = strings.TrimSpace(step)
	if step == "" {
		step = s.codexOAuthProtocolFailureStep(ctx, jobID)
	}
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRecoverable, true, false, err, data); markErr != nil {
		return nil, markErr
	}
	return &n8nCodexOAuthResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuthProtocol, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) bindN8NCodexOAuthProtocolExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) codexOAuthProtocolFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepCodexOAuthProtocolStart
}
