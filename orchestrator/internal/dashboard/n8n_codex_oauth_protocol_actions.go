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

type N8NCodexOAuthProtocolActions interface {
	StartN8NCodexOAuthProtocol(ctx context.Context, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error)
	N8NCodexOAuthProtocolDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	RecordN8NCodexOAuthProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error)
	FailN8NCodexOAuthProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
	UseN8NCodexOAuthProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	CodexOAuthStartProtocol(ctx context.Context, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error)
	CodexOAuthDetectProtocolStage(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitProtocolEmail(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitProtocolPassword(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitProtocolEmailOTP(ctx context.Context, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthCompleteProtocol(ctx context.Context, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error)
	CodexOAuthStopProtocol(ctx context.Context, req *pb.CodexOAuthStopBrowserInput) (any, error)
	CompleteN8NCodexOAuthProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, result map[string]any) (any, error)
	FailN8NCodexOAuthProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data map[string]any) (any, error)
}

type codexOAuthProtocolProxyRecordRequest struct {
	protocolAuthStepRequest
	Data map[string]any `json:"data"`
}

func (s *server) handleCodexOAuthProtocolAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nCodexOAuthProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex oauth protocol action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionCodexOAuthProtocol)
	switch action {
	case "proxy-settings":
		s.codexOAuthProtocolProxySettings(w, r)
	case "record-proxy":
		s.recordCodexOAuthProtocolProxy(w, r)
	case "fail-proxy":
		s.failCodexOAuthProtocolProxy(w, r)
	case "use-proxy":
		s.useCodexOAuthProtocolProxy(w, r)
	case "start":
		req := &pb.CodexOAuthStartBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthProtocolActions.CodexOAuthStartProtocol(r.Context(), req)
		})
	case "detect":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthProtocolActions.CodexOAuthDetectProtocolStage(r.Context(), req)
		})
	case "submit-email":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthProtocolActions.CodexOAuthSubmitProtocolEmail(r.Context(), req)
		})
	case "submit-password":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthProtocolActions.CodexOAuthSubmitProtocolPassword(r.Context(), req)
		})
	case "submit-email-otp":
		req := &pb.CodexOAuthSubmitEmailOTPInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthProtocolActions.CodexOAuthSubmitProtocolEmailOTP(r.Context(), req)
		})
	case "complete-protocol":
		req := &pb.CodexOAuthCompleteBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthProtocolActions.CodexOAuthCompleteProtocol(r.Context(), req)
		})
	case "stop-protocol":
		s.stopCodexOAuthProtocol(w, r)
	case "complete":
		s.completeCodexOAuthProtocol(w, r)
	case "fail":
		s.failCodexOAuthProtocol(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported codex oauth protocol action: %s", action))
	}
}

func (s *server) codexOAuthProtocolProxySettings(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.N8NCodexOAuthProtocolDynamicProxySettings(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) recordCodexOAuthProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthProtocolProxyRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.RecordN8NCodexOAuthProtocolDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL, req.Data)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) failCodexOAuthProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.FailN8NCodexOAuthProtocolDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) useCodexOAuthProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.UseN8NCodexOAuthProtocolProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) stopCodexOAuthProtocol(w http.ResponseWriter, r *http.Request) {
	var req pb.CodexOAuthStopBrowserInput
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.CodexOAuthStopProtocol(r.Context(), &req)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) completeCodexOAuthProtocol(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.CompleteN8NCodexOAuthProtocol(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.Result)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) failCodexOAuthProtocol(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthProtocolActions.FailN8NCodexOAuthProtocol(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.Step, req.ErrorMessage, req.Data)
	writeCodexOAuthAction(w, resp, err)
}
