package dashboard

import (
	"errors"
	"net/http"
	"strings"

	"github.com/byte-v-forge/common-lib/httpx"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"orchestrator/internal/chatgptauth"
	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
)

func (s *server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := int32(httpx.QueryInt(r, "limit", 100))
		resp, err := s.accountClient.ListAccounts(r.Context(), &pb.ListAccountsRequest{
			Status: r.URL.Query().Get("status"),
			Limit:  limit,
			Email:  r.URL.Query().Get("email"),
			Cursor: r.URL.Query().Get("cursor"),
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req pb.CreateGPTAccountRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.GetAccountId()) == "" {
			req.AccountId = randomID()
		}
		resp, err := s.workflowAPI.CreateGPTAccount(r.Context(), &req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if resp.GetErrorMessage() != "" {
			writeError(w, http.StatusBadGateway, errors.New(resp.GetErrorMessage()))
			return
		}
		writeProtoJSON(w, http.StatusCreated, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *server) handleAccount(w http.ResponseWriter, r *http.Request) {
	accountPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/accounts/"), "/")
	parts := strings.Split(accountPath, "/")
	accountID := parts[0]
	if accountID == "" {
		writeError(w, http.StatusBadRequest, errors.New("account_id is required"))
		return
	}
	if len(parts) > 1 {
		if len(parts) == 2 && parts[1] == "access-token" {
			s.handleAccountAccessToken(w, r, accountID)
			return
		}
		if len(parts) == 2 && parts[1] == "auth" {
			s.handleAccountAuth(w, r, accountID)
			return
		}
		if len(parts) == 2 && parts[1] == "checkout-link" {
			s.handleAccountCheckoutLink(w, r, accountID)
			return
		}
		if parts[1] == "fingerprint" {
			action := ""
			if len(parts) == 3 {
				action = parts[2]
			}
			if len(parts) <= 3 {
				s.handleAccountFingerprint(w, r, accountID, action)
				return
			}
		}
		if len(parts) == 2 && parts[1] == "proxy-usages" {
			s.handleAccountProxyUsages(w, r, accountID)
			return
		}
		if len(parts) == 3 && parts[1] == "mailbox" && parts[2] == "inbox" {
			s.handleAccountMailboxInbox(w, r, accountID)
			return
		}
		writeError(w, http.StatusNotFound, errors.New("account endpoint not found"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		resp, err := s.accountClient.GetAccount(r.Context(), &pb.GetAccountRequest{AccountId: accountID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	case http.MethodPatch, http.MethodPut:
		var req pb.UpdateAccountAuthRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		sessionToken, accessToken := normalizeAccountAuthInput(req.GetSessionToken(), req.GetAccessToken())
		if sessionToken == "" && accessToken == "" && req.ActivationChannel == nil {
			writeError(w, http.StatusBadRequest, errors.New("session_token, access_token, or activation_channel is required"))
			return
		}
		if err := s.saveAccountAuth(r.Context(), accountID, sessionToken, accessToken); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		account := gptaccount.Patch(accountID)
		if sessionToken != "" || accessToken != "" {
			gptaccount.SetStatus(account, gptplugin.AccountStatusRegistered, "")
		}
		if req.ActivationChannel != nil {
			activationChannel := strings.TrimSpace(req.GetActivationChannel())
			account.ActivationChannel = &activationChannel
		}
		resp, err := s.accountClient.UpdateAccount(r.Context(), &pb.UpdateAccountRequest{Account: account})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	case http.MethodDelete:
		resp, err := s.accountClient.DeleteAccount(r.Context(), &pb.DeleteAccountRequest{AccountId: accountID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if s.runtimeSecrets != nil {
			_ = s.runtimeSecrets.Delete(r.Context(), chatgptauth.AccountAuthSecretKey(accountID))
		}
		writeProtoJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
