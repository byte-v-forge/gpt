package dashboard

import (
	"net/http"

	"orchestrator/internal/actionregistry"
)

func (s *server) handleActionCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeProtoJSON(w, http.StatusOK, actionregistry.CatalogResponse(s.actionRegistry.Actions()))
}
