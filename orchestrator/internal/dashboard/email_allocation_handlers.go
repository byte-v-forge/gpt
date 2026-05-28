package dashboard

import (
	"net/http"
	"strings"

	"github.com/byte-v-forge/common-lib/httpx"

	"orchestrator/pb"
)

func (s *server) handleGPTEmailAllocations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.accountClient.ListGPTEmailAllocations(r.Context(), &pb.ListGPTEmailAllocationsRequest{
		Status:       strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:        int32(httpx.QueryInt(r, "limit", 500)),
		PrimaryEmail: strings.TrimSpace(r.URL.Query().Get("primary_email")),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	allocations := resp.GetAllocations()
	if allocations == nil {
		allocations = []*pb.GPTEmailAllocation{}
	}
	writeJSON(w, http.StatusOK, allocations)
}
