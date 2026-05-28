package dashboard

import (
	"errors"
	"net/http"
	"strings"

	"orchestrator/internal/accountfingerprint"
)

type accountFingerprintRequest struct {
	AccountID   string `json:"account_id"`
	CountryCode string `json:"country_code"`
	Region      string `json:"region"`
}

type accountFingerprintResponse struct {
	AccountID              string `json:"account_id"`
	Generated              bool   `json:"generated,omitempty"`
	BrowserProfileTemplate string `json:"browser_profile_template"`
	BrowserFamily          string `json:"browser_family"`
	BrowserMajorVersion    string `json:"browser_major_version"`
	OSFamily               string `json:"os_family"`
	TLSProfileFamily       string `json:"tls_profile_family"`
	TLSFingerprintVariant  string `json:"tls_fingerprint_variant"`
	Locale                 string `json:"locale"`
	Timezone               string `json:"timezone"`
	UserAgent              string `json:"user_agent"`
	AcceptLanguage         string `json:"accept_language"`
	Language               string `json:"language"`
	DeviceID               string `json:"device_id"`
	CreatedAt              int64  `json:"created_at,omitempty"`
	UpdatedAt              int64  `json:"updated_at,omitempty"`
}

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
		writeJSON(w, http.StatusOK, fingerprintResponse(profile, false))
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
	var req accountFingerprintRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	accountID := strings.TrimSpace(req.AccountID)
	if accountID == "" {
		accountID = randomID()
	}
	profile := s.fingerprints.Preview(accountID, accountfingerprint.GenerateParams{CountryCode: req.CountryCode, Region: req.Region})
	writeJSON(w, http.StatusOK, fingerprintResponse(profile, false))
}

func (s *server) generateAccountFingerprint(w http.ResponseWriter, r *http.Request, accountID string) {
	var req accountFingerprintRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile, created, err := s.fingerprints.Generate(r.Context(), accountID, accountfingerprint.GenerateParams{
		CountryCode: req.CountryCode,
		Region:      req.Region,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, fingerprintResponse(profile, created))
}

func fingerprintResponse(profile accountfingerprint.Profile, generated bool) accountFingerprintResponse {
	return accountFingerprintResponse{
		AccountID:              profile.AccountID,
		Generated:              generated,
		BrowserProfileTemplate: profile.BrowserProfileTemplate,
		BrowserFamily:          profile.BrowserFamily,
		BrowserMajorVersion:    profile.BrowserMajorVersion,
		OSFamily:               profile.OSFamily,
		TLSProfileFamily:       profile.TLSProfileFamily,
		TLSFingerprintVariant:  profile.TLSFingerprintVariant,
		Locale:                 profile.Locale,
		Timezone:               profile.Timezone,
		UserAgent:              profile.UserAgent,
		AcceptLanguage:         profile.AcceptLanguage,
		Language:               profile.Language,
		DeviceID:               profile.DeviceID,
		CreatedAt:              profile.CreatedAt,
		UpdatedAt:              profile.UpdatedAt,
	}
}
