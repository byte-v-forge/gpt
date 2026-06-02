package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	"github.com/byte-v-forge/common-lib/grpcclient"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/hotstreamnats"
	"github.com/byte-v-forge/common-lib/natseventbus"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/accountmail"
	"orchestrator/internal/accountproxyusage"
	"orchestrator/internal/actionregistry"
	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/gptsettings"
	"orchestrator/internal/jobevents"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/mailboxevents"
	"orchestrator/internal/otpprojection"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type orchestratorDependencies struct {
	db                *gorm.DB
	jobStore          *jobprojection.Store
	jobEvents         *jobevents.Store
	fingerprints      *accountfingerprint.Store
	accountProxyUsage *accountproxyusage.Store
	gptSettings       *gptsettings.Store
	otpProjection     *otpprojection.Store
	channelOTPWaits   *channelotpwait.Store
	mailboxState      *accountmail.Projector
	secrets           runtimesecrets.Store
	platformBus       *natseventbus.Bus
	hotStream         hotstream.Bus
	actionRegistry    *actionregistry.Registry

	accountClient           pb.GPTAccountServiceClient
	browserAutomationClient browserautomationv1.BrowserAutomationServiceClient
	paymentClient           pb.PaymentServiceClient
	smsClient               smsv1.SmsOrderServiceClient
	smsCatalogClient        smsv1.SmsCatalogServiceClient
	mailboxPollRequester    *mailboxevents.Requester

	closers []func() error
}

func newOrchestratorDependencies(ctx context.Context, cfg orchestratorConfig, actionPlugins []gptplugin.Plugin) (*orchestratorDependencies, error) {
	deps := &orchestratorDependencies{actionRegistry: actionregistry.NewDefault(actionPlugins...)}
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
	if err := configurePrivateDependencies(ctx, cfg, deps, otpRelayClient); err != nil {
		deps.Close()
		return nil, err
	}

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
	channelOTPWaits, err := channelotpwait.NewStore(runtimeSecretClient, "byte-v-forge:gpt:channel-otp-wait", channelotpwait.Config{
		RequiredError:      "channel otp wait redis client is required",
		MissingErrorPrefix: "channel otp wait",
	})
	if err != nil {
		deps.Close()
		return nil, fmt.Errorf("initialize channel otp wait store: %w", err)
	}
	deps.channelOTPWaits = channelOTPWaits
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
	deps.jobStore = jobprojection.NewStore(database, deps.actionRegistry).
		WithPublisher(deps.jobEvents)
	deps.fingerprints = accountfingerprint.NewStore(database)
	deps.accountProxyUsage = accountproxyusage.NewStore(database)
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
	deps.smsClient = smsv1.NewSmsOrderServiceClient(smsConn)
	deps.smsCatalogClient = smsv1.NewSmsCatalogServiceClient(smsConn)

	return deps, nil
}

func (d *orchestratorDependencies) hasAnyAction(actionIDs ...string) bool {
	if d == nil || d.actionRegistry == nil {
		return false
	}
	for _, actionID := range actionIDs {
		if _, ok := d.actionRegistry.Action(actionID); ok {
			return true
		}
	}
	return false
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
	group, groupCtx := errgroup.WithContext(ctx)
	for _, spec := range otpprojection.ConsumerSpecs(store, mailboxProjector) {
		spec := spec
		consumer, err := bus.PullWorkerConsumer(cfg.EventStreamName, spec.Subject, spec.Durable, 10, 30*time.Second)
		if err != nil {
			return fmt.Errorf("initialize GPT %s projection consumer: %w", spec.Label, err)
		}
		group.Go(func() error { return spec.Run(groupCtx, consumer) })
	}
	return group.Wait()
}

func newRequiredRedisClient(ctx context.Context, redisURL string, requiredMessage string) (*redis.Client, error) {
	client, err := redisx.NewRequiredClient(ctx, redisURL, requiredMessage)
	if err != nil {
		return nil, fmt.Errorf("initialize redis client: %w", err)
	}
	return client, nil
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
