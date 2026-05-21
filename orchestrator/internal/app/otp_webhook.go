package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"orchestrator/internal/accountmail"
	"orchestrator/internal/gopayotp"
	"orchestrator/pb"
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
	relay               *gopayotp.Relay
	accountClient       pb.AccountDatabaseServiceClient
	mailboxWebhookToken string
}

func startGoPayOTPWebhookServer(addr string, relay *gopayotp.Relay, accountClient pb.AccountDatabaseServiceClient, mailboxWebhookToken string) (*http.Server, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen gopay otp webhook %s: %w", addr, err)
	}
	server := &http.Server{
		Handler:           goPayOTPWebhookHandler{relay: relay, accountClient: accountClient, mailboxWebhookToken: strings.TrimSpace(mailboxWebhookToken)},
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
	if r.URL.Path == "/webhooks/mailbox/email" {
		h.handleMailboxEmail(w, r)
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

func (h goPayOTPWebhookHandler) handleMailboxEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGoPayOTPWebhookJSON(w, http.StatusMethodNotAllowed, goPayOTPWebhookResponse{Success: false, ErrorMessage: "method not allowed"})
		return
	}
	if h.accountClient == nil {
		writeGoPayOTPWebhookJSON(w, http.StatusServiceUnavailable, goPayOTPWebhookResponse{Success: false, ErrorMessage: "account service not configured"})
		return
	}
	if !h.validMailboxWebhookToken(r) {
		writeGoPayOTPWebhookJSON(w, http.StatusUnauthorized, goPayOTPWebhookResponse{Success: false, ErrorMessage: "unauthorized"})
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
	if err != nil {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: "invalid json payload"})
		return
	}
	var event pb.OutboundEmailWebhookEvent
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &event); err != nil {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: "invalid mailbox email event"})
		return
	}
	message := mailboxWebhookMessage(event.GetMessage())
	recipients := mailboxWebhookRecipients(message)
	if len(recipients) == 0 {
		writeGoPayOTPWebhookJSON(w, http.StatusBadRequest, goPayOTPWebhookResponse{Success: false, ErrorMessage: "recipient is required"})
		return
	}
	updated := 0
	for _, recipient := range recipients {
		resp, err := h.accountClient.ListAccounts(r.Context(), &pb.ListAccountsRequest{Email: recipient, Limit: 20})
		if err != nil {
			log.Printf("mailbox webhook account lookup failed recipient=%s: %v", redactMailboxWebhookEmail(recipient), err)
			continue
		}
		for _, account := range resp.GetAccounts() {
			if account == nil {
				continue
			}
			if _, err := h.accountClient.UpdateAccount(r.Context(), &pb.UpdateAccountRequest{Account: mailboxWebhookAccountUpdate(account, message)}); err != nil {
				log.Printf("mailbox webhook account update failed account_id=%s: %v", account.GetAccountId(), err)
				continue
			}
			updated++
		}
	}
	log.Printf("mailbox webhook accepted message_id=%s recipients=%d accounts_updated=%d", strings.TrimSpace(message.GetId()), len(recipients), updated)
	writeGoPayOTPWebhookJSON(w, http.StatusAccepted, goPayOTPWebhookResponse{Success: true})
}

func (h goPayOTPWebhookHandler) validMailboxWebhookToken(r *http.Request) bool {
	expected := strings.TrimSpace(h.mailboxWebhookToken)
	if expected == "" {
		return false
	}
	token := strings.TrimSpace(r.Header.Get("X-Webhook-Token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[len("bearer "):])
		}
	}
	return token == expected
}

func mailboxWebhookAccountUpdate(account *pb.Account, message *pb.EmailInboxMessage) *pb.Account {
	update := &pb.Account{
		AccountId:                account.GetAccountId(),
		PrimaryMailboxEmail:      mailboxWebhookPrimaryMailbox(message),
		MailboxLastMessageAtUnix: message.GetReceivedAtUnix(),
	}
	state := accountmail.DetectState(account, []*pb.EmailInboxMessage{message})
	if state.Status != "" {
		update.Status = state.Status
		update.ErrorMessage = state.ErrorMessage
	}
	if otp := accountmail.LatestOTP(account, []*pb.EmailInboxMessage{message}); otp.Code != "" {
		update.MailboxLatestOtp = otp.Code
		update.MailboxLatestOtpSubject = otp.Subject
		update.MailboxLatestOtpReceivedAtUnix = otp.ReceivedAtUnix
	}
	if state.PlusActive {
		value := true
		update.PlusActive = &value
		update.Tier = "plus"
	}
	return update
}

func mailboxWebhookMessage(message *pb.EmailInboxMessage) *pb.EmailInboxMessage {
	if message == nil {
		message = &pb.EmailInboxMessage{}
	}
	receivedAt := message.GetReceivedAtUnix()
	if receivedAt <= 0 {
		receivedAt = time.Now().Unix()
	}
	clone := *message
	clone.ReceivedAtUnix = receivedAt
	clone.MailboxEmail = accountmail.NormalizeEmail(message.GetMailboxEmail())
	clone.SourceMailboxEmail = accountmail.NormalizeEmail(message.GetSourceMailboxEmail())
	clone.FromAddress = accountmail.NormalizeEmail(message.GetFromAddress())
	clone.Recipients = mailboxWebhookRecipients(message)
	clone.Subject = strings.TrimSpace(message.GetSubject())
	clone.BodyPreview = strings.TrimSpace(message.GetBodyPreview())
	clone.BodyText = strings.TrimSpace(message.GetBodyText())
	clone.HtmlBody = strings.TrimSpace(message.GetHtmlBody())
	clone.Provider = strings.TrimSpace(message.GetProvider())
	return &clone
}

func mailboxWebhookPrimaryMailbox(message *pb.EmailInboxMessage) string {
	if value := accountmail.NormalizeEmail(message.GetSourceMailboxEmail()); value != "" {
		return value
	}
	if value := accountmail.NormalizeEmail(message.GetMailboxEmail()); value != "" {
		return value
	}
	return accountmail.CanonicalEmail(firstNonEmptyString(message.GetRecipients()...))
}

func mailboxWebhookRecipients(message *pb.EmailInboxMessage) []string {
	if message == nil {
		return nil
	}
	return uniqueMailboxWebhookEmails(append([]string{message.GetMailboxEmail()}, message.GetRecipients()...))
}

func uniqueMailboxWebhookEmails(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		email := accountmail.NormalizeEmail(value)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func redactMailboxWebhookEmail(email string) string {
	email = accountmail.NormalizeEmail(email)
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return "<redacted>"
	}
	if len(local) > 2 {
		local = local[:2]
	}
	return local + "***@" + domain
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
