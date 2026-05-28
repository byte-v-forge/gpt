package app

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/byte-v-forge/common-lib/eventcatalog"
	"github.com/byte-v-forge/common-lib/grpchealth"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"orchestrator/internal/api"
	"orchestrator/internal/dashboard"
	"orchestrator/internal/jobqueue"
	"orchestrator/pb"
)

func Run() {
	log.Println("Initializing GPT service API...")

	cfg := loadOrchestratorConfig()
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	deps, err := newOrchestratorDependencies(rootCtx, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize GPT service dependencies: %v", err)
	}
	defer deps.Close()
	otpWebhookServer, err := startGoPayOTPWebhookServer(cfg.GoPayOTPWebhookListenAddr, deps.otpRelay)
	if err != nil {
		log.Fatalf("Failed to start GoPay OTP webhook: %v", err)
	}
	if otpWebhookServer != nil {
		defer otpWebhookServer.Close()
	}

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	dashboardWorkflowConn, err := newGRPCClientConn("GPT dashboard workflow API", dashboardSelfTarget(cfg.ListenAddr))
	if err != nil {
		log.Fatalf("failed to connect GPT dashboard workflow API: %v", err)
	}
	defer dashboardWorkflowConn.Close()

	activityServer := newActivityServer(cfg, deps)
	apiServer := api.NewServer(api.Config{
		DB:                                   deps.db,
		JobStore:                             deps.jobStore,
		JobEvents:                            deps.jobEvents,
		RuntimeSecrets:                       deps.secrets,
		Fingerprints:                         deps.fingerprints,
		GPTSettings:                          deps.gptSettings,
		Activities:                           activityServer,
		AccountClient:                        deps.accountClient,
		PaymentClient:                        deps.paymentClient,
		MailboxPollRequester:                 deps.mailboxPollRequester,
		OTPProjection:                        deps.otpProjection,
		RegisterProtocolOTPWaits:             deps.registerProtocolOTPWaits,
		GoPayClient:                          deps.gopayClient,
		DefaultGoPayAddBalance:               defaultGoPayAddBalance(cfg),
		DefaultGoPayAddBalances:              defaultGoPayAddBalances(cfg),
		GoPayAddBalanceConfirmTimeoutSeconds: cfg.GoPayAddBalanceConfirmTimeoutSeconds,
	})

	dashboardServer, err := dashboard.Start(rootCtx, dashboard.Config{
		ListenAddr:                 cfg.DashboardHTTPAddr,
		N8NWebhookBaseURL:          cfg.N8NWebhookBaseURL,
		N8NProbeActions:            apiServer,
		N8NRegisterProtocolActions: apiServer,
		AccountClient:              deps.accountClient,
		PaymentClient:              deps.paymentClient,
		RuntimeSecrets:             deps.secrets,
		Fingerprints:               deps.fingerprints,
		DB:                         deps.db,
		Settings:                   deps.gptSettings,
		WorkflowConn:               dashboardWorkflowConn,
		HotStream:                  deps.hotStream,
	})
	if err != nil {
		log.Fatalf("failed to start GPT dashboard BFF: %v", err)
	}
	defer dashboardServer.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterAccountWorkflowServiceServer(grpcServer, apiServer)
	pb.RegisterPaymentWorkflowServiceServer(grpcServer, apiServer)
	pb.RegisterGoPayAppWorkflowServiceServer(grpcServer, apiServer)
	pb.RegisterOTPServiceServer(grpcServer, apiServer)
	pb.RegisterJobServiceServer(grpcServer, apiServer)
	grpchealth.RegisterServing(grpcServer)

	group, groupCtx := errgroup.WithContext(rootCtx)
	group.Go(func() error { return jobqueue.RunOutboxWorker(groupCtx, deps.db, deps.platformBus) })
	group.Go(func() error {
		return startOTPProjectionConsumers(groupCtx, deps.platformBus, cfg, deps.otpProjection, deps.mailboxState)
	})
	group.Go(func() error { return runRegisterProtocolOTPResumeWorker(groupCtx, cfg, deps, apiServer) })
	group.Go(func() error { return runJobActionWorker(groupCtx, cfg, deps, apiServer) })
	go func() {
		<-groupCtx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("GPT service gRPC API listening on %s", cfg.ListenAddr)
	group.Go(func() error {
		if err := grpcServer.Serve(lis); err != nil && groupCtx.Err() == nil {
			return err
		}
		return nil
	})
	if err := group.Wait(); err != nil {
		stop()
		log.Fatalf("GPT service failed: %v", err)
	}
}

func runJobActionWorker(ctx context.Context, cfg orchestratorConfig, deps *orchestratorDependencies, apiServer *api.Server) error {
	consumer, err := deps.platformBus.PullWorkerConsumer(cfg.EventStreamName, eventcatalog.GPTJobActionRequested.Subject, eventcatalog.GPTJobActionRequested.ConsumerDurable, 1, 2*time.Hour)
	if err != nil {
		return err
	}
	return api.RunJobActionWorker(ctx, consumer, apiServer)
}

func runRegisterProtocolOTPResumeWorker(ctx context.Context, cfg orchestratorConfig, deps *orchestratorDependencies, apiServer *api.Server) error {
	consumer, err := deps.platformBus.PullWorkerConsumer(
		cfg.EventStreamName,
		eventcatalog.MailboxEmailReceived.Subject,
		api.N8NRegisterProtocolOTPResumeConsumerDurable,
		10,
		30*time.Second,
	)
	if err != nil {
		return err
	}
	return api.RunN8NRegisterProtocolOTPResumeWorker(ctx, consumer, apiServer)
}

func dashboardSelfTarget(listenAddr string) string {
	addr := strings.TrimSpace(listenAddr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
