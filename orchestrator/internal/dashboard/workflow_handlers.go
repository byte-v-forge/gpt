package dashboard

import (
	"errors"
	"net/http"

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
	resp, err := s.accountWorkflowClient.RegisterAccount(r.Context(), &req)
	if err != nil {
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
	if s.n8nRegisterProtocol == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register-protocol workflow is not configured"))
		return
	}
	if s.n8nRegisterProtocolActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n register-protocol action runner is not configured"))
		return
	}
	resp, accountID, err := s.n8nRegisterProtocolActions.StartN8NRegisterProtocol(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.n8nRegisterProtocol.triggerRegisterProtocol(r.Context(), resp.GetJobId(), accountID); err != nil {
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

func (s *server) handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.ActivateAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.paymentWorkflowClient.ActivateAccount(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleAutopay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.ActivateAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.paymentWorkflowClient.AutopayAccount(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
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
	resp, err := s.accountWorkflowClient.LoginAccount(r.Context(), &req)
	if err != nil {
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
	resp, err := s.accountWorkflowClient.LoginAccountProtocol(r.Context(), &req)
	if err != nil {
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
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.accountWorkflowClient.CodexOAuth(r.Context(), &req)
	if err != nil {
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
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.accountWorkflowClient.CodexOAuthProtocol(r.Context(), &req)
	if err != nil {
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
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.accountWorkflowClient.CodexOAuthAddPhone(r.Context(), &req)
	if err != nil {
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
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.accountWorkflowClient.CodexOAuthBatchAddPhone(r.Context(), &req)
	if err != nil {
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
	if s.n8nProbe == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n probe-account workflow is not configured"))
		return
	}
	if s.n8nProbeActions == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n probe action runner is not configured"))
		return
	}
	resp, err := s.n8nProbeActions.StartN8NProbeAccount(r.Context(), req.GetAccountId())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.n8nProbe.triggerProbeAccount(r.Context(), resp.GetJobId(), req.GetAccountId()); err != nil {
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

func (s *server) handleRegisterAndActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.RegisterAndActivateAccountRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.accountWorkflowClient.RegisterAndActivateAccount(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
