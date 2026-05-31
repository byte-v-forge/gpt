package dashboard

import (
	"net/http"
	"strings"

	"github.com/byte-v-forge/common-lib/httpx"
	"github.com/byte-v-forge/common-lib/pagex"

	"orchestrator/pb"
)

func (s *server) handleGPTEmailAllocations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.accountClient.ListGPTEmailAllocations(r.Context(), &pb.ListGPTEmailAllocationsRequest{
		Status:       strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:        int32(httpx.QueryInt(r, "limit", pagex.DefaultLimit)),
		PrimaryEmail: strings.TrimSpace(r.URL.Query().Get("primary_email")),
		Cursor:       strings.TrimSpace(r.URL.Query().Get("cursor")),
		IsPrimary:    queryOptionalBool(r, "is_primary"),
		Email:        strings.TrimSpace(r.URL.Query().Get("email")),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}

func queryOptionalBool(r *http.Request, key string) *bool {
	if strings.TrimSpace(r.URL.Query().Get(key)) == "" {
		return nil
	}
	value := httpx.QueryBool(r, key, false)
	return &value
}
