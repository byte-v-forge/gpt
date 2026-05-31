package app

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	activityServer := newActivityServer(cfg, deps)
	apiServer := api.NewServer(api.Config{
		JobStore:             deps.jobStore,
		RuntimeSecrets:       deps.secrets,
		Fingerprints:         deps.fingerprints,
		AccountProxyUsages:   deps.accountProxyUsage,
		GPTSettings:          deps.gptSettings,
		Activities:           activityServer,
		AccountClient:        deps.accountClient,
		PaymentClient:        deps.paymentClient,
		MailboxPollRequester: deps.mailboxPollRequester,
		OTPProjection:        deps.otpProjection,
		ChannelOTPWaits:      deps.channelOTPWaits,
	})

	dashboardServer, err := dashboard.Start(rootCtx, dashboard.Config{
		ListenAddr:         cfg.DashboardHTTPAddr,
		N8NWebhookBaseURL:  cfg.N8NWebhookBaseURL,
		N8NActions:         apiServer,
		AccountClient:      deps.accountClient,
		PaymentClient:      deps.paymentClient,
		RuntimeSecrets:     deps.secrets,
		Fingerprints:       deps.fingerprints,
		AccountProxyUsages: deps.accountProxyUsage,
		WorkflowAPI:        apiServer,
		Settings:           deps.gptSettings,
		HotStream:          deps.hotStream,
		ActionRegistry:     deps.actionRegistry,
	})
	if err != nil {
		log.Fatalf("failed to start GPT dashboard BFF: %v", err)
	}
	defer dashboardServer.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterAccountWorkflowServiceServer(grpcServer, apiServer)
	registerPrivateWorkflowServices(grpcServer, apiServer)
	pb.RegisterOTPServiceServer(grpcServer, apiServer)
	pb.RegisterJobServiceServer(grpcServer, apiServer)
	grpchealth.RegisterServing(grpcServer)

	group, groupCtx := errgroup.WithContext(rootCtx)
	group.Go(func() error { return jobqueue.RunOutboxWorker(groupCtx, deps.db, deps.platformBus) })
	group.Go(func() error {
		return startOTPProjectionConsumers(groupCtx, deps.platformBus, cfg, deps.otpProjection, deps.mailboxState)
	})
	for _, worker := range api.N8NChannelOTPResumeWorkerSpecs() {
		worker := worker
		group.Go(func() error { return runOTPResumeWorker(groupCtx, cfg, deps, apiServer, worker) })
	}
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

func runOTPResumeWorker(ctx context.Context, cfg orchestratorConfig, deps *orchestratorDependencies, apiServer *api.Server, worker api.N8NChannelOTPResumeWorkerSpec) error {
	consumer, err := deps.platformBus.PullWorkerConsumer(
		cfg.EventStreamName,
		worker.Subject,
		worker.Durable,
		10,
		30*time.Second,
	)
	if err != nil {
		return err
	}
	if worker.Run == nil {
		return errors.New("n8n channel otp resume worker runner is required")
	}
	return worker.Run(ctx, consumer, apiServer)
}
