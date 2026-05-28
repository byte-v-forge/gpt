package runtimesecrets

import (
	"context"
	"time"
)

type Store interface {
	DefaultTTL() time.Duration
	Load(ctx context.Context, key string) (string, bool, error)
	LoadMany(ctx context.Context, keys ...string) (map[string]string, error)
	Save(ctx context.Context, key string, value string) error
	SaveTTL(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	HashLoadMany(ctx context.Context, key string, fields ...string) (map[string]string, error)
	HashSaveTTL(ctx context.Context, key string, values map[string]string, ttl time.Duration) error
	HashDelete(ctx context.Context, key string, fields ...string) error
}
