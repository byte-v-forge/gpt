package dashboard

import (
	"net/http"

	"orchestrator/internal/accountauth"
)

type accountAuthResponse struct {
	AccountID                 string `json:"account_id"`
	SessionToken              string `json:"session_token"`
	SessionTokenExpiresAtUnix int64  `json:"session_token_expires_at_unix"`
	AccessToken               string `json:"access_token"`
	AccessTokenExpiresAtUnix  int64  `json:"access_token_expires_at_unix"`
	SessionTokenPresent       bool   `json:"session_token_present"`
	AccessTokenPresent        bool   `json:"access_token_present"`
}

func (s *server) handleAccountAuth(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sessionToken, err := accountauth.LoadChatGPTSessionTokenSnapshot(r.Context(), s.runtimeSecrets, accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	accessToken, err := accountauth.LoadChatGPTAccessTokenSnapshot(r.Context(), s.runtimeSecrets, accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, accountAuthResponse{
		AccountID:                 accountID,
		SessionToken:              sessionToken.Value,
		SessionTokenExpiresAtUnix: sessionToken.ExpiresAtUnix,
		AccessToken:               accessToken.Value,
		AccessTokenExpiresAtUnix:  accessToken.ExpiresAtUnix,
		SessionTokenPresent:       sessionToken.Present,
		AccessTokenPresent:        accessToken.Present,
	})
}
