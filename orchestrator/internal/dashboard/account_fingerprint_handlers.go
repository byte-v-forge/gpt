package dashboard

import (
	"errors"
	"net/http"
	"strings"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/pb"
)

func (s *server) handleAccountFingerprint(w http.ResponseWriter, r *http.Request, accountID string, action string) {
	if s.fingerprints == nil {
		writeError(w, http.StatusBadGateway, errors.New("account fingerprint store is not configured"))
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		profile, ok, err := s.fingerprints.Get(r.Context(), accountID)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, errors.New("account fingerprint not generated"))
			return
		}
		writeProtoJSON(w, http.StatusOK, accountfingerprint.Response(profile, false))
	case action == "generate" && r.Method == http.MethodPost:
		s.generateAccountFingerprint(w, r, accountID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *server) handleFingerprintPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.fingerprints == nil {
		writeError(w, http.StatusBadGateway, errors.New("account fingerprint store is not configured"))
		return
	}
	var req pb.AccountFingerprintRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = randomID()
	}
	profile := s.fingerprints.Preview(accountID, accountfingerprint.GenerateParams{CountryCode: req.GetCountryCode(), Region: req.GetRegion()})
	writeProtoJSON(w, http.StatusOK, accountfingerprint.Response(profile, false))
}

func (s *server) generateAccountFingerprint(w http.ResponseWriter, r *http.Request, accountID string) {
	var req pb.AccountFingerprintRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile, created, err := s.fingerprints.Generate(r.Context(), accountID, accountfingerprint.GenerateParams{
		CountryCode: req.GetCountryCode(),
		Region:      req.GetRegion(),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeProtoJSON(w, http.StatusCreated, accountfingerprint.Response(profile, created))
}
