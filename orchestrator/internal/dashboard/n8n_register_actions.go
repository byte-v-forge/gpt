package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NRegisterActions interface {
	StartN8NRegister(ctx context.Context, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error)
	StartN8NRegisterBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	AwaitN8NRegisterOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error)
	CompleteN8NRegisterBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, otpSource string, otpIssuedAfterUnix int64) (any, error)
	FinishN8NRegister(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error)
	FailN8NRegister(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, errorMessage string, data map[string]any) (any, error)
}

func (s *server) handleRegisterAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nRegisterActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionRegister)
	switch action {
	case "start":
		s.startRegisterBrowser(w, r)
	case "await-otp":
		s.awaitRegisterOTP(w, r)
	case "complete":
		s.completeRegisterBrowser(w, r)
	case "finish":
		s.finishRegister(w, r)
	case "fail":
		s.failRegister(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported register action: %s", action))
	}
}

func (s *server) startRegisterBrowser(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterActions.StartN8NRegisterBrowser(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeRegisterAction(w, resp, err)
}

func (s *server) awaitRegisterOTP(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthOTPWaitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterActions.AwaitN8NRegisterOTP(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.Email, req.TimeoutSeconds, req.OtpIssuedAfterUnix, req.ResumeURL)
	s.writeRegisterAction(w, resp, err)
}

func (s *server) completeRegisterBrowser(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterActions.CompleteN8NRegisterBrowser(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.OtpSource, req.OtpIssuedAfterUnix)
	s.writeRegisterAction(w, resp, err)
}

func (s *server) finishRegister(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFinishRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterActions.FinishN8NRegister(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ResultRef)
	s.writeRegisterAction(w, resp, err)
}

func (s *server) failRegister(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterActions.FailN8NRegister(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.ErrorMessage, req.Data)
	s.writeRegisterAction(w, resp, err)
}

func (s *server) writeRegisterAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
