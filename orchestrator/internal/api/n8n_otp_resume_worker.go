package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventcatalog"
	"google.golang.org/protobuf/proto"

	"orchestrator/internal/channelotpwait"
)

type N8NChannelOTPResumeWorkerSpec struct {
	Binding eventcatalog.ConsumerBinding
	Run     func(context.Context, eventbus.Consumer, *Server) error
}

type n8nChannelOTPResumeWorkerConfig[T proto.Message] struct {
	Name           string
	Label          string
	Expected       eventbus.ExpectedMessage
	MalformedLabel string
	RetryLabel     string
	AckLabel       string
	NewEvent       func() T
	Event          func(context.Context, *Server, T) (channelOTPEvent, error)
}

func newN8NChannelOTPResumeWorkerConfig[T proto.Message](channel string, definition eventcatalog.Definition, newEvent func() T, event func(context.Context, *Server, T) (channelOTPEvent, error)) n8nChannelOTPResumeWorkerConfig[T] {
	label := channelOTPLogLabel(channel)
	lowerLabel := strings.ToLower(label)
	return n8nChannelOTPResumeWorkerConfig[T]{
		Name:           "GPT " + label + " OTP resume events",
		Label:          label,
		Expected:       definition.ExpectedMessage(),
		MalformedLabel: "terminate malformed " + lowerLabel + " otp event",
		RetryLabel:     "retry " + label + " OTP resume",
		AckLabel:       "ack " + label + " OTP resume event",
		NewEvent:       newEvent,
		Event:          event,
	}
}

func startN8NChannelOTPResumeWorker[T proto.Message](ctx context.Context, consumer eventbus.Consumer, server *Server, cfg n8nChannelOTPResumeWorkerConfig[T]) error {
	worker := &n8nChannelOTPResumeWorker[T]{
		cfg:    normalizeN8NChannelOTPResumeWorkerConfig(cfg),
		client: &http.Client{Timeout: 20 * time.Second},
		server: server,
	}
	return eventbus.RunTypedConsumerWorker(ctx, eventbus.TypedConsumerWorkerConfig[T]{
		Name:           worker.cfg.Name,
		Consumer:       consumer,
		Expected:       worker.cfg.Expected,
		NewMessage:     worker.cfg.NewEvent,
		Handler:        worker.handle,
		MalformedLabel: worker.cfg.MalformedLabel,
		Logf:           logN8NOTPResumeWorker,
	})
}

func N8NChannelOTPResumeWorkerSpecs() []N8NChannelOTPResumeWorkerSpec {
	return []N8NChannelOTPResumeWorkerSpec{
		n8nChannelOTPResumeWorkerSpec(channelotpwait.ChannelEmail, eventcatalog.MailboxEmailReceived, func(ctx context.Context, consumer eventbus.Consumer, server *Server) error {
			return startN8NChannelOTPResumeWorker(ctx, consumer, server, emailN8NChannelOTPResumeWorkerConfig())
		}),
		n8nChannelOTPResumeWorkerSpec(channelotpwait.ChannelSMS, eventcatalog.SMSCodeReceived, func(ctx context.Context, consumer eventbus.Consumer, server *Server) error {
			return startN8NChannelOTPResumeWorker(ctx, consumer, server, smsN8NChannelOTPResumeWorkerConfig())
		}),
		n8nChannelOTPResumeWorkerSpec(channelotpwait.ChannelWA, eventcatalog.WAOTPReceived, func(ctx context.Context, consumer eventbus.Consumer, server *Server) error {
			return startN8NChannelOTPResumeWorker(ctx, consumer, server, waN8NChannelOTPResumeWorkerConfig())
		}),
	}
}

func n8nChannelOTPResumeWorkerSpec(channel string, definition eventcatalog.Definition, run func(context.Context, eventbus.Consumer, *Server) error) N8NChannelOTPResumeWorkerSpec {
	channel = channelotpwait.NormalizeChannel(channel)
	return N8NChannelOTPResumeWorkerSpec{
		Binding: definition.ConsumerBinding("gpt-" + channel + "-otp-resume"),
		Run:     run,
	}
}

func channelOTPLogLabel(channel string) string {
	switch channelotpwait.NormalizeChannel(channel) {
	case channelotpwait.ChannelEmail:
		return "email"
	case channelotpwait.ChannelSMS:
		return "SMS"
	case channelotpwait.ChannelWA:
		return "WA"
	default:
		return "unknown"
	}
}

type n8nChannelOTPResumeWorker[T proto.Message] struct {
	cfg    n8nChannelOTPResumeWorkerConfig[T]
	client *http.Client
	server *Server
}

func (w *n8nChannelOTPResumeWorker[T]) handle(ctx context.Context, event T, message eventbus.ReceivedMessage) eventbus.HandlerResult {
	count, err := w.resume(ctx, event)
	if err != nil {
		log.Printf("[orchestrator] resume %s OTP failed event_id=%s: %v", w.cfg.Label, eventbus.EventID(message), err)
		return eventbus.NakResult(15*time.Second, w.cfg.RetryLabel)
	}
	if count > 0 {
		log.Printf("[orchestrator] resumed %s OTP wait count=%d event_id=%s", w.cfg.Label, count, eventbus.EventID(message))
	}
	return eventbus.AckResult(w.cfg.AckLabel)
}

func (w *n8nChannelOTPResumeWorker[T]) resume(ctx context.Context, event T) (int, error) {
	if w == nil || w.server == nil || w.cfg.Event == nil {
		return 0, nil
	}
	otpEvent, err := w.cfg.Event(ctx, w.server, event)
	if err != nil {
		return 0, err
	}
	return w.server.resumePendingChannelOTP(ctx, w.client, otpEvent)
}

func normalizeN8NChannelOTPResumeWorkerConfig[T proto.Message](cfg n8nChannelOTPResumeWorkerConfig[T]) n8nChannelOTPResumeWorkerConfig[T] {
	cfg.Name = firstNonEmpty(strings.TrimSpace(cfg.Name), "GPT OTP resume events")
	cfg.Label = firstNonEmpty(strings.TrimSpace(cfg.Label), "unknown")
	cfg.MalformedLabel = firstNonEmpty(strings.TrimSpace(cfg.MalformedLabel), "terminate malformed otp event")
	cfg.RetryLabel = firstNonEmpty(strings.TrimSpace(cfg.RetryLabel), "retry OTP resume")
	cfg.AckLabel = firstNonEmpty(strings.TrimSpace(cfg.AckLabel), "ack OTP resume event")
	return cfg
}

func logN8NOTPResumeWorker(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
