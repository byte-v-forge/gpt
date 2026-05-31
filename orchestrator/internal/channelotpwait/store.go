package channelotpwait

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/redis/go-redis/v9"

	"orchestrator/internal/otpwaitstore"
)

const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
	ChannelWA    = "wa"
)

const (
	defaultEmailTimeoutSeconds = 180
	defaultSMSTimeoutSeconds   = 300
	defaultWATimeoutSeconds    = 300
	defaultOTPTimeoutSeconds   = 300
)

type Config struct {
	RequiredError      string
	MissingErrorPrefix string
}

type Entry struct {
	JobID            string `json:"job_id"`
	AccountID        string `json:"account_id,omitempty"`
	Action           string `json:"action,omitempty"`
	Operation        string `json:"operation,omitempty"`
	FlowID           string `json:"flow_id,omitempty"`
	Channel          string `json:"channel"`
	Target           string `json:"target"`
	StepName         string `json:"step_name"`
	IssuedAfterUnix  int64  `json:"issued_after_unix"`
	TimeoutSeconds   int32  `json:"timeout_seconds"`
	ResumeSecretKey  string `json:"resume_secret_key"`
	N8NExecutionID   string `json:"n8n_execution_id,omitempty"`
	OTPParam         string `json:"otp_param,omitempty"`
	SubmittedAtParam string `json:"submitted_at_param,omitempty"`
}

type Store struct {
	cfg   Config
	inner *otpwaitstore.Store[Entry]
}

func NewStore(client redis.Cmdable, keyPrefix string, cfg Config) (*Store, error) {
	cfg = normalizeConfig(cfg)
	keyPrefix = strings.TrimSpace(keyPrefix)
	store := &Store{cfg: cfg}
	inner, err := otpwaitstore.New(client, keyPrefix, otpwaitstore.Config[Entry]{
		RequiredError:    cfg.RequiredError,
		Normalize:        store.normalize,
		Validate:         store.validate,
		JobID:            func(entry Entry) string { return entry.JobID },
		Indexes:          func(entry Entry) []string { return indexes(entry.Channel, entry.Target) },
		IssuedAfterUnix:  func(entry Entry) int64 { return entry.IssuedAfterUnix },
		TimeoutSeconds:   func(entry Entry) int32 { return entry.TimeoutSeconds },
		DropAfterTimeout: true,
	})
	if err != nil {
		return nil, err
	}
	store.inner = inner
	return store, nil
}

func (s *Store) Register(ctx context.Context, entry Entry, ttl time.Duration) error {
	if s == nil || s.inner == nil {
		return errors.New("channel otp wait store is required")
	}
	return s.inner.Register(ctx, entry, ttl)
}

func (s *Store) Pending(ctx context.Context, channel string, target string, receivedAtUnix int64) ([]Entry, error) {
	if s == nil || s.inner == nil {
		return nil, errors.New("channel otp wait store is required")
	}
	candidateIndexes := indexes(channel, target)
	if len(candidateIndexes) == 0 {
		return nil, nil
	}
	return s.inner.Pending(ctx, candidateIndexes, receivedAtUnix)
}

func (s *Store) Get(ctx context.Context, jobID string) (Entry, bool, error) {
	if s == nil || s.inner == nil {
		return Entry{}, false, errors.New("channel otp wait store is required")
	}
	return s.inner.Get(ctx, jobID)
}

func (s *Store) Delete(ctx context.Context, entry Entry) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Delete(ctx, entry)
}

func (s *Store) Claim(ctx context.Context, jobID string, ttl time.Duration) (bool, error) {
	if s == nil || s.inner == nil {
		return false, errors.New("channel otp wait store is required")
	}
	return s.inner.Claim(ctx, jobID, ttl)
}

func (s *Store) ReleaseClaim(ctx context.Context, jobID string) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.ReleaseClaim(ctx, jobID)
}

