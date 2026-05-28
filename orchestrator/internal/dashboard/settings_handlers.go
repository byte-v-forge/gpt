package dashboard

import (
	"errors"
	"net/http"

	"orchestrator/pb"
)

func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusBadGateway, errors.New("GPT settings store is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := s.settings.Get(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, &pb.GetGPTSettingsResponse{Settings: settings})
	case http.MethodPut, http.MethodPost:
		var req pb.UpdateGPTSettingsRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		settings, err := s.settings.Update(r.Context(), req.GetSettings())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, &pb.UpdateGPTSettingsResponse{Settings: settings})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut+", "+http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
