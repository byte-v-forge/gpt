package api

import (
	"context"
	"time"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
)

type OTPProjection interface {
	LatestSMSCode(ctx context.Context, activationID string, issuedAfterUnix int64) (string, bool, error)
	WaitMailboxSignal(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64, timeout time.Duration, interval time.Duration) (*mailboxv1.EmailInboxMessage, string, bool, error)
	LatestMailboxSignal(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64) (*mailboxv1.EmailInboxMessage, string, bool, error)
}
