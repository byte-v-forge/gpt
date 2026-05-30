package paymentotpwait

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/redis/go-redis/v9"
)

const defaultKeyPrefix = "byte-v-forge:gpt:payment-otp-wait"

type Entry struct {
	JobID            string `json:"job_id"`
	AccountID        string `json:"account_id,omitempty"`
	Action           string `json:"action,omitempty"`
	Operation        string `json:"operation,omitempty"`
	UserID           string `json:"user_id,omitempty"`
	Source           string `json:"source"`
	Purpose          string `json:"purpose"`
	QueueKey         string `json:"queue_key"`
	StepName         string `json:"step_name"`
	IssuedAfterUnix  int64  `json:"issued_after_unix"`
	TimeoutSeconds   int32  `json:"timeout_seconds"`
	ResumeSecretKey  string `json:"resume_secret_key"`
	N8NExecutionID   string `json:"n8n_execution_id,omitempty"`
	OTPParam         string `json:"otp_param,omitempty"`
	SubmittedAtParam string `json:"submitted_at_param,omitempty"`
}

type CodeRecord struct {
	Source         string `json:"source"`
	OTPSource      string `json:"otp_source,omitempty"`
	Purpose        string `json:"purpose"`
	QueueKey       string `json:"queue_key"`
	Code           string `json:"code"`
	ReceivedAtUnix int64  `json:"received_at_unix"`
}

type Store struct {
	client redis.Cmdable
	keys   redisx.Keyspace
}

func NewStore(client redis.Cmdable, keyPrefix string) (*Store, error) {
	if client == nil {
		return nil, errors.New("payment otp wait redis client is required")
	}
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}
	return &Store{client: client, keys: redisx.NewKeyspace(keyPrefix)}, nil
}

func QueueKey(source string, purpose string) (string, error) {
	source, err := NormalizeSource(source)
	if err != nil {
		return "", err
	}
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if purpose == "" {
		return "", errors.New("otp purpose is required")
	}
	if strings.Contains(purpose, "/") {
		return "", errors.New("otp purpose must be a single path segment")
	}
	return source + "/" + purpose, nil
}

func NormalizeSource(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "local" {
		return "local", nil
	}
	if strings.HasPrefix(value, "tg:") {
		userID := strings.TrimSpace(strings.TrimPrefix(value, "tg:"))
		if validTelegramUserID(userID) {
			return "tg:" + userID, nil
		}
	}
	return "", errors.New("source must be local or tg:<user_id>")
}

func (s *Store) Register(ctx context.Context, entry Entry, ttl time.Duration) error {
	if s == nil {
		return errors.New("payment otp wait store is required")
	}
	entry = entry.normalized()
	if err := entry.validate(); err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if old, ok, err := s.Get(ctx, entry.JobID); err != nil {
		return err
	} else if ok {
		if err := s.Delete(ctx, old); err != nil {
			return err
		}
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.jobKey(entry.JobID), payload, ttl)
	pipe.SAdd(ctx, s.queueKey(entry.QueueKey), entry.JobID)
	pipe.Expire(ctx, s.queueKey(entry.QueueKey), ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) RecordCode(ctx context.Context, record CodeRecord, ttl time.Duration) error {
	if s == nil {
		return errors.New("payment otp wait store is required")
	}
	record = record.normalized()
	if record.QueueKey == "" || record.Code == "" {
		return nil
	}
	if record.ReceivedAtUnix <= 0 {
		record.ReceivedAtUnix = time.Now().Unix()
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.latestKey(record.QueueKey), payload, ttl).Err()
}

func (s *Store) LatestCode(ctx context.Context, queueKey string, issuedAfterUnix int64) (CodeRecord, bool, error) {
	if s == nil {
		return CodeRecord{}, false, errors.New("payment otp wait store is required")
	}
	queueKey = strings.TrimSpace(queueKey)
	if queueKey == "" {
		return CodeRecord{}, false, nil
	}
	payload, err := s.client.Get(ctx, s.latestKey(queueKey)).Bytes()
	if errors.Is(err, redis.Nil) {
		return CodeRecord{}, false, nil
	}
	if err != nil {
		return CodeRecord{}, false, err
	}
	var record CodeRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return CodeRecord{}, false, err
	}
	record = record.normalized()
	if record.Code == "" || record.ReceivedAtUnix < issuedAfterUnix {
		return CodeRecord{}, false, nil
	}
	return record, true, nil
}

