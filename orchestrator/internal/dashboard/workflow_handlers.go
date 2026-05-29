package dashboard

import (
	"errors"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.RegisterAccountRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionRegister)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register workflow is not configured"))
		return
	}
	if s.n8nRegisterActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nRegisterActions.StartN8NRegister(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]string{
		"job_id":     resp.GetJobId(),
		"account_id": accountID,
	}); err != nil {
		_, _ = s.n8nRegisterActions.FailN8NRegister(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleRegisterProtocol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.RegisterAccountRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionRegisterProtocol)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register-protocol workflow is not configured"))
		return
	}
	if s.n8nRegisterProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register-protocol action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nRegisterProtocolActions.StartN8NRegisterProtocol(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.triggerRegisterProtocol(r.Context(), resp.GetJobId(), accountID); err != nil {
		_, _ = s.n8nRegisterProtocolActions.FailN8NRegisterProtocol(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.LoginAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionLoginSession)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n login-session workflow is not configured"))
		return
	}
	if s.n8nLoginSessionActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n login-session action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nLoginSessionActions.StartN8NLoginSession(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]string{
		"job_id":     resp.GetJobId(),
		"account_id": accountID,
	}); err != nil {
		_, _ = s.n8nLoginSessionActions.FailN8NLoginSession(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleLoginProtocol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.LoginAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionLoginSessionProtocol)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n login-session-protocol workflow is not configured"))
		return
	}
	if s.n8nLoginSessionProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n login-session-protocol action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nLoginSessionProtocolActions.StartN8NLoginSessionProtocol(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]string{
		"job_id":     resp.GetJobId(),
		"account_id": accountID,
	}); err != nil {
		_, _ = s.n8nLoginSessionProtocolActions.FailN8NLoginSessionProtocol(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleCodexOAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.CodexOAuthRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionCodexOAuth)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth workflow is not configured"))
		return
	}
	if s.n8nCodexOAuthActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nCodexOAuthActions.StartN8NCodexOAuth(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]string{
		"job_id":     resp.GetJobId(),
		"account_id": accountID,
		"label":      req.GetLabel(),
	}); err != nil {
		_, _ = s.n8nCodexOAuthActions.FailN8NCodexOAuth(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleCodexOAuthProtocol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.CodexOAuthRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionCodexOAuthProtocol)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth-protocol workflow is not configured"))
		return
	}
	if s.n8nCodexOAuthProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth-protocol action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nCodexOAuthProtocolActions.StartN8NCodexOAuthProtocol(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]string{
		"job_id":     resp.GetJobId(),
		"account_id": accountID,
		"label":      req.GetLabel(),
	}); err != nil {
		_, _ = s.n8nCodexOAuthProtocolActions.FailN8NCodexOAuthProtocol(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleCodexOAuthAddPhone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.CodexOAuthAddPhoneRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionCodexOAuthAddPhone)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth-add-phone workflow is not configured"))
		return
	}
	if s.n8nCodexOAuthAddPhoneActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth-add-phone action API is not configured"))
		return
	}
	resp, accountID, err := s.n8nCodexOAuthAddPhoneActions.StartN8NCodexOAuthAddPhone(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]any{
		"job_id":          resp.GetJobId(),
		"account_id":      accountID,
		"label":           req.GetLabel(),
		"max_reuse_count": req.GetMaxReuseCount(),
	}); err != nil {
		_, _ = s.n8nCodexOAuthAddPhoneActions.FailN8NCodexOAuthAddPhone(
			r.Context(),
			resp.GetJobId(),
			accountID,
			"",
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleCodexOAuthBatchAddPhone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.CodexOAuthBatchAddPhoneRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionCodexOAuthBatchAddPhone)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth-batch-add-phone workflow is not configured"))
		return
	}
	if s.n8nCodexOAuthBatchActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n codex-oauth-batch-add-phone action API is not configured"))
		return
	}
	resp, err := s.n8nCodexOAuthBatchActions.StartN8NCodexOAuthBatchAddPhone(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), map[string]any{
		"job_id":          resp.GetJobId(),
		"account_ids":     req.GetAccountIds(),
		"label":           req.GetLabel(),
		"max_reuse_count": req.GetMaxReuseCount(),
	}); err != nil {
		_, _ = s.n8nCodexOAuthBatchActions.FailN8NCodexOAuthBatchAddPhone(
			r.Context(),
			resp.GetJobId(),
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func (s *server) handleProbeAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.ProbeAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(contracts.ActionProbeAccount)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n probe-account workflow is not configured"))
		return
	}
	if s.n8nProbeActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n probe action API is not configured"))
		return
	}
	resp, err := s.n8nProbeActions.StartN8NProbeAccount(r.Context(), req.GetAccountId())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.triggerProbeAccount(r.Context(), resp.GetJobId(), req.GetAccountId()); err != nil {
		_, _ = s.n8nProbeActions.FailN8NProbeAccount(
			r.Context(),
			resp.GetJobId(),
			req.GetAccountId(),
			"",
			err.Error(),
			map[string]any{"reason": "n8n_trigger_failed"},
		)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}
