package otpprojection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/byte-v-forge/common-lib/timex"
	"github.com/redis/go-redis/v9"

	"orchestrator/internal/accountmail"
)

const (
	SourceSMS     = "sms"
	SourceMailbox = "mailbox"
	DefaultTTL    = 5 * time.Minute
)

type Store struct {
	client redis.Cmdable
	keys   redisx.Keyspace
	ttl    time.Duration
}

type otpRecord struct {
	Source         string `json:"source"`
	SMSOrderID     string `json:"sms_order_id,omitempty"`
	EmailAddress   string `json:"email_address,omitempty"`
	SignalKind     string `json:"signal_kind,omitempty"`
	Code           string `json:"code"`
	ReceivedAtUnix int64  `json:"received_at_unix"`
}

func NewStore(client redis.Cmdable, keyPrefix string, ttl time.Duration) (*Store, error) {
	if client == nil {
		return nil, errors.New("gpt otp projection redis client is required")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		keyPrefix = "byte-v-forge:gpt:otp-projection"
	}
	return &Store{client: client, keys: redisx.NewKeyspace(keyPrefix), ttl: ttl}, nil
}

func (s *Store) RecordSMSCode(ctx context.Context, event *smsv1.SmsCodeReceivedEvent) error {
	if s == nil || event == nil || event.GetCode() == nil {
		return nil
	}
	activationID := strings.TrimSpace(event.GetOrderId())
	code := strings.TrimSpace(event.GetCode().GetValue())
	if activationID == "" || code == "" {
		return nil
	}
	receivedAt := smsReceivedAt(event)
	return s.set(ctx, s.smsKey(activationID), otpRecord{
		Source:         SourceSMS,
		SMSOrderID:     activationID,
		Code:           code,
		ReceivedAtUnix: receivedAt,
	})
}

func (s *Store) RecordMailboxEmail(ctx context.Context, event *mailboxv1.MailboxEmailReceivedEvent) error {
	if s == nil || event == nil || event.GetMessage() == nil {
		return nil
	}
	message := accountmail.EnrichMessage(event.GetMessage())
	if !accountmail.IsOpenAIMessage(message) {
		return nil
	}
	code := accountmail.OTPCode(message)
	if code == "" {
		return nil
	}
	record := otpRecord{
		Source:         SourceMailbox,
		EmailAddress:   emailx.Normalize(message.GetMailboxEmail()),
		SignalKind:     mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP.String(),
		Code:           code,
		ReceivedAtUnix: message.GetReceivedAtUnix(),
	}
	if record.ReceivedAtUnix <= 0 {
		record.ReceivedAtUnix = time.Now().Unix()
	}
	for _, email := range emailCandidates(append([]string{message.GetMailboxEmail(), message.GetSourceMailboxEmail()}, message.GetRecipients()...)...) {
		next := record
		next.EmailAddress = email
		if err := s.set(ctx, s.mailboxKey(email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP), next); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LatestSMSCode(ctx context.Context, activationID string, issuedAfterUnix int64) (string, bool, error) {
	activationID = strings.TrimSpace(activationID)
	if activationID == "" {
		return "", false, errors.New("activation id missing")
	}
	record, ok, err := s.get(ctx, s.smsKey(activationID))
	if err != nil || !ok || record.Code == "" || record.ReceivedAtUnix < issuedAfterUnix {
		return "", false, err
	}
	return record.Code, true, nil
}

func (s *Store) WaitSMSCode(ctx context.Context, activationID string, issuedAfterUnix int64, timeout time.Duration, interval time.Duration) (string, bool, error) {
	activationID = strings.TrimSpace(activationID)
	if activationID == "" {
		return "", false, errors.New("activation id missing")
	}
	deadline := deadlineFromTimeout(timeout)
	for {
		record, ok, err := s.get(ctx, s.smsKey(activationID))
		if err != nil || (ok && record.Code != "" && record.ReceivedAtUnix >= issuedAfterUnix) || !time.Now().Before(deadline) {
			return record.Code, ok && record.Code != "" && record.ReceivedAtUnix >= issuedAfterUnix, err
		}
		if err := timex.Sleep(ctx, waitInterval(interval)); err != nil {
			return "", false, err
		}
	}
}

func (s *Store) WaitMailboxSignal(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64, timeout time.Duration, interval time.Duration) (*mailboxv1.EmailInboxMessage, string, bool, error) {
	email = emailx.Normalize(email)
	if email == "" {
		return nil, "", false, errors.New("email address missing")
	}
	if kind == mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED {
		kind = mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP
	}
	deadline := deadlineFromTimeout(timeout)
	for {
		record, ok, err := s.latestMailboxRecord(ctx, email, kind, issuedAfterUnix)
		if err != nil || ok || !time.Now().Before(deadline) {
			return record.message(), record.Code, ok, err
		}
		if err := timex.Sleep(ctx, waitInterval(interval)); err != nil {
			return nil, "", false, err
		}
	}
}

func (s *Store) LatestMailboxSignal(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64) (*mailboxv1.EmailInboxMessage, string, bool, error) {
	if kind == mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED {
		kind = mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP
	}
	record, ok, err := s.latestMailboxRecord(ctx, email, kind, issuedAfterUnix)
	return record.message(), record.Code, ok, err
}

func (s *Store) latestMailboxRecord(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64) (otpRecord, bool, error) {
	var best otpRecord
	found := false
	for _, candidate := range emailCandidates(email) {
		record, ok, err := s.get(ctx, s.mailboxKey(candidate, kind))
		if err != nil {
			return otpRecord{}, false, err
		}
		if !ok || record.Code == "" || record.ReceivedAtUnix < issuedAfterUnix {
			continue
		}
		if !found || record.ReceivedAtUnix > best.ReceivedAtUnix {
			best = record
			found = true
		}
	}
	return best, found, nil
}

func (s *Store) set(ctx context.Context, key string, record otpRecord) error {
	if s == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, payload, s.ttl).Err()
}

func (s *Store) get(ctx context.Context, key string) (otpRecord, bool, error) {
	if s == nil || strings.TrimSpace(key) == "" {
		return otpRecord{}, false, nil
	}
	payload, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return otpRecord{}, false, nil
	}
	if err != nil {
		return otpRecord{}, false, err
	}
	var record otpRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return otpRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) smsKey(activationID string) string {
	return s.key("sms", strings.TrimSpace(activationID))
}

