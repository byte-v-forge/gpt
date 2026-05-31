package dashboard

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/accountauth"
	"orchestrator/pb"
)

func (s *server) handleAccountAccessToken(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	accountResp, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	account := accountResp.GetAccount()
	if account == nil {
		writeError(w, http.StatusNotFound, errors.New("account not found"))
		return
	}
	sessionToken, _, err := accountauth.LoadChatGPTSessionToken(ctx, s.runtimeSecrets, accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if sessionToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("session_token is required"))
		return
	}

	credential, err := s.paymentCredential(ctx, accountID, sessionToken, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	resp, err := s.paymentClient.FetchAccessToken(ctx, &pb.FetchAccessTokenPaymentRequest{Credential: credential})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp == nil || !resp.GetSuccess() || strings.TrimSpace(resp.GetAccessToken()) == "" {
		msg := "access token fetch failed"
		if resp != nil && strings.TrimSpace(resp.GetErrorMessage()) != "" {
			msg = resp.GetErrorMessage()
		}
		writeError(w, http.StatusBadGateway, errors.New(msg))
		return
	}
	accessToken := resp.GetAccessToken()
	if err := accountauth.SaveChatGPTAccessToken(ctx, s.runtimeSecrets, accountID, accessToken); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeProtoJSON(w, http.StatusOK, &pb.GetAccountResponse{Account: account})
}

func (s *server) handleAccountCheckoutLink(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	accountResp, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	account := accountResp.GetAccount()
	if account == nil {
		writeError(w, http.StatusNotFound, errors.New("account not found"))
		return
	}

	sessionToken, accessToken, err := s.accountAuthTokens(ctx, accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if sessionToken == "" && accessToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("session_token or access_token is required"))
		return
	}

	credential, err := s.paymentCredential(ctx, accountID, sessionToken, accessToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	resp, err := s.paymentClient.CreateCheckoutLink(ctx, &pb.CreateCheckoutLinkRequest{Credential: credential})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !resp.GetSuccess() || resp.GetErrorMessage() != "" {
		msg := strings.TrimSpace(resp.GetErrorMessage())
		if msg == "" {
			msg = "checkout link creation failed"
		}
		writeError(w, http.StatusBadGateway, errors.New(msg))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}
