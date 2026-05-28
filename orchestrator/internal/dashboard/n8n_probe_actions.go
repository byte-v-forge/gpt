package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"orchestrator/pb"
)

type N8NProbeActions interface {
	StartN8NProbeAccount(ctx context.Context, accountID string) (*pb.ProbeAccountResponse, error)
	N8NProbeDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error)
	RecordN8NProbeDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error)
	FailN8NProbeDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
	CheckN8NProbeToken(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string) (any, error)
	RunN8NProbePlusTrial(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string) (any, error)
	RunN8NProbeTier(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string) (any, error)
	CompleteN8NProbeAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string, plusTrial map[string]any, tier map[string]any) (any, error)
	FailN8NProbeAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error)
}

type probeStepRequest struct {
	JobID          string `json:"job_id"`
	AccountID      string `json:"account_id"`
	N8NExecutionID string `json:"n8n_execution_id"`
	ProxyURL       string `json:"proxy_url"`
}

type probeCompleteRequest struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	PlusTrial      map[string]any `json:"plus_trial"`
	Tier           map[string]any `json:"tier"`
}

type probeFailRequest struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	ErrorMessage   string         `json:"error_message"`
	Data           map[string]any `json:"data"`
}

type probeProxyRecordRequest struct {
	probeStepRequest
	Data map[string]any `json:"data"`
}

func (s *server) handleProbeAccountAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nProbeActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n probe action runner is not configured"))
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/actions/probe-account/"), "/")
	switch action {
	case "proxy-settings":
		s.probeProxySettings(w, r)
	case "record-proxy":
		s.recordProbeProxy(w, r)
	case "fail-proxy":
		s.failProbeProxy(w, r)
	case "check-token":
		s.checkProbeToken(w, r)
	case "plus-trial":
		s.runProbePlusTrial(w, r)
	case "tier":
		s.runProbeTier(w, r)
	case "complete":
		s.completeProbeAccount(w, r)
	case "fail":
		s.failProbeAccount(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported probe action: %s", action))
	}
}

func (s *server) probeProxySettings(w http.ResponseWriter, r *http.Request) {
	var req probeStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.N8NProbeDynamicProxySettings(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID)
	writeProbeAction(w, resp, err)
}

func (s *server) recordProbeProxy(w http.ResponseWriter, r *http.Request) {
	var req probeProxyRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.RecordN8NProbeDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL, req.Data)
	writeProbeAction(w, resp, err)
}

func (s *server) failProbeProxy(w http.ResponseWriter, r *http.Request) {
	var req probeFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.FailN8NProbeDynamicProxy(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	writeProbeAction(w, resp, err)
}

func (s *server) runProbePlusTrial(w http.ResponseWriter, r *http.Request) {
	var req probeStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.RunN8NProbePlusTrial(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) checkProbeToken(w http.ResponseWriter, r *http.Request) {
	var req probeStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.CheckN8NProbeToken(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) runProbeTier(w http.ResponseWriter, r *http.Request) {
	var req probeStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.RunN8NProbeTier(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeProbeAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) completeProbeAccount(w http.ResponseWriter, r *http.Request) {
	var req probeCompleteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.CompleteN8NProbeAccount(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.PlusTrial, req.Tier)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) failProbeAccount(w http.ResponseWriter, r *http.Request) {
	var req probeFailRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.n8nProbeActions.FailN8NProbeAccount(r.Context(), req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
