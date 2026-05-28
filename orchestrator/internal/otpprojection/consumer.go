package otpprojection

import (
	"context"
	"errors"
	"log"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventcatalog"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
)

const (
	SMSCodeConsumerDurable      = "gpt-otp-sms-code"
	MailboxEmailConsumerDurable = "gpt-otp-mailbox-email"
)

var errMalformedEvent = errors.New("malformed otp projection event")

type Consumer struct {
	store            *Store
	kind             string
	mailboxProjector MailboxEmailProjector
}

type MailboxEmailProjector interface {
	ProjectMailboxEmail(context.Context, *mailboxv1.MailboxEmailReceivedEvent) error
}

func RunSMSCodeConsumer(ctx context.Context, consumer eventbus.Consumer, store *Store) error {
	worker := &Consumer{store: store, kind: SourceSMS}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     SourceSMS + " otp projection events",
		Consumer: consumer,
		Handler:  worker.handle,
		Logf:     logConsumer,
	})
}

func RunMailboxEmailConsumer(ctx context.Context, consumer eventbus.Consumer, store *Store, projector MailboxEmailProjector) error {
	worker := &Consumer{store: store, kind: SourceMailbox, mailboxProjector: projector}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     SourceMailbox + " otp projection events",
		Consumer: consumer,
		Handler:  worker.handle,
		Logf:     logConsumer,
	})
}

func SMSCodeSubject() string {
	return eventcatalog.SMSCodeReceived.Subject
}

func MailboxEmailSubject() string {
	return eventcatalog.MailboxEmailReceived.Subject
}

func (c *Consumer) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	var err error
	switch c.kind {
	case SourceSMS:
		err = c.recordSMS(ctx, message)
	case SourceMailbox:
		err = c.recordMailbox(ctx, message)
	}
	if err != nil {
		if errors.Is(err, errMalformedEvent) {
			eventbus.TermMessage(ctx, message, "terminate malformed otp projection event", logConsumer)
			return
		}
		log.Printf("[orchestrator] record %s otp projection failed event_id=%s: %v", c.kind, eventbus.EventID(message), err)
		eventbus.NakMessage(ctx, message, "retry otp projection event", logConsumer)
		return
	}
	eventbus.AckMessage(ctx, message, "ack otp projection event", logConsumer)
}

func (c *Consumer) recordSMS(ctx context.Context, message eventbus.ReceivedMessage) error {
	event := &smsv1.SmsCodeReceivedEvent{}
	if err := eventbus.UnmarshalPayload(message, event); err != nil {
		return errMalformedEvent
	}
	return c.store.RecordSMSCode(ctx, event)
}

func (c *Consumer) recordMailbox(ctx context.Context, message eventbus.ReceivedMessage) error {
	event := &mailboxv1.MailboxEmailReceivedEvent{}
	if err := eventbus.UnmarshalPayload(message, event); err != nil {
		return errMalformedEvent
	}
	if err := c.store.RecordMailboxEmail(ctx, event); err != nil {
		return err
	}
	if c.mailboxProjector != nil {
		return c.mailboxProjector.ProjectMailboxEmail(ctx, event)
	}
	return nil
}

func logConsumer(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
