package dashboard

import (
	"net/http"

	"orchestrator/internal/accountauth"
)

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
	writeProtoJSON(w, http.StatusOK, accountauth.Response(accountID, sessionToken, accessToken))
}
