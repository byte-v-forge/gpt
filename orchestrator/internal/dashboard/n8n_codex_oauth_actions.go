package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NCodexOAuthActions interface {
	StartN8NCodexOAuth(ctx context.Context, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error)
	BindN8NCodexOAuthExecution(ctx context.Context, jobID string, n8nExecutionID string) (any, error)
	CodexOAuthStartBrowser(ctx context.Context, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error)
	CodexOAuthDetectBrowserStage(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitEmail(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitPassword(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitEmailOTP(ctx context.Context, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthAddPhoneBrowser(ctx context.Context, req *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error)
	CodexOAuthCompleteBrowser(ctx context.Context, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error)
	CodexOAuthStopBrowser(ctx context.Context, req *pb.CodexOAuthStopBrowserInput) (any, error)
	CompleteN8NCodexOAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, result map[string]any) (any, error)
	FailN8NCodexOAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data map[string]any) (any, error)
}

type codexOAuthBindRequest struct {
	JobID          string `json:"job_id"`
	N8NExecutionID string `json:"n8n_execution_id"`
}

type codexOAuthCompleteRequest struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	Result         map[string]any `json:"result"`
}

type codexOAuthFailRequest struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	Step           string         `json:"step"`
	ErrorMessage   string         `json:"error_message"`
	Data           map[string]any `json:"data"`
}

func (s *server) handleCodexOAuthAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nCodexOAuthActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex oauth action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionCodexOAuth)
	switch action {
	case "bind":
		s.bindCodexOAuthExecution(w, r)
	case "start-browser":
		req := &pb.CodexOAuthStartBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthStartBrowser(r.Context(), req)
		})
	case "detect-browser-stage":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthDetectBrowserStage(r.Context(), req)
		})
	case "submit-email":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthSubmitEmail(r.Context(), req)
		})
	case "submit-password":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthSubmitPassword(r.Context(), req)
		})
	case "submit-email-otp":
		req := &pb.CodexOAuthSubmitEmailOTPInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthSubmitEmailOTP(r.Context(), req)
		})
	case "add-phone-browser":
		req := &pb.CodexOAuthAddPhoneBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthAddPhoneBrowser(r.Context(), req)
		})
	case "complete-browser":
		req := &pb.CodexOAuthCompleteBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthActions.CodexOAuthCompleteBrowser(r.Context(), req)
		})
	case "stop-browser":
		s.stopCodexOAuthBrowser(w, r)
	case "complete":
		s.completeCodexOAuth(w, r)
	case "fail":
		s.failCodexOAuth(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported codex oauth action: %s", action))
	}
}

func (s *server) bindCodexOAuthExecution(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthBindRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthActions.BindN8NCodexOAuthExecution(r.Context(), req.JobID, req.N8NExecutionID)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) stopCodexOAuthBrowser(w http.ResponseWriter, r *http.Request) {
	var req pb.CodexOAuthStopBrowserInput
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthActions.CodexOAuthStopBrowser(r.Context(), &req)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) completeCodexOAuth(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthActions.CompleteN8NCodexOAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.Result)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) failCodexOAuth(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthActions.FailN8NCodexOAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.Step, req.ErrorMessage, req.Data)
	writeCodexOAuthAction(w, resp, err)
}

func writeCodexOAuthAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
