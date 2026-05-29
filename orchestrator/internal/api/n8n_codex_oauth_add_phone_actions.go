package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) StartN8NCodexOAuthAddPhone(ctx context.Context, req *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, string, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.CodexOAuthAddPhoneResponse{ErrorMessage: "account_id is required"}, "", fmt.Errorf("account_id is required")
	}
	jobID := uuid.NewString()
	params := codexOAuthAddPhoneJobParams(accountID, req.GetLabel(), req.GetMaxReuseCount())
	params["engine"] = "n8n"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionCodexOAuthAddPhone, params); err != nil {
		return &pb.CodexOAuthAddPhoneResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.CodexOAuthAddPhoneResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) N8NCodexOAuthAddPhoneDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	return s.n8nDynamicProxySettings(ctx, jobID, accountID, n8nExecutionID, actionCodexOAuthAddPhone, s.bindN8NCodexOAuthAddPhoneExecution)
}

func (s *Server) RecordN8NCodexOAuthAddPhoneDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error) {
	return s.recordN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, proxyURL, data, actionCodexOAuthAddPhone, s.bindN8NCodexOAuthAddPhoneExecution)
}

func (s *Server) FailN8NCodexOAuthAddPhoneDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	return s.failN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, errorMessage, data, s.bindN8NCodexOAuthAddPhoneExecution)
}

func (s *Server) UseN8NCodexOAuthAddPhoneProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthAddPhoneExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolUseProxyActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, codexOAuthProtocolAddPhoneMode))
	result := n8nProtocolStepResult(jobID, accountID, n8nExecutionID, stepProtocolUseProxy, out.GetData())
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProtocolUseProxy, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	result.Success = true
	return result, nil
}

func (s *Server) CodexOAuthAcquirePhone(ctx context.Context, req *pb.CodexOAuthAcquirePhoneInput) (*pb.CodexOAuthPhoneLease, error) {
	out, err := s.activities.CodexOAuthAcquirePhoneActivity(ctx, *req)
	return out, err
}

func (s *Server) CodexOAuthAddPhoneProtocol(ctx context.Context, req *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error) {
	out, err := s.activities.CodexOAuthAddPhoneProtocolActivity(ctx, *req)
	return &out, err
}

func (s *Server) CodexOAuthReleasePhone(ctx context.Context, req *pb.CodexOAuthReleasePhoneInput) (any, error) {
	if err := s.activities.CodexOAuthReleasePhoneActivity(ctx, *req); err != nil {
		return nil, err
	}
	return map[string]any{"job_id": strings.TrimSpace(req.GetJobId()), "activation_id": strings.TrimSpace(req.GetActivationId()), "success": true}, nil
}

func (s *Server) CompleteN8NCodexOAuthAddPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, result map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthAddPhoneExecution(ctx, jobID, n8nExecutionID); err != nil {
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
	return &n8nCodexOAuthResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuthAddPhone, Success: true, Result: result}, nil
}

func (s *Server) FailN8NCodexOAuthAddPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NCodexOAuthAddPhoneExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "codex oauth add phone failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	step = strings.TrimSpace(step)
	if step == "" {
		step = s.codexOAuthAddPhoneFailureStep(ctx, jobID)
	}
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, err, data); markErr != nil {
		return nil, markErr
	}
	return &n8nCodexOAuthResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionCodexOAuthAddPhone, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) bindN8NCodexOAuthAddPhoneExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) codexOAuthAddPhoneFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepCodexOAuthProtocolStart
}
