package gopayotp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/byte-v-forge/common-lib/timex"
)

const (
	defaultTTL      = 10 * time.Minute
	defaultMaxItems = 100
	pollInterval    = 500 * time.Millisecond
)

type Entry struct {
	Purpose    string
	Source     string
	OTP        string
	ReceivedAt time.Time
	ExpiresAt  time.Time
}

type Relay interface {
	Put(purpose string, otp string, source string) (Entry, error)
	Wait(ctx context.Context, purpose string, issuedAfterUnix int64, timeout time.Duration) (Entry, bool, error)
}

type MemoryRelay struct {
	mu       sync.Mutex
	items    map[string][]Entry
	ttl      time.Duration
	maxItems int
}

func NewMemoryRelay(ttl time.Duration, maxItems int) *MemoryRelay {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}
	return &MemoryRelay{
		items:    make(map[string][]Entry),
		ttl:      ttl,
		maxItems: maxItems,
	}
}

func (r *MemoryRelay) Put(purpose string, otp string, source string) (Entry, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return Entry{}, fmt.Errorf("otp purpose is required")
	}
	otp = normalizeOTP(otp)
	if otp == "" {
		return Entry{}, fmt.Errorf("otp is required")
	}
	now := time.Now().UTC()
	entry := Entry{
		Purpose:    purpose,
		Source:     normalizeWebhookSource(source),
		OTP:        otp,
		ReceivedAt: now,
		ExpiresAt:  now.Add(r.ttl),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(now)
	r.items[purpose] = append(r.items[purpose], entry)
	r.trimLocked()
	return entry, nil
}

func (r *MemoryRelay) Wait(ctx context.Context, purpose string, issuedAfterUnix int64, timeout time.Duration) (Entry, bool, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return Entry{}, false, fmt.Errorf("otp purpose is required")
	}
	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	for {
		if entry, found := r.take(purpose, issuedAfterUnix); found {
			return entry, true, nil
		}
		if err := timex.Sleep(waitCtx, pollInterval); err != nil {
			if ctx.Err() != nil {
				return Entry{}, false, ctx.Err()
			}
			return Entry{}, false, nil
		}
	}
}

func QueueKey(source string, purpose string) (string, error) {
	source, err := NormalizeSource(source)
	if err != nil {
		return "", err
	}
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if purpose == "" {
		return "", fmt.Errorf("otp purpose is required")
	}
	if strings.Contains(purpose, "/") {
		return "", fmt.Errorf("otp purpose must be a single path segment")
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
	return "", fmt.Errorf("source must be local or tg:<user_id>")
}

func (r *MemoryRelay) take(purpose string, issuedAfterUnix int64) (Entry, bool) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(now)

	queue := r.items[purpose]
	for index, entry := range queue {
		if issuedAfterUnix > 0 && entry.ReceivedAt.Unix() < issuedAfterUnix {
			continue
		}
		r.items[purpose] = append(queue[:index], queue[index+1:]...)
		if len(r.items[purpose]) == 0 {
			delete(r.items, purpose)
		}
		return entry, true
	}
	return Entry{}, false
}

func (r *MemoryRelay) pruneLocked(now time.Time) {
	for purpose, queue := range r.items {
		kept := queue[:0]
		for _, entry := range queue {
			if entry.ExpiresAt.After(now) {
				kept = append(kept, entry)
			}
		}
		if len(kept) == 0 {
			delete(r.items, purpose)
			continue
		}
		r.items[purpose] = kept
	}
}

func (r *MemoryRelay) trimLocked() {
	total := 0
	for _, queue := range r.items {
		total += len(queue)
	}
	for total > r.maxItems {
		var oldestPurpose string
		oldestIndex := -1
		var oldestTime time.Time
		for purpose, queue := range r.items {
			for index, entry := range queue {
				if oldestIndex == -1 || entry.ReceivedAt.Before(oldestTime) {
					oldestPurpose = purpose
					oldestIndex = index
					oldestTime = entry.ReceivedAt
				}
			}
		}
		if oldestIndex < 0 {
			return
		}
		queue := r.items[oldestPurpose]
		r.items[oldestPurpose] = append(queue[:oldestIndex], queue[oldestIndex+1:]...)
		if len(r.items[oldestPurpose]) == 0 {
			delete(r.items, oldestPurpose)
		}
		total--
	}
}

func normalizeOTP(value string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "")
	return strings.TrimSpace(replacer.Replace(value))
}

func normalizeWebhookSource(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "webhook"
	}
	return value
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
