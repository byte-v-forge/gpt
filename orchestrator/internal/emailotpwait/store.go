package emailotpwait

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/redis/go-redis/v9"
)

const defaultKeyPrefix = "byte-v-forge:gpt:email-otp-wait"

type Entry struct {
	JobID           string `json:"job_id"`
	AccountID       string `json:"account_id"`
	FlowID          string `json:"flow_id"`
	Email           string `json:"email"`
	StepName        string `json:"step_name,omitempty"`
	IssuedAfterUnix int64  `json:"issued_after_unix"`
	TimeoutSeconds  int32  `json:"timeout_seconds"`
	ResumeSecretKey string `json:"resume_secret_key"`
	N8NExecutionID  string `json:"n8n_execution_id,omitempty"`
}

type Store struct {
	client redis.Cmdable
	keys   redisx.Keyspace
}

func NewStore(client redis.Cmdable, keyPrefix string) (*Store, error) {
	if client == nil {
		return nil, errors.New("email otp wait redis client is required")
	}
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}
	return &Store{client: client, keys: redisx.NewKeyspace(keyPrefix)}, nil
}

func (s *Store) Register(ctx context.Context, entry Entry, ttl time.Duration) error {
	if s == nil {
		return errors.New("email otp wait store is required")
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
	for _, candidate := range EmailCandidates(entry.Email) {
		key := s.emailKey(candidate)
		pipe.SAdd(ctx, key, entry.JobID)
		pipe.Expire(ctx, key, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) PendingForEmails(ctx context.Context, emails []string, receivedAtUnix int64) ([]Entry, error) {
	if s == nil {
		return nil, errors.New("email otp wait store is required")
	}
	seenJobs := map[string]struct{}{}
	out := []Entry{}
	for _, candidate := range EmailCandidates(emails...) {
		ids, err := s.client.SMembers(ctx, s.emailKey(candidate)).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seenJobs[id]; ok {
				continue
			}
			seenJobs[id] = struct{}{}
			entry, ok, err := s.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if !ok {
				_ = s.client.SRem(ctx, s.emailKey(candidate), id).Err()
				continue
			}
			if entry.IssuedAfterUnix > 0 && receivedAtUnix > 0 && receivedAtUnix < entry.IssuedAfterUnix {
				continue
			}
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *Store) Get(ctx context.Context, jobID string) (Entry, bool, error) {
	if s == nil {
		return Entry{}, false, errors.New("email otp wait store is required")
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
	if strings.TrimSpace(entry.JobID) == "" {
		return nil
	}
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.jobKey(entry.JobID), s.claimKey(entry.JobID))
	for _, candidate := range EmailCandidates(entry.Email) {
		pipe.SRem(ctx, s.emailKey(candidate), entry.JobID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) Claim(ctx context.Context, jobID string, ttl time.Duration) (bool, error) {
	if s == nil {
		return false, errors.New("email otp wait store is required")
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

func (s *Store) jobKey(jobID string) string {
	return s.key("job", strings.TrimSpace(jobID))
}

func (s *Store) emailKey(email string) string {
	return s.key("email", emailx.Normalize(email))
}

func (s *Store) claimKey(jobID string) string {
	return s.key("claim", strings.TrimSpace(jobID))
}

func (s *Store) key(parts ...string) string {
	key, _ := s.keys.Key(strings.Join(parts, ":"))
	return key
}

func (e Entry) normalized() Entry {
	e.JobID = strings.TrimSpace(e.JobID)
	e.AccountID = strings.TrimSpace(e.AccountID)
	e.FlowID = strings.TrimSpace(e.FlowID)
	e.Email = emailx.Normalize(e.Email)
	e.StepName = strings.TrimSpace(e.StepName)
	e.ResumeSecretKey = strings.TrimSpace(e.ResumeSecretKey)
	e.N8NExecutionID = strings.TrimSpace(e.N8NExecutionID)
	return e
}

func (e Entry) validate() error {
	missing := []string{}
	if e.JobID == "" {
		missing = append(missing, "job_id")
	}
	if e.AccountID == "" {
		missing = append(missing, "account_id")
	}
	if e.FlowID == "" {
		missing = append(missing, "flow_id")
	}
	if e.Email == "" {
		missing = append(missing, "email")
	}
	if e.ResumeSecretKey == "" {
		missing = append(missing, "resume_secret_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("email otp wait missing %s", strings.Join(missing, ", "))
	}
	return nil
}

func EmailCandidates(values ...string) []string {
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
