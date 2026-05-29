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

type N8NCodexOAuthAddPhoneActions interface {
	StartN8NCodexOAuthAddPhone(ctx context.Context, req *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, string, error)
	N8NCodexOAuthAddPhoneDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	RecordN8NCodexOAuthAddPhoneDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error)
	FailN8NCodexOAuthAddPhoneDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
	UseN8NCodexOAuthAddPhoneProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	CodexOAuthStartProtocol(ctx context.Context, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error)
	CodexOAuthDetectProtocolStage(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitProtocolEmail(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitProtocolPassword(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitProtocolEmailOTP(ctx context.Context, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthAcquirePhone(ctx context.Context, req *pb.CodexOAuthAcquirePhoneInput) (*pb.CodexOAuthPhoneLease, error)
	CodexOAuthAddPhoneProtocol(ctx context.Context, req *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error)
	CodexOAuthCompleteProtocol(ctx context.Context, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error)
	CodexOAuthStopProtocol(ctx context.Context, req *pb.CodexOAuthStopBrowserInput) (any, error)
	CodexOAuthReleasePhone(ctx context.Context, req *pb.CodexOAuthReleasePhoneInput) (any, error)
	CompleteN8NCodexOAuthAddPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, result map[string]any) (any, error)
	FailN8NCodexOAuthAddPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, errorMessage string, data map[string]any) (any, error)
}

func (s *server) handleCodexOAuthAddPhoneAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nCodexOAuthAddPhoneActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex oauth add-phone action API is not configured"))
		return
	}
	action := s.actionSubPath(r, contracts.ActionCodexOAuthAddPhone)
	switch action {
	case "proxy-settings":
		s.codexOAuthAddPhoneProxySettings(w, r)
	case "record-proxy":
		s.recordCodexOAuthAddPhoneProxy(w, r)
	case "fail-proxy":
		s.failCodexOAuthAddPhoneProxy(w, r)
	case "use-proxy":
		s.useCodexOAuthAddPhoneProtocolProxy(w, r)
	case "start-protocol":
		req := &pb.CodexOAuthStartBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthStartProtocol(r.Context(), req)
		})
	case "detect":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthDetectProtocolStage(r.Context(), req)
		})
	case "submit-email":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthSubmitProtocolEmail(r.Context(), req)
		})
	case "submit-password":
		req := &pb.CodexOAuthBrowserStepInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthSubmitProtocolPassword(r.Context(), req)
		})
	case "submit-email-otp":
		req := &pb.CodexOAuthSubmitEmailOTPInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthSubmitProtocolEmailOTP(r.Context(), req)
		})
	case "acquire-phone":
		req := &pb.CodexOAuthAcquirePhoneInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthAcquirePhone(r.Context(), req)
		})
	case "add-phone-protocol":
		req := &pb.CodexOAuthAddPhoneBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthAddPhoneProtocol(r.Context(), req)
		})
	case "complete-protocol":
		req := &pb.CodexOAuthCompleteBrowserInput{}
		serveProtoAction(w, r, req, func() (proto.Message, error) {
			return s.n8nCodexOAuthAddPhoneActions.CodexOAuthCompleteProtocol(r.Context(), req)
		})
	case "stop-protocol":
		s.stopCodexOAuthAddPhoneProtocol(w, r)
	case "release-phone":
		s.releaseCodexOAuthPhone(w, r)
	case "complete":
		s.completeCodexOAuthAddPhone(w, r)
	case "fail":
		s.failCodexOAuthAddPhone(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported codex oauth add-phone action: %s", action))
	}
}

func (s *server) codexOAuthAddPhoneProxySettings(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.N8NCodexOAuthAddPhoneDynamicProxySettings(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) recordCodexOAuthAddPhoneProxy(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthProtocolProxyRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.RecordN8NCodexOAuthAddPhoneDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL, req.Data)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) failCodexOAuthAddPhoneProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.FailN8NCodexOAuthAddPhoneDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) useCodexOAuthAddPhoneProtocolProxy(w http.ResponseWriter, r *http.Request) {
	var req protocolAuthStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.UseN8NCodexOAuthAddPhoneProtocolProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) releaseCodexOAuthPhone(w http.ResponseWriter, r *http.Request) {
	var req pb.CodexOAuthReleasePhoneInput
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.CodexOAuthReleasePhone(r.Context(), &req)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) stopCodexOAuthAddPhoneProtocol(w http.ResponseWriter, r *http.Request) {
	var req pb.CodexOAuthStopBrowserInput
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.CodexOAuthStopProtocol(r.Context(), &req)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) completeCodexOAuthAddPhone(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.CompleteN8NCodexOAuthAddPhone(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.Result)
	writeCodexOAuthAction(w, resp, err)
}

func (s *server) failCodexOAuthAddPhone(w http.ResponseWriter, r *http.Request) {
	var req codexOAuthFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nCodexOAuthAddPhoneActions.FailN8NCodexOAuthAddPhone(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.Step, req.ErrorMessage, req.Data)
	writeCodexOAuthAction(w, resp, err)
}
