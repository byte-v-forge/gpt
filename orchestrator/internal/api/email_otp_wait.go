package api

import (
	"context"
	"time"

	"orchestrator/internal/emailotpwait"
)

type emailOTPWaitStore interface {
	Register(ctx context.Context, entry emailotpwait.Entry, ttl time.Duration) error
	PendingForEmails(ctx context.Context, emails []string, receivedAtUnix int64) ([]emailotpwait.Entry, error)
	Get(ctx context.Context, jobID string) (emailotpwait.Entry, bool, error)
	Delete(ctx context.Context, entry emailotpwait.Entry) error
	Claim(ctx context.Context, jobID string, ttl time.Duration) (bool, error)
	ReleaseClaim(ctx context.Context, jobID string) error
}
