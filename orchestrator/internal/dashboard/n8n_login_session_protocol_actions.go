package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NLoginSessionProtocolActions interface {
	StartN8NLoginSessionProtocol(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error)
	N8NLoginSessionProtocolDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	RecordN8NLoginSessionProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error)
	FailN8NLoginSessionProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
	UseN8NLoginSessionProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	StartN8NLoginSessionProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	WaitN8NLoginSessionProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string) (any, error)
	AwaitN8NLoginSessionProtocolOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error)
	CompleteN8NLoginSessionProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, otpSource string, otpIssuedAfterUnix int64) (any, error)
	FinishN8NLoginSessionProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error)
	FailN8NLoginSessionProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
}

func (s *server) handleLoginProtocolAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nLoginSessionProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n login-session-protocol action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionLoginSessionProtocol)
	switch action {
	case "proxy-settings":
		s.proxySettingsLoginProtocol(w, r)
	case "record-proxy":
		s.recordLoginProtocolProxy(w, r)
	case "fail-proxy":
		s.failLoginProtocolProxy(w, r)
	case "use-proxy":
		s.useLoginProtocolProxy(w, r)
	case "start":
		s.startLoginProtocolAuth(w, r)
	case "wait":
		s.waitLoginProtocolAuth(w, r)
	case "await-otp":
		s.awaitLoginProtocolOTP(w, r)
	case "complete":
		s.completeLoginProtocolAuth(w, r)
	case "finish":
		s.finishLoginProtocol(w, r)
	case "fail":
		s.failLoginProtocol(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported login-session-protocol action: %s", action))
	}
}

func (s *server) proxySettingsLoginProtocol(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.N8NLoginSessionProtocolDynamicProxySettings(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) recordLoginProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthProxyRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.RecordN8NLoginSessionProtocolDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL, req.Data)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) failLoginProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.FailN8NLoginSessionProtocolDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) useLoginProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.UseN8NLoginSessionProtocolProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) startLoginProtocolAuth(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.StartN8NLoginSessionProtocolAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) waitLoginProtocolAuth(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.WaitN8NLoginSessionProtocolAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.Email)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) awaitLoginProtocolOTP(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthOTPWaitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.AwaitN8NLoginSessionProtocolOTP(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.Email, req.TimeoutSeconds, req.OtpIssuedAfterUnix, req.ResumeURL)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) completeLoginProtocolAuth(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.CompleteN8NLoginSessionProtocolAuth(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.OtpSource, req.OtpIssuedAfterUnix)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) finishLoginProtocol(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFinishRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.FinishN8NLoginSessionProtocol(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ResultRef)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) failLoginProtocol(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nLoginSessionProtocolActions.FailN8NLoginSessionProtocol(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	s.writeLoginProtocolAction(w, resp, err)
}

func (s *server) writeLoginProtocolAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
