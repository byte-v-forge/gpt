package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventcatalog"
	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	"github.com/byte-v-forge/common-lib/grpcclient"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/hotstreamnats"
	"github.com/byte-v-forge/common-lib/natseventbus"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/accountmail"
	"orchestrator/internal/gopayotp"
	"orchestrator/internal/gptsettings"
	"orchestrator/internal/jobevents"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobqueue"
	"orchestrator/internal/mailboxevents"
	"orchestrator/internal/otpprojection"
	"orchestrator/internal/registerotpwait"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type orchestratorDependencies struct {
	db                       *gorm.DB
	jobStore                 *jobprojection.Store
	jobEvents                *jobevents.Store
	fingerprints             *accountfingerprint.Store
	gptSettings              *gptsettings.Store
	otpProjection            *otpprojection.Store
	registerProtocolOTPWaits *registerotpwait.Store
	mailboxState             *accountmail.Projector
	secrets                  runtimesecrets.Store
	platformBus              *natseventbus.Bus
	hotStream                hotstream.Bus

	accountClient           pb.GPTAccountServiceClient
	browserAutomationClient browserautomationv1.BrowserAutomationServiceClient
	paymentClient           pb.PaymentServiceClient
	gopayClient             pb.GopayAppServiceClient
	smsClient               smsv1.SmsOrderServiceClient
	smsCatalogClient        smsv1.SmsCatalogServiceClient
	mailboxPollRequester    *mailboxevents.Requester
	otpRelay                gopayotp.Relay

	closers []func() error
}

