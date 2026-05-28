package api

import (
	"context"
	"time"

	"orchestrator/internal/registerotpwait"
)

type registerProtocolOTPWaitStore interface {
	Register(ctx context.Context, entry registerotpwait.Entry, ttl time.Duration) error
	PendingForEmails(ctx context.Context, emails []string, receivedAtUnix int64) ([]registerotpwait.Entry, error)
	Get(ctx context.Context, jobID string) (registerotpwait.Entry, bool, error)
	Delete(ctx context.Context, entry registerotpwait.Entry) error
	Claim(ctx context.Context, jobID string, ttl time.Duration) (bool, error)
	ReleaseClaim(ctx context.Context, jobID string) error
}
