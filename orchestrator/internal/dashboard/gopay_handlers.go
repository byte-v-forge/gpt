package dashboard

import (
	"net/http"
	"strings"

	"orchestrator/pb"
)

func (s *server) handleGoPayState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		userID = "local"
	}
	resp, err := s.gopayAppClient.GoPayUserStatus(r.Context(), &pb.GoPayUserStatusRequest{UserId: userID})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	waPhone, err := s.gopayAppClient.GoPayUserGetWAPhone(r.Context(), &pb.GoPayUserGetWAPhoneRequest{UserId: userID})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":                resp.GetSuccess(),
		"error_message":          resp.GetErrorMessage(),
		"user_id":                userID,
		"wa_phone":               waPhone.GetWaPhone(),
		"wa_phone_error_message": waPhone.GetErrorMessage(),
		"status":                 resp.GetStatus(),
	})
}

func (s *server) handleGoPayProfile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		if userID == "" {
			userID = "local"
		}
		resp, err := s.gopayAppClient.GoPayUserGetWAPhone(r.Context(), &pb.GoPayUserGetWAPhoneRequest{UserId: userID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req pb.GoPayUserSetWAPhoneRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		resp, err := s.gopayAppClient.GoPayUserSetWAPhone(r.Context(), &req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
