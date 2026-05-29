package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NCodexOAuthBatchActions interface {
	StartN8NCodexOAuthBatchAddPhone(ctx context.Context, req *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, error)
	CreateN8NCodexOAuthBatchAddPhoneChild(ctx context.Context, parentJobID string, accountID string, label string, maxReuseCount int32, n8nExecutionID string) (any, error)
	CompleteN8NCodexOAuthBatchAddPhone(ctx context.Context, jobID string, n8nExecutionID string, result map[string]any) (any, error)
	FailN8NCodexOAuthBatchAddPhone(ctx context.Context, jobID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
}

type codexOAuthBatchChildRequest struct {
	ParentJobID    string `json:"parent_job_id"`
	AccountID      string `json:"account_id"`
	Label          string `json:"label"`
	MaxReuseCount  int32  `json:"max_reuse_count"`
	N8NExecutionID string `json:"n8n_execution_id"`
}

type codexOAuthBatchCompleteRequest struct {
	JobID          string         `json:"job_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	Result         map[string]any `json:"result"`
}

type codexOAuthBatchFailRequest struct {
	JobID          string         `json:"job_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	ErrorMessage   string         `json:"error_message"`
	Data           map[string]any `json:"data"`
}

func (s *server) handleCodexOAuthBatchAddPhoneAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nCodexOAuthBatchActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex oauth batch action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionCodexOAuthBatchAddPhone)
	switch action {
	case "create-child":
		s.createCodexOAuthBatchAddPhoneChild(w, r)
	case "complete":
		s.completeCodexOAuthBatchAddPhone(w, r)
	case "fail":
		s.failCodexOAuthBatchAddPhone(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported codex oauth batch action: %s", action))
	}
}

func (s *server) createCodexOAuthBatchAddPhoneChild(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthBatchChildRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthBatchActions.CreateN8NCodexOAuthBatchAddPhoneChild(r.Context(), req.ParentJobID, req.AccountID, req.Label, req.MaxReuseCount, req.N8NExecutionID)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) completeCodexOAuthBatchAddPhone(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthBatchCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthBatchActions.CompleteN8NCodexOAuthBatchAddPhone(r.Context(), req.JobID, req.N8NExecutionID, req.Result)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) failCodexOAuthBatchAddPhone(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthBatchFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthBatchActions.FailN8NCodexOAuthBatchAddPhone(r.Context(), req.JobID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	writeCodexOAuthAction(w, resp, err)
}
