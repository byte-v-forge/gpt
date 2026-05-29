package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NLoginSessionActions interface {
	StartN8NLoginSession(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error)
	StartN8NLoginSessionBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	AwaitN8NLoginSessionOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error)
	CompleteN8NLoginSessionBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, otpSource string, otpIssuedAfterUnix int64) (any, error)
	FinishN8NLoginSession(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error)
	FailN8NLoginSession(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, errorMessage string, data map[string]any) (any, error)
}

func (s *server) handleLoginAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nLoginSessionActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n login-session action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionLoginSession)
	switch action {
	case "start":
		s.startLoginBrowser(w, r)
	case "await-otp":
		s.awaitLoginOTP(w, r)
	case "complete":
		s.completeLoginBrowser(w, r)
	case "finish":
		s.finishLogin(w, r)
	case "fail":
		s.failLogin(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported login-session action: %s", action))
	}
}

func (s *server) startLoginBrowser(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionActions.StartN8NLoginSessionBrowser(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeLoginAction(w, resp, err)
}

func (s *server) awaitLoginOTP(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthOTPWaitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionActions.AwaitN8NLoginSessionOTP(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.Email, req.TimeoutSeconds, req.OtpIssuedAfterUnix, req.ResumeURL)
	s.writeLoginAction(w, resp, err)
}

func (s *server) completeLoginBrowser(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionActions.CompleteN8NLoginSessionBrowser(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.OtpSource, req.OtpIssuedAfterUnix)
	s.writeLoginAction(w, resp, err)
}

func (s *server) finishLogin(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFinishRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionActions.FinishN8NLoginSession(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ResultRef)
	s.writeLoginAction(w, resp, err)
}

func (s *server) failLogin(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionActions.FailN8NLoginSession(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.ErrorMessage, req.Data)
	s.writeLoginAction(w, resp, err)
}

func (s *server) writeLoginAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
