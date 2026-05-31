package otpwaitstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/redis/go-redis/v9"
)

type Config[E any] struct {
	RequiredError    string
	Normalize        func(E) E
	Validate         func(E) error
	JobID            func(E) string
	Indexes          func(E) []string
	IssuedAfterUnix  func(E) int64
	TimeoutSeconds   func(E) int32
	DropAfterTimeout bool
}

type Store[E any] struct {
	client redis.Cmdable
	keys   redisx.Keyspace
	cfg    Config[E]
}

func New[E any](client redis.Cmdable, keyPrefix string, cfg Config[E]) (*Store[E], error) {
	if client == nil {
		return nil, errors.New(requiredError(cfg))
	}
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		return nil, errors.New("otp wait key prefix is required")
	}
	if cfg.JobID == nil || cfg.Indexes == nil || cfg.Validate == nil {
		return nil, errors.New("otp wait store entry callbacks are required")
	}
	return &Store[E]{client: client, keys: redisx.NewKeyspace(keyPrefix), cfg: cfg}, nil
}

func (s *Store[E]) Register(ctx context.Context, entry E, ttl time.Duration) error {
	if s == nil {
		return errors.New("otp wait store is required")
	}
	entry = s.normalize(entry)
	if err := s.cfg.Validate(entry); err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	jobID := s.jobID(entry)
	if old, ok, err := s.Get(ctx, jobID); err != nil {
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
	pipe.Set(ctx, s.jobKey(jobID), payload, ttl)
	for _, index := range s.indexes(entry) {
		key := s.indexKey(index)
		pipe.SAdd(ctx, key, jobID)
		pipe.Expire(ctx, key, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store[E]) Pending(ctx context.Context, indexes []string, receivedAtUnix int64) ([]E, error) {
	if s == nil {
		return nil, errors.New("otp wait store is required")
	}
	seenJobs := map[string]struct{}{}
	out := []E{}
	for _, index := range normalizeParts(indexes...) {
		indexKey := s.indexKey(index)
		ids, err := s.client.SMembers(ctx, indexKey).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		for _, id := range normalizeParts(ids...) {
			if _, ok := seenJobs[id]; ok {
				continue
			}
			seenJobs[id] = struct{}{}
			entry, ok, err := s.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if !ok {
				_ = s.client.SRem(ctx, indexKey, id).Err()
				continue
			}
			if !s.accepts(entry, receivedAtUnix) {
				continue
			}
			out = append(out, entry)
		}
	}
	if out == nil {
		return []E{}, nil
	}
	return out, nil
}

func (s *Store[E]) Get(ctx context.Context, jobID string) (E, bool, error) {
	var zero E
	if s == nil {
		return zero, false, errors.New("otp wait store is required")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return zero, false, nil
	}
	payload, err := s.client.Get(ctx, s.jobKey(jobID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	var entry E
	if err := json.Unmarshal(payload, &entry); err != nil {
		return zero, false, err
	}
	return s.normalize(entry), true, nil
}

func (s *Store[E]) Delete(ctx context.Context, entry E) error {
	if s == nil {
		return nil
	}
	entry = s.normalize(entry)
	jobID := s.jobID(entry)
	if jobID == "" {
		return nil
	}
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.jobKey(jobID), s.claimKey(jobID))
	for _, index := range s.indexes(entry) {
		pipe.SRem(ctx, s.indexKey(index), jobID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store[E]) Claim(ctx context.Context, jobID string, ttl time.Duration) (bool, error) {
	if s == nil {
		return false, errors.New("otp wait store is required")
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

func (s *Store[E]) ReleaseClaim(ctx context.Context, jobID string) error {
	if s == nil {
		return nil
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}
	return s.client.Del(ctx, s.claimKey(jobID)).Err()
}

func (s *Store[E]) normalize(entry E) E {
	if s.cfg.Normalize == nil {
		return entry
	}
	return s.cfg.Normalize(entry)
}

func (s *Store[E]) jobID(entry E) string {
	if s.cfg.JobID == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.JobID(entry))
}

func (s *Store[E]) indexes(entry E) []string {
	if s.cfg.Indexes == nil {
		return nil
	}
	return normalizeParts(s.cfg.Indexes(entry)...)
}

func (s *Store[E]) accepts(entry E, receivedAtUnix int64) bool {
	issuedAfter := int64(0)
	if s.cfg.IssuedAfterUnix != nil {
		issuedAfter = s.cfg.IssuedAfterUnix(entry)
	}
	if issuedAfter > 0 && receivedAtUnix > 0 && receivedAtUnix < issuedAfter {
		return false
	}
	if s.cfg.DropAfterTimeout && issuedAfter > 0 && receivedAtUnix > 0 && s.cfg.TimeoutSeconds != nil {
		timeout := s.cfg.TimeoutSeconds(entry)
		if timeout > 0 && receivedAtUnix > issuedAfter+int64(timeout) {
			return false
		}
	}
	return true
}

func (s *Store[E]) jobKey(jobID string) string { return s.key("job", strings.TrimSpace(jobID)) }
func (s *Store[E]) indexKey(index string) string {
	return s.key("index", strings.TrimSpace(index))
}
func (s *Store[E]) claimKey(jobID string) string { return s.key("claim", strings.TrimSpace(jobID)) }
func (s *Store[E]) key(parts ...string) string {
	key, _ := s.keys.Key(strings.Join(parts, ":"))
	return key
}

func requiredError[E any](cfg Config[E]) string {
	if text := strings.TrimSpace(cfg.RequiredError); text != "" {
		return text
	}
	return "otp wait redis client is required"
}

func normalizeParts(values ...string) []string {
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
