package gopayotp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRedisKeyPrefix = "byte-v-forge:gpt:gopay-otp"

type RedisRelay struct {
	client   redis.Cmdable
	prefix   string
	ttl      time.Duration
	maxItems int64
}

func NewRedisRelay(client redis.Cmdable, prefix string, ttl time.Duration, maxItems int) *RedisRelay {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), ":")
	if prefix == "" {
		prefix = defaultRedisKeyPrefix
	}
	return &RedisRelay{client: client, prefix: prefix, ttl: ttl, maxItems: int64(maxItems)}
}

func (r *RedisRelay) Put(purpose string, otp string, source string) (Entry, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return Entry{}, fmt.Errorf("otp purpose is required")
	}
	otp = normalizeOTP(otp)
	if otp == "" {
		return Entry{}, fmt.Errorf("otp is required")
	}
	if r == nil || r.client == nil {
		return Entry{}, fmt.Errorf("redis otp relay is not configured")
	}
	now := time.Now().UTC()
	entry := Entry{
		Purpose:    purpose,
		Source:     normalizeWebhookSource(source),
		OTP:        otp,
		ReceivedAt: now,
		ExpiresAt:  now.Add(r.ttl),
	}
	key := r.key(purpose)
	ctx := context.Background()
	if _, err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: r.maxItems,
		Approx: true,
		Values: map[string]any{
			"purpose":     entry.Purpose,
			"source":      entry.Source,
			"otp":         entry.OTP,
			"received_at": entry.ReceivedAt.Unix(),
			"expires_at":  entry.ExpiresAt.Unix(),
		},
	}).Result(); err != nil {
		return Entry{}, fmt.Errorf("append otp relay stream: %w", err)
	}
	if err := r.client.Expire(ctx, key, r.ttl).Err(); err != nil {
		return Entry{}, fmt.Errorf("expire otp relay stream: %w", err)
	}
	return entry, nil
}

func (r *RedisRelay) Wait(ctx context.Context, purpose string, issuedAfterUnix int64, timeout time.Duration) (Entry, bool, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return Entry{}, false, fmt.Errorf("otp purpose is required")
	}
	if r == nil || r.client == nil {
		return Entry{}, false, fmt.Errorf("redis otp relay is not configured")
	}
	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	key := r.key(purpose)
	lastID, entry, found, err := r.takeExisting(waitCtx, key, purpose, issuedAfterUnix)
	if err != nil || found {
		return entry, found, err
	}
	for {
		if err := waitCtx.Err(); err != nil {
			return Entry{}, false, relayWaitErr(ctx, err)
		}
		streams, err := r.client.XRead(waitCtx, &redis.XReadArgs{
			Streams: []string{key, lastID},
			Count:   10,
			Block:   pollInterval,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if waitCtx.Err() != nil {
				return Entry{}, false, relayWaitErr(ctx, waitCtx.Err())
			}
			return Entry{}, false, fmt.Errorf("read otp relay stream: %w", err)
		}
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				lastID = msg.ID
				entry, ok := redisMessageEntry(purpose, msg)
				if !ok {
					continue
				}
				if entry.ExpiresAt.Before(time.Now().UTC()) {
					_ = r.client.XDel(waitCtx, key, msg.ID).Err()
					continue
				}
				if issuedAfterUnix > 0 && entry.ReceivedAt.Unix() < issuedAfterUnix {
					continue
				}
				consumed, err := r.consume(waitCtx, key, msg.ID)
				if err != nil {
					return Entry{}, false, err
				}
				if !consumed {
					continue
				}
				return entry, true, nil
			}
		}
	}
}

func (r *RedisRelay) takeExisting(ctx context.Context, key string, purpose string, issuedAfterUnix int64) (string, Entry, bool, error) {
	messages, err := r.client.XRevRangeN(ctx, key, "+", "-", r.maxItems).Result()
	if errors.Is(err, redis.Nil) {
		return "0-0", Entry{}, false, nil
	}
	if err != nil {
		return "", Entry{}, false, fmt.Errorf("scan otp relay stream: %w", err)
	}
	lastID := "0-0"
	if len(messages) > 0 {
		lastID = messages[0].ID
	}
	now := time.Now().UTC()
	for _, msg := range messages {
		entry, ok := redisMessageEntry(purpose, msg)
		if !ok {
			continue
		}
		if entry.ExpiresAt.Before(now) {
			_ = r.client.XDel(ctx, key, msg.ID).Err()
			continue
		}
		if issuedAfterUnix > 0 && entry.ReceivedAt.Unix() < issuedAfterUnix {
			continue
		}
		consumed, err := r.consume(ctx, key, msg.ID)
		if err != nil {
			return "", Entry{}, false, err
		}
		if !consumed {
			continue
		}
		return lastID, entry, true, nil
	}
	return lastID, Entry{}, false, nil
}

func (r *RedisRelay) consume(ctx context.Context, key string, id string) (bool, error) {
	deleted, err := r.client.XDel(ctx, key, id).Result()
	if err != nil {
		return false, fmt.Errorf("consume otp relay stream: %w", err)
	}
	return deleted > 0, nil
}

func (r *RedisRelay) key(purpose string) string {
	return r.prefix + ":" + strings.TrimSpace(purpose)
}

func redisMessageEntry(defaultPurpose string, msg redis.XMessage) (Entry, bool) {
	otp := normalizeOTP(redisString(msg.Values, "otp"))
	if otp == "" {
		return Entry{}, false
	}
	receivedAt := redisInt64(msg.Values, "received_at")
	if receivedAt <= 0 {
		receivedAt = time.Now().UTC().Unix()
	}
	expiresAt := redisInt64(msg.Values, "expires_at")
	if expiresAt <= 0 {
		expiresAt = time.Now().UTC().Add(defaultTTL).Unix()
	}
	purpose := strings.TrimSpace(redisString(msg.Values, "purpose"))
	if purpose == "" {
		purpose = defaultPurpose
	}
	source := normalizeWebhookSource(redisString(msg.Values, "source"))
	return Entry{Purpose: purpose, Source: source, OTP: otp, ReceivedAt: time.Unix(receivedAt, 0).UTC(), ExpiresAt: time.Unix(expiresAt, 0).UTC()}, true
}

func redisString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func redisInt64(values map[string]any, key string) int64 {
	raw := redisString(values, key)
	if raw == "" {
		return 0
	}
	value, _ := strconv.ParseInt(raw, 10, 64)
	return value
}

func relayWaitErr(parent context.Context, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
