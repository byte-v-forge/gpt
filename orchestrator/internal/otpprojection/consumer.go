package otpprojection

import (
	"context"
	"log"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventcatalog"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	wav1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/wa/v1"
	"google.golang.org/protobuf/proto"

	"orchestrator/internal/smsotp"
)

type MailboxEmailProjector interface {
	ProjectMailboxEmail(context.Context, *mailboxv1.MailboxEmailReceivedEvent) error
}

type ConsumerSpec struct {
	Label   string
	Binding eventcatalog.ConsumerBinding
	Run     func(context.Context, eventbus.Consumer) error
}

type projectionConfig[T proto.Message] struct {
	Source   string
	Name     string
	Expected eventbus.ExpectedMessage
	New      func() T
	Project  func(context.Context, T) error
}

type projectionConsumer[T proto.Message] struct {
	cfg projectionConfig[T]
}

func runSMSCodeConsumer(ctx context.Context, consumer eventbus.Consumer, store *Store, smsResolver smsotp.Resolver) error {
	return runProjectionConsumer(ctx, consumer, projectionConfig[*smsv1.SmsCodeReceivedEvent]{
		Source:   SourceSMS,
		Name:     SourceSMS + " otp projection events",
		Expected: eventcatalog.SMSCodeReceived.ExpectedMessage(),
		New:      func() *smsv1.SmsCodeReceivedEvent { return &smsv1.SmsCodeReceivedEvent{} },
		Project: func(ctx context.Context, event *smsv1.SmsCodeReceivedEvent) error {
			return store.RecordSMSCode(ctx, event, smsResolver)
		},
	})
}

func runMailboxEmailConsumer(ctx context.Context, consumer eventbus.Consumer, store *Store, projector MailboxEmailProjector) error {
	return runProjectionConsumer(ctx, consumer, projectionConfig[*mailboxv1.MailboxEmailReceivedEvent]{
		Source:   SourceMailbox,
		Name:     SourceMailbox + " otp projection events",
		Expected: eventcatalog.MailboxEmailReceived.ExpectedMessage(),
		New:      func() *mailboxv1.MailboxEmailReceivedEvent { return &mailboxv1.MailboxEmailReceivedEvent{} },
		Project: func(ctx context.Context, event *mailboxv1.MailboxEmailReceivedEvent) error {
			if err := store.RecordMailboxEmail(ctx, event); err != nil {
				return err
			}
			if projector != nil {
				return projector.ProjectMailboxEmail(ctx, event)
			}
			return nil
		},
	})
}

func runWAOTPConsumer(ctx context.Context, consumer eventbus.Consumer, store *Store) error {
	return runProjectionConsumer(ctx, consumer, projectionConfig[*wav1.WaOtpReceivedEvent]{
		Source:   SourceWA,
		Name:     SourceWA + " otp projection events",
		Expected: eventcatalog.WAOTPReceived.ExpectedMessage(),
		New:      func() *wav1.WaOtpReceivedEvent { return &wav1.WaOtpReceivedEvent{} },
		Project: func(ctx context.Context, event *wav1.WaOtpReceivedEvent) error {
			return store.RecordWAOTP(ctx, event)
		},
	})
}

func ConsumerSpecs(store *Store, mailboxProjector MailboxEmailProjector, smsResolver smsotp.Resolver) []ConsumerSpec {
	return []ConsumerSpec{
		{
			Label:   "SMS OTP",
			Binding: eventcatalog.SMSCodeReceived.ConsumerBinding("gpt-otp-sms-code"),
			Run: func(ctx context.Context, consumer eventbus.Consumer) error {
				return runSMSCodeConsumer(ctx, consumer, store, smsResolver)
			},
		},
		{
			Label:   "mailbox OTP",
			Binding: eventcatalog.MailboxEmailReceived.ConsumerBinding("gpt-otp-mailbox-email"),
			Run: func(ctx context.Context, consumer eventbus.Consumer) error {
				return runMailboxEmailConsumer(ctx, consumer, store, mailboxProjector)
			},
		},
		{
			Label:   "WA OTP",
			Binding: eventcatalog.WAOTPReceived.ConsumerBinding("gpt-otp-wa-otp"),
			Run: func(ctx context.Context, consumer eventbus.Consumer) error {
				return runWAOTPConsumer(ctx, consumer, store)
			},
		},
	}
}

func runProjectionConsumer[T proto.Message](ctx context.Context, consumer eventbus.Consumer, cfg projectionConfig[T]) error {
	worker := &projectionConsumer[T]{cfg: cfg}
	return eventbus.RunTypedConsumerWorker(ctx, eventbus.TypedConsumerWorkerConfig[T]{
		Name:           cfg.Name,
		Consumer:       consumer,
		Expected:       cfg.Expected,
		NewMessage:     cfg.New,
		Handler:        worker.handle,
		MalformedLabel: "terminate malformed otp projection event",
		Logf:           logConsumer,
	})
}

func (c *projectionConsumer[T]) handle(ctx context.Context, event T, message eventbus.ReceivedMessage) eventbus.HandlerResult {
	if c.cfg.Project == nil {
		return eventbus.AckResult("ack otp projection event")
	}
	if err := c.cfg.Project(ctx, event); err != nil {
		log.Printf("[orchestrator] record %s otp projection failed event_id=%s: %v", c.cfg.Source, eventbus.EventID(message), err)
		return eventbus.NakResult(0, "retry otp projection event")
	}
	return eventbus.AckResult("ack otp projection event")
}

func logConsumer(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
