package mailboxevents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventcatalog"
	"github.com/byte-v-forge/common-lib/eventoutbox"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"gorm.io/gorm"

	"orchestrator/db"
)

const (
	sourceService = "gpt-service"
)

type Requester struct {
	db *gorm.DB
}

func NewRequester(database *gorm.DB) *Requester {
	return &Requester{db: database}
}

func (r *Requester) RequestMailboxEmailPoll(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64, timeout time.Duration, reason string) error {
	if r == nil || r.db == nil {
		return nil
	}
	email = emailx.Normalize(email)
	if email == "" {
		return nil
	}
	deadlineUnix := time.Now().Add(timeout).Unix()
	if timeout <= 0 {
		deadlineUnix = time.Now().Add(2 * time.Minute).Unix()
	}
	request := &mailboxv1.MailboxEmailPollRequest{
		EmailAddress:    email,
		SignalKind:      kind,
		IssuedAfterUnix: issuedAfterUnix,
		DeadlineUnix:    deadlineUnix,
		Reason:          strings.TrimSpace(reason),
	}
	eventID := eventbus.StableEventID("gpt-mailbox-email-poll-", email, kind.String(), fmt.Sprintf("%d", issuedAfterUnix), fmt.Sprintf("%d", deadlineUnix), request.GetReason())
	eventCtx := eventbus.NewEventContext(eventbus.EventContextConfig{
		EventID:       eventID,
		EventName:     eventcatalog.MailboxEmailPollRequested.EventName,
		EventVersion:  eventcatalog.MailboxEmailPollRequested.EventVersion,
		SourceService: sourceService,
		CorrelationID: email,
	})
	record, err := eventoutbox.NewRecordFor(
		eventcatalog.MailboxEmailPollRequested,
		request,
		eventCtx,
		eventbus.Attributes(
			"email_address", email,
			"signal_kind", kind.String(),
			"reason", request.GetReason(),
		),
	)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return eventoutbox.InsertRecordGORM(ctx, tx, db.PlatformEventOutboxTable, record, time.Now().Unix())
	})
}
