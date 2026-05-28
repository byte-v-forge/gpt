package appsvc

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/redisx"
)

type StateStore struct {
	clientClose func() error
	values      *redisx.StringStore
}

func NewStateStore(ctx context.Context, redisURL string, keyPrefix string, ttl time.Duration) (*StateStore, error) {
	client, err := redisx.NewRequiredClient(ctx, redisURL, "GOPAY_STATE_REDIS_URL is required for gopay-app runtime state")
	if err != nil {
		return nil, err
	}
	return &StateStore{
		clientClose: client.Close,
		values:      redisx.NewStringStore(client, keyPrefix, ttl),
	}, nil
}

func NormalizeStateKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "local" {
		return "local", nil
	}
	if strings.HasPrefix(value, "tg:") {
		userID := strings.TrimSpace(strings.TrimPrefix(value, "tg:"))
		if userID != "" && regexp.MustCompile(`^\d+$`).MatchString(userID) {
			return "tg:" + userID, nil
		}
	}
	return "", fmt.Errorf("user_id must be local or tg:<user_id>")
}

func (s *StateStore) Load(ctx context.Context, key string) (string, error) {
	if s == nil || s.values == nil {
		return "", fmt.Errorf("gopay-app state store is not configured")
	}
	stateJSON, found, err := s.values.Load(ctx, key)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(stateJSON) == "" {
		return "{}", nil
	}
	return stateJSON, nil
}

func (s *StateStore) Save(ctx context.Context, key string, raw string) (string, error) {
	if s == nil || s.values == nil {
		return "", fmt.Errorf("gopay-app state store is not configured")
	}
	state, err := parseState(raw)
	if err != nil {
		return "", err
	}
	normalized := stateJSON(state)
	if err := s.values.Save(ctx, key, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func (s *StateStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.values == nil {
		return fmt.Errorf("gopay-app state store is not configured")
	}
	return s.values.Delete(ctx, key)
}

func (s *StateStore) Close() error {
	if s == nil || s.clientClose == nil {
		return nil
	}
	return s.clientClose()
}
