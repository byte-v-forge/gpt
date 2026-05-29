package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NRegisterProtocolActions interface {
	StartN8NRegisterProtocol(ctx context.Context, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error)
	N8NDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	RecordN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error)
	FailN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
	UseN8NRegisterProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	StartN8NRegisterProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	WaitN8NRegisterProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string) (any, error)
	AwaitN8NRegisterProtocolOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error)
	CompleteN8NRegisterProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, otpSource string, otpIssuedAfterUnix int64) (any, error)
	FinishN8NRegisterProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error)
	FailN8NRegisterProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
}

type protocolAuthStepRequest struct {
	JobID          string `json:"job_id"`
	AccountID      string `json:"account_id"`
	N8NExecutionID string `json:"n8n_execution_id"`
	FlowID         string `json:"flow_id"`
	Email          string `json:"email"`
	ProxyURL       string `json:"proxy_url"`
}

type protocolAuthOTPWaitRequest struct {
	protocolAuthStepRequest
	TimeoutSeconds     int32  `json:"timeout_seconds"`
	OtpIssuedAfterUnix int64  `json:"otp_issued_after_unix"`
	ResumeURL          string `json:"resume_url"`
}

type protocolAuthCompleteRequest struct {
	protocolAuthStepRequest
	OtpSource          string `json:"otp_source"`
	OtpIssuedAfterUnix int64  `json:"otp_issued_after_unix"`
}

type protocolAuthFinishRequest struct {
	protocolAuthStepRequest
	ResultRef string `json:"result_ref"`
}

type protocolAuthFailRequest struct {
	protocolAuthStepRequest
	ErrorMessage string         `json:"error_message"`
	Data         map[string]any `json:"data"`
}

type protocolAuthProxyRecordRequest struct {
	protocolAuthStepRequest
	Data map[string]any `json:"data"`
}

func (s *server) handleRegisterProtocolAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nRegisterProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register-protocol action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionRegisterProtocol)
	switch action {
	case "proxy-settings":
		s.proxySettingsRegisterProtocol(w, r)
	case "record-proxy":
		s.recordRegisterProtocolProxy(w, r)
	case "fail-proxy":
		s.failRegisterProtocolProxy(w, r)
	case "use-proxy":
		s.useRegisterProtocolProxy(w, r)
	case "start":
		s.startRegisterProtocolAuth(w, r)
	case "wait":
		s.waitRegisterProtocolAuth(w, r)
	case "await-otp":
		s.awaitRegisterProtocolOTP(w, r)
	case "complete":
		s.completeRegisterProtocolAuth(w, r)
	case "finish":
		s.finishRegisterProtocol(w, r)
	case "fail":
		s.failRegisterProtocol(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported register-protocol action: %s", action))
	}
}

func (s *server) proxySettingsRegisterProtocol(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.N8NDynamicProxySettings(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) recordRegisterProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthProxyRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.RecordN8NDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL, req.Data)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) failRegisterProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.FailN8NDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) useRegisterProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.UseN8NRegisterProtocolProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) startRegisterProtocolAuth(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.StartN8NRegisterProtocolAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) waitRegisterProtocolAuth(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.WaitN8NRegisterProtocolAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.Email)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) awaitRegisterProtocolOTP(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthOTPWaitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.AwaitN8NRegisterProtocolOTP(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.Email, req.TimeoutSeconds, req.OtpIssuedAfterUnix, req.ResumeURL)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) completeRegisterProtocolAuth(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.CompleteN8NRegisterProtocolAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.OtpSource, req.OtpIssuedAfterUnix)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) finishRegisterProtocol(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFinishRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.FinishN8NRegisterProtocol(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ResultRef)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) failRegisterProtocol(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nRegisterProtocolActions.FailN8NRegisterProtocol(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	s.writeRegisterProtocolAction(w, resp, err)
}

func (s *server) writeRegisterProtocolAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
