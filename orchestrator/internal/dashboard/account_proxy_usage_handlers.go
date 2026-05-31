package dashboard

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"orchestrator/internal/accountproxyusage"
)

func (s *server) handleAccountProxyUsages(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.accountProxyUsages == nil {
		writeError(w, http.StatusBadGateway, errors.New("account proxy usage store is not configured"))
		return
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		writeError(w, http.StatusBadRequest, errors.New("account_id is required"))
		return
	}
	rows, err := s.accountProxyUsages.ListByAccount(r.Context(), accountID, queryLimit(r, 50, 200))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeProtoJSON(w, http.StatusOK, accountproxyusage.ListResponse(rows))
}

func queryLimit(r *http.Request, fallback int, maxValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
