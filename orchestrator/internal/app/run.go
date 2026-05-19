package app

import (
	"log"
	"net"

	workflowruntime "github.com/byte-v-forge/workflow-runtime"
	"google.golang.org/grpc"
	"orchestrator/internal/api"
	"orchestrator/pb"
)

func Run() {
	log.Println("Initializing Orchestrator API...")

	cfg := loadOrchestratorConfig()
	deps, err := newOrchestratorDependencies(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize orchestrator dependencies: %v", err)
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

	activityServer := newActivityServer(cfg, deps)
	apiServer := api.NewServer(api.Config{
		DB:                                   deps.db,
		JobStore:                             deps.jobStore,
		JobEvents:                            deps.jobEvents,
		Temporal:                             deps.temporal,
		TaskQueue:                            cfg.Temporal.TaskQueue,
		AccountClient:                        deps.accountClient,
		GoPayClient:                          deps.gopayClient,
		DefaultGoPayAddBalance:               defaultGoPayAddBalance(cfg),
		DefaultGoPayAddBalances:              defaultGoPayAddBalances(cfg),
		GoPayAddBalanceConfirmTimeoutSeconds: cfg.GoPayAddBalanceConfirmTimeoutSeconds,
	})

	temporalWorker, err := workflowruntime.NewWorker(deps.temporal, temporalWorkerSpec(cfg.Temporal.TaskQueue, activityServer))
	if err != nil {
		log.Fatalf("Failed to create Temporal worker: %v", err)
	}
	if err := temporalWorker.Start(); err != nil {
		log.Fatalf("Temporal worker failed: %v", err)
	}
	defer temporalWorker.Stop()

	grpcServer := grpc.NewServer()
	pb.RegisterAccountWorkflowServiceServer(grpcServer, apiServer)
	pb.RegisterPaymentWorkflowServiceServer(grpcServer, apiServer)
	pb.RegisterGoPayAppWorkflowServiceServer(grpcServer, apiServer)
	pb.RegisterOTPServiceServer(grpcServer, apiServer)
	pb.RegisterJobServiceServer(grpcServer, apiServer)

	log.Printf("Orchestrator gRPC API listening on %s", cfg.ListenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