func newOrchestratorDependencies(ctx context.Context, cfg orchestratorConfig) (*orchestratorDependencies, error) {
	deps := &orchestratorDependencies{}
	runtimeSecretClient, err := newRequiredRedisClient(ctx, cfg.RuntimeSecretRedisURL, "GPT_RUNTIME_SECRET_REDIS_URL is required for GPT runtime secrets")
	if err != nil {
		return nil, err
	}
	deps.addCloser(runtimeSecretClient.Close)
	otpRelayClient, err := newRequiredRedisClient(ctx, cfg.OTPRelayRedisURL, "GPT_OTP_RELAY_REDIS_URL is required for GPT OTP relay")
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(otpRelayClient.Close)
	deps.otpRelay = newGoPayOTPRelay(otpRelayClient, cfg)

	browserAutomationConn, err := newGRPCClientConn("browser-automation service", cfg.BrowserAutomationAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(browserAutomationConn.Close)

	paymentConn, err := newGRPCClientConn("payment service", cfg.PaymentAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(paymentConn.Close)

	gopayConn, err := newGRPCClientConn(
		"GoPay app channel",
		cfg.GoPayAppAddr,
		grpc.WithDefaultServiceConfig(gopayAppGRPCRetryServiceConfig()),
	)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(gopayConn.Close)

	smsConn, err := newGRPCClientConn("sms service", cfg.SmsAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(smsConn.Close)

	gptAccountConn, err := newGRPCClientConn("GPT account service", cfg.GPTAccountAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(gptAccountConn.Close)

	database := db.InitDB()
	deps.db = database
	otpStore, err := otpprojection.NewStore(otpRelayClient, "byte-v-forge:gpt:otp-projection", 5*time.Minute)
	if err != nil {
		deps.Close()
		return nil, fmt.Errorf("initialize otp projection: %w", err)
	}
	deps.otpProjection = otpStore
	registerProtocolOTPWaits, err := registerotpwait.NewStore(runtimeSecretClient, "byte-v-forge:gpt:register-protocol-otp-wait")
	if err != nil {
		deps.Close()
		return nil, fmt.Errorf("initialize register protocol otp wait store: %w", err)
	}
	deps.registerProtocolOTPWaits = registerProtocolOTPWaits
	platformEventBus, closePlatformEventBus, err := newPlatformEventBus(ctx, cfg)
	if err != nil {
		deps.Close()
		return nil, fmt.Errorf("initialize GPT platform event bus: %w", err)
	}
	deps.addCloser(func() error { closePlatformEventBus(); return nil })
	deps.platformBus = platformEventBus
	hotStream, closeHotStream, err := newHotStreamBus(ctx, cfg)
	if err != nil {
		deps.Close()
		return nil, fmt.Errorf("initialize GPT hotstream: %w", err)
	}
	deps.addCloser(func() error { closeHotStream(); return nil })
	deps.hotStream = hotStream
	deps.jobEvents = jobevents.NewStore(database, db.DSN()).WithHotStream(deps.hotStream)
	deps.addCloser(deps.jobEvents.Close)
	deps.jobStore = jobprojection.NewStore(database).
		WithPublisher(deps.jobEvents).
		WithActionDispatcher(jobqueue.NewOutboxDispatcher())
	deps.fingerprints = accountfingerprint.NewStore(database)
	deps.gptSettings = gptsettings.NewStore(database)
	deps.mailboxPollRequester = mailboxevents.NewRequester(database)
	secrets, err := newRuntimeSecretStore(runtimeSecretClient, cfg)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.secrets = secrets
	deps.accountClient = pb.NewGPTAccountServiceClient(gptAccountConn)
	deps.mailboxState = accountmail.NewProjector(deps.accountClient).WithHotStream(deps.hotStream)
	deps.browserAutomationClient = browserautomationv1.NewBrowserAutomationServiceClient(browserAutomationConn)
	deps.paymentClient = pb.NewPaymentServiceClient(paymentConn)
	deps.gopayClient = pb.NewGopayAppServiceClient(gopayConn)
	deps.smsClient = smsv1.NewSmsOrderServiceClient(smsConn)
	deps.smsCatalogClient = smsv1.NewSmsCatalogServiceClient(smsConn)

	return deps, nil
}

func newPlatformEventBus(ctx context.Context, cfg orchestratorConfig) (*natseventbus.Bus, func(), error) {
	if strings.TrimSpace(cfg.PlatformNATSURL) == "" {
		return nil, nil, fmt.Errorf("PLATFORM_NATS_URL is required for GPT platform events")
	}
	bus, err := natseventbus.Connect(natseventbus.Config{
		URL:        cfg.PlatformNATSURL,
		ClientName: "gpt-service",
	})
	if err != nil {
		return nil, nil, err
	}
	return bus, bus.Close, nil
}

func newHotStreamBus(ctx context.Context, cfg orchestratorConfig) (hotstream.Bus, func(), error) {
	if strings.TrimSpace(cfg.PlatformNATSURL) == "" {
		return nil, nil, fmt.Errorf("PLATFORM_NATS_URL is required for GPT hotstream")
	}
	bus, err := hotstreamnats.Connect(ctx, hotstreamnats.Config{
		URL:        cfg.PlatformNATSURL,
		ClientName: "gpt-service",
		Subject:    hotstream.ServiceStateSubject("gpt"),
	})
	if err != nil {
		return nil, nil, err
	}
	return bus, bus.Close, nil
}

func startOTPProjectionConsumers(ctx context.Context, bus *natseventbus.Bus, cfg orchestratorConfig, store *otpprojection.Store, mailboxProjector otpprojection.MailboxEmailProjector) error {
	smsConsumer, err := bus.PullWorkerConsumer(cfg.EventStreamName, eventcatalog.SMSCodeReceived.Subject, otpprojection.SMSCodeConsumerDurable, 10, 30*time.Second)
	if err != nil {
		return fmt.Errorf("initialize GPT SMS OTP projection consumer: %w", err)
	}
	mailboxConsumer, err := bus.PullWorkerConsumer(cfg.EventStreamName, eventcatalog.MailboxEmailReceived.Subject, otpprojection.MailboxEmailConsumerDurable, 10, 30*time.Second)
	if err != nil {
		return fmt.Errorf("initialize GPT mailbox OTP projection consumer: %w", err)
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return otpprojection.RunSMSCodeConsumer(groupCtx, smsConsumer, store) })
	group.Go(func() error {
		return otpprojection.RunMailboxEmailConsumer(groupCtx, mailboxConsumer, store, mailboxProjector)
	})
	return group.Wait()
}

func newRequiredRedisClient(ctx context.Context, redisURL string, requiredMessage string) (*redis.Client, error) {
	client, err := redisx.NewRequiredClient(ctx, redisURL, requiredMessage)
	if err != nil {
		return nil, fmt.Errorf("initialize redis client: %w", err)
	}
	return client, nil
}

func newGoPayOTPRelay(client redis.Cmdable, cfg orchestratorConfig) gopayotp.Relay {
	return gopayotp.NewRedisRelay(client, cfg.GoPayOTPRelayKeyPrefix, cfg.GoPayOTPWebhookTTL, cfg.GoPayOTPWebhookMaxItems)
}

func newRuntimeSecretStore(client redis.Cmdable, cfg orchestratorConfig) (runtimesecrets.Store, error) {
	if client == nil {
		return nil, fmt.Errorf("GPT_RUNTIME_SECRET_REDIS_URL is required for GPT runtime secrets")
	}
	return redisx.NewStringStore(client, cfg.RuntimeSecretKeyPrefix, cfg.RuntimeSecretTTL), nil
}

func newGRPCClientConn(name string, addr string, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	conn, err := grpcclient.NewInsecure(addr, extraOpts...)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", name, err)
	}
	return conn, nil
}

func gopayAppGRPCRetryServiceConfig() string {
	return `{
		"methodConfig": [{
			"name": [{"service": "gopay_app.GopayAppService"}],
			"retryPolicy": {
				"MaxAttempts": 3,
				"InitialBackoff": "0.3s",
				"MaxBackoff": "2s",
				"BackoffMultiplier": 2,
				"RetryableStatusCodes": ["UNAVAILABLE"]
			}
		}]
	}`
}

func (d *orchestratorDependencies) addCloser(closeFn func() error) {
	d.closers = append(d.closers, closeFn)
}

func (d *orchestratorDependencies) Close() {
	var closeErr error
	for i := len(d.closers) - 1; i >= 0; i-- {
		if err := d.closers[i](); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	if closeErr != nil {
		log.Printf("Orchestrator dependency close failed: %v", closeErr)
	}
}