func (s *Store) PendingForQueue(ctx context.Context, queueKey string, receivedAtUnix int64) ([]Entry, error) {
	if s == nil {
		return nil, errors.New("payment otp wait store is required")
	}
	queueKey = strings.TrimSpace(queueKey)
	if queueKey == "" {
		return nil, nil
	}
	ids, err := s.client.SMembers(ctx, s.queueKey(queueKey)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make([]Entry, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		entry, ok, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			_ = s.client.SRem(ctx, s.queueKey(queueKey), id).Err()
			continue
		}
		if entry.IssuedAfterUnix > 0 && receivedAtUnix > 0 && receivedAtUnix < entry.IssuedAfterUnix {
			continue
		}
		if entry.IssuedAfterUnix > 0 && receivedAtUnix > 0 && entry.TimeoutSeconds > 0 && receivedAtUnix > entry.IssuedAfterUnix+int64(entry.TimeoutSeconds) {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s *Store) Get(ctx context.Context, jobID string) (Entry, bool, error) {
	if s == nil {
		return Entry{}, false, errors.New("payment otp wait store is required")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return Entry{}, false, nil
	}
	payload, err := s.client.Get(ctx, s.jobKey(jobID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return Entry{}, false, err
	}
	return entry.normalized(), true, nil
}

func (s *Store) Delete(ctx context.Context, entry Entry) error {
	if s == nil {
		return nil
	}
	entry = entry.normalized()
	if entry.JobID == "" {
		return nil
	}
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.jobKey(entry.JobID), s.claimKey(entry.JobID))
	if entry.QueueKey != "" {
		pipe.SRem(ctx, s.queueKey(entry.QueueKey), entry.JobID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) Claim(ctx context.Context, jobID string, ttl time.Duration) (bool, error) {
	if s == nil {
		return false, errors.New("payment otp wait store is required")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return false, nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	return s.client.SetNX(ctx, s.claimKey(jobID), "1", ttl).Result()
}

func (s *Store) ReleaseClaim(ctx context.Context, jobID string) error {
	if s == nil {
		return nil
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}
	return s.client.Del(ctx, s.claimKey(jobID)).Err()
}

func (s *Store) jobKey(jobID string) string      { return s.key("job", strings.TrimSpace(jobID)) }
func (s *Store) queueKey(queueKey string) string { return s.key("queue", strings.TrimSpace(queueKey)) }
func (s *Store) latestKey(queueKey string) string {
	return s.key("latest", strings.TrimSpace(queueKey))
}
func (s *Store) claimKey(jobID string) string { return s.key("claim", strings.TrimSpace(jobID)) }
func (s *Store) key(parts ...string) string {
	key, _ := s.keys.Key(strings.Join(parts, ":"))
	return key
}

func (e Entry) normalized() Entry {
	e.JobID = strings.TrimSpace(e.JobID)
	e.AccountID = strings.TrimSpace(e.AccountID)
	e.Action = strings.ToUpper(strings.TrimSpace(e.Action))
	e.Operation = strings.TrimSpace(e.Operation)
	e.UserID = strings.TrimSpace(e.UserID)
	e.Source, _ = NormalizeSource(e.Source)
	e.Purpose = strings.ToLower(strings.TrimSpace(e.Purpose))
	if e.QueueKey == "" && e.Source != "" && e.Purpose != "" {
		e.QueueKey, _ = QueueKey(e.Source, e.Purpose)
	}
	e.QueueKey = strings.TrimSpace(e.QueueKey)
	e.StepName = strings.TrimSpace(e.StepName)
	e.ResumeSecretKey = strings.TrimSpace(e.ResumeSecretKey)
	e.N8NExecutionID = strings.TrimSpace(e.N8NExecutionID)
	e.OTPParam = strings.TrimSpace(e.OTPParam)
	e.SubmittedAtParam = strings.TrimSpace(e.SubmittedAtParam)
	return e
}

func (e Entry) validate() error {
	missing := []string{}
	if e.JobID == "" {
		missing = append(missing, "job_id")
	}
	if e.Source == "" {
		missing = append(missing, "source")
	}
	if e.Purpose == "" {
		missing = append(missing, "purpose")
	}
	if e.QueueKey == "" {
		missing = append(missing, "queue_key")
	}
	if e.StepName == "" {
		missing = append(missing, "step_name")
	}
	if e.ResumeSecretKey == "" {
		missing = append(missing, "resume_secret_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("payment otp wait missing %s", strings.Join(missing, ", "))
	}
	return nil
}

func (r CodeRecord) normalized() CodeRecord {
	r.Source, _ = NormalizeSource(r.Source)
	r.Purpose = strings.ToLower(strings.TrimSpace(r.Purpose))
	if r.QueueKey == "" && r.Source != "" && r.Purpose != "" {
		r.QueueKey, _ = QueueKey(r.Source, r.Purpose)
	}
	r.QueueKey = strings.TrimSpace(r.QueueKey)
	r.OTPSource = strings.TrimSpace(r.OTPSource)
	r.Code = normalizeOTP(r.Code)
	return r
}

func normalizeOTP(value string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "")
	return strings.TrimSpace(replacer.Replace(value))
}

func validTelegramUserID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
