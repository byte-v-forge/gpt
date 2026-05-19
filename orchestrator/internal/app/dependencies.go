package app

import (
	"errors"
	"fmt"
	"log"

	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	temporalclient "go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/internal/jobevents"
	"orchestrator/internal/jobprojection"
	"orchestrator/pb"
)

type orchestratorDependencies struct {
	db        *gorm.DB
	jobStore  *jobprojection.Store
	jobEvents *jobevents.Store
	temporal  temporalclient.Client

	accountClient pb.AccountDatabaseServiceClient
	browserClient pb.BrowserRegistrationClient
	paymentClient pb.PaymentServiceClient
	gopayClient   pb.GopayAppServiceClient
	smsClient     smsv1.SmsActivationServiceClient
	mailboxClient pb.MailboxServiceClient

	closers []func() error
}

func newOrchestratorDependencies(cfg orchestratorConfig) (*orchestratorDependencies, error) {
	deps := &orchestratorDependencies{}

	browserConn, err := newGRPCClientConn("browser service", cfg.BrowserAddr)
	if err != nil {
		return nil, err
	}
	deps.addCloser(browserConn.Close)

	paymentConn, err := newGRPCClientConn("payment service", cfg.PaymentAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(paymentConn.Close)

	gopayConn, err := newGRPCClientConn(
		"gopay-app service",
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

	accountDBConn, err := newGRPCClientConn("account-db service", cfg.AccountDBAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(accountDBConn.Close)

	mailboxConn, err := newGRPCClientConn("mailbox service", cfg.MailboxAddr)
	if err != nil {
		deps.Close()
		return nil, err
	}
	deps.addCloser(mailboxConn.Close)

	temporalClient, closeTemporal, err := newTemporalClient(cfg)
	if err != nil {
		deps.Close()
		return nil, fmt.Errorf("connect to Temporal: %w", err)
	}
	deps.temporal = temporalClient
	deps.addCloser(func() error {
		closeTemporal()
		return nil
	})

	database := db.InitDB()
	deps.db = database
	deps.jobEvents = jobevents.NewStore(database, db.DSN())
	deps.addCloser(deps.jobEvents.Close)
	deps.jobStore = jobprojection.NewStore(database).WithPublisher(deps.jobEvents)
	deps.accountClient = pb.NewAccountDatabaseServiceClient(accountDBConn)
	deps.browserClient = pb.NewBrowserRegistrationClient(browserConn)
	deps.paymentClient = pb.NewPaymentServiceClient(paymentConn)
	deps.gopayClient = pb.NewGopayAppServiceClient(gopayConn)
	deps.smsClient = smsv1.NewSmsActivationServiceClient(smsConn)
	deps.mailboxClient = pb.NewMailboxServiceClient(mailboxConn)

	return deps, nil
}

func newGRPCClientConn(name string, addr string, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	opts = append(opts, extraOpts...)
	conn, err := grpc.NewClient(addr, opts...)
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
