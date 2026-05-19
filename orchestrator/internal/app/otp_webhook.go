package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"orchestrator/internal/gopayotp"
)

type goPayOTPWebhookPayload struct {
	OTP    string `json:"otp"`
	Source string `json:"source"`
}

type goPayOTPWebhookResponse struct {
	Success      bool   `json:"success"`
	Purpose      string `json:"purpose,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type goPayOTPWebhookHandler struct {
	relay *gopayotp.Relay
}

func startGoPayOTPWebhookServer(addr string, relay *gopayotp.Relay) (*http.Server, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen gopay otp webhook %s: %w", addr, err)
	}
	server := &http.Server{
		Handler:           goPayOTPWebhookHandler{relay: relay},
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("GoPay OTP webhook listening on %s", addr)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("GoPay OTP webhook stopped: %v", err)
		}
	}()
	return server, nil
}

func (h goPayOTPWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		writeGoPayOTPWebhookJSON(w, http.StatusOK, goPayOTPWebhookResponse{Success: true})
		return
	}
	if r.Method != http.MethodPost {
		writeGoPayOTPWebhookJSON(w, http.StatusMethodNotAllowed, goPayOTPWebhookResponse{Success: false, ErrorMessage: "method not allowed"})
		return
	}
	if h.relay == nil {
		writeGoPayOTPWebhookJSON(w, http.StatusServiceUnavailable, goPayOTPWebhookResponse{Success: false, ErrorMessage: "otp relay not configured"})
		return
	}

	queueSource, purposeSegment, err := parseGoPayOTPWebhookPath(r.URL.Path)
	if err != nil {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: err.Error()})
		return
	}
	purpose, err := gopayotp.QueueKey(queueSource, purposeSegment)
	if err != nil {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: err.Error()})
		return
	}

	var payload goPayOTPWebhookPayload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	if err := decoder.Decode(&payload); err != nil {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: "invalid json payload"})
		return
	}
	entry, err := h.relay.Put(purpose, payload.OTP, payload.Source)
	if err != nil {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: err.Error()})
		return
	}
	log.Printf("GoPay OTP webhook accepted purpose=%s source=%s received_at=%d", entry.Purpose, entry.Source, entry.ReceivedAt.Unix())
	writeGoPayOTPWebhookJSON(w, http.StatusAccepted, goPayOTPWebhookResponse{Success: true, Purpose: entry.Purpose})
}

func parseGoPayOTPWebhookPath(path string) (string, string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("webhook path must be /<source>/<purpose>")
	}
	source, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("invalid source path segment")
	}
	purpose, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid purpose path segment")
	}
	return source, purpose, nil
}

func writeGoPayOTPWebhookJSON(w http.ResponseWriter, status int, response goPayOTPWebhookResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