func (s *Store) normalize(entry Entry) Entry {
	entry.JobID = strings.TrimSpace(entry.JobID)
	entry.AccountID = strings.TrimSpace(entry.AccountID)
	entry.Action = strings.ToUpper(strings.TrimSpace(entry.Action))
	entry.Operation = strings.TrimSpace(entry.Operation)
	entry.FlowID = strings.TrimSpace(entry.FlowID)
	entry.Channel = NormalizeChannel(entry.Channel)
	entry.Target = NormalizeTarget(entry.Channel, entry.Target)
	entry.StepName = strings.TrimSpace(entry.StepName)
	entry.ResumeSecretKey = strings.TrimSpace(entry.ResumeSecretKey)
	entry.N8NExecutionID = strings.TrimSpace(entry.N8NExecutionID)
	entry.OTPParam = strings.TrimSpace(entry.OTPParam)
	entry.SubmittedAtParam = strings.TrimSpace(entry.SubmittedAtParam)
	return entry
}

func (s *Store) validate(entry Entry) error {
	missing := []string{}
	if entry.JobID == "" {
		missing = append(missing, "job_id")
	}
	if entry.Channel == "" {
		missing = append(missing, "channel")
	}
	if entry.Target == "" {
		missing = append(missing, "target")
	}
	if entry.StepName == "" {
		missing = append(missing, "step_name")
	}
	if entry.ResumeSecretKey == "" {
		missing = append(missing, "resume_secret_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s missing %s", s.cfg.MissingErrorPrefix, strings.Join(missing, ", "))
	}
	return nil
}

func NormalizeChannel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeCode(value string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "")
	return strings.TrimSpace(replacer.Replace(value))
}

func DefaultTimeoutSeconds(channel string) int32 {
	switch NormalizeChannel(channel) {
	case ChannelEmail:
		return defaultEmailTimeoutSeconds
	case ChannelSMS:
		return defaultSMSTimeoutSeconds
	case ChannelWA:
		return defaultWATimeoutSeconds
	default:
		return defaultOTPTimeoutSeconds
	}
}

func NormalizeTarget(channel string, target string) string {
	switch NormalizeChannel(channel) {
	case ChannelEmail:
		return emailx.Normalize(target)
	case ChannelWA:
		if number := NormalizeNumber(target); number != "" {
			return number
		}
		return strings.TrimSpace(target)
	default:
		return strings.TrimSpace(target)
	}
}

func EmailCandidates(values ...string) []string {
	out := []string{}
	for _, value := range values {
		out = append(out, value, emailx.CanonicalPlusAlias(value))
	}
	return uniqueNonEmptyEmail(out...)
}

func PhoneCandidates(values ...string) []string {
	out := []string{}
	for _, value := range values {
		number := NormalizeNumber(value)
		if number == "" {
			continue
		}
		out = append(out, number, strings.TrimPrefix(number, "+"))
	}
	return uniqueNonEmpty(out...)
}

func NormalizeNumber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	hasDigit := false
	for i, r := range value {
		if i == 0 && r == '+' {
			builder.WriteRune(r)
			continue
		}
		if unicode.IsDigit(r) {
			hasDigit = true
			builder.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) || r == '-' || r == '(' || r == ')' || r == '.' {
			continue
		}
		return ""
	}
	if !hasDigit {
		return ""
	}
	return strings.TrimSpace(builder.String())
}

func normalizeConfig(cfg Config) Config {
	cfg.RequiredError = strings.TrimSpace(cfg.RequiredError)
	if cfg.RequiredError == "" {
		cfg.RequiredError = "channel otp wait redis client is required"
	}
	cfg.MissingErrorPrefix = strings.TrimSpace(cfg.MissingErrorPrefix)
	if cfg.MissingErrorPrefix == "" {
		cfg.MissingErrorPrefix = "channel otp wait"
	}
	return cfg
}

func indexes(channel string, target string) []string {
	channel = NormalizeChannel(channel)
	if channel == "" {
		return nil
	}
	candidates := []string{NormalizeTarget(channel, target)}
	switch channel {
	case ChannelEmail:
		candidates = EmailCandidates(target)
	case ChannelWA:
		candidates = append(PhoneCandidates(target), strings.TrimSpace(target))
	}
	out := []string{}
	for _, candidate := range uniqueNonEmpty(candidates...) {
		out = append(out, channel+":"+candidate)
	}
	return out
}

func uniqueNonEmptyEmail(values ...string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = emailx.Normalize(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