func (s *Store) mailboxKey(email string, kind mailboxv1.EmailSignalKind) string {
	return s.key("mailbox", emailx.Normalize(email), kind.String())
}

func (s *Store) key(parts ...string) string {
	key, ok := s.keys.Key(strings.Join(parts, ":"))
	if !ok {
		return ""
	}
	return key
}

func (r otpRecord) message() *mailboxv1.EmailInboxMessage {
	if strings.TrimSpace(r.EmailAddress) == "" && strings.TrimSpace(r.Code) == "" {
		return nil
	}
	signal := &mailboxv1.EmailSignal{Kind: mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP, Code: r.Code, Label: "verification_code", Profile: "gpt", Parser: "gpt-account-mail", Confidence: 70}
	return &mailboxv1.EmailInboxMessage{
		MailboxEmail:   r.EmailAddress,
		ReceivedAtUnix: r.ReceivedAtUnix,
		PrimarySignal:  signal,
		Signals:        []*mailboxv1.EmailSignal{signal},
	}
}

func smsReceivedAt(event *smsv1.SmsCodeReceivedEvent) int64 {
	if event.GetCode() != nil && event.GetCode().GetReceivedAt() != nil {
		return event.GetCode().GetReceivedAt().AsTime().Unix()
	}
	if event.GetContext() != nil && event.GetContext().GetOccurredAt() != nil {
		return event.GetContext().GetOccurredAt().AsTime().Unix()
	}
	return time.Now().Unix()
}

func deadlineFromTimeout(timeout time.Duration) time.Time {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return time.Now().Add(timeout)
}

func emailCandidates(values ...string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		for _, candidate := range []string{value, emailx.CanonicalPlusAlias(value)} {
			candidate = emailx.Normalize(candidate)
			if candidate == "" {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}

func waitInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return interval
}
