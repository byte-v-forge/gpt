package api

import (
	"context"
	"strings"

	temporalclient "go.temporal.io/sdk/client"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/internal/contracts"
	"orchestrator/internal/jobevents"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type Config struct {
	DB                                   *gorm.DB
	JobStore                             *jobprojection.Store
	JobEvents                            *jobevents.Store
	Temporal                             temporalclient.Client
	TaskQueue                            string
	AccountClient                        pb.AccountDatabaseServiceClient
	EmailClient                          pb.EmailServiceClient
	GoPayClient                          pb.GopayAppServiceClient
	DefaultGoPayAddBalance               *pb.GoPayAddBalance
	DefaultGoPayAddBalances              map[string]*pb.GoPayAddBalance
	GoPayAddBalanceConfirmTimeoutSeconds int32
	OutlookRegisterEnableOAuth2          bool
}

type Server struct {
	pb.UnimplementedAccountWorkflowServiceServer
	pb.UnimplementedPaymentWorkflowServiceServer
	pb.UnimplementedGoPayAppWorkflowServiceServer
	pb.UnimplementedMailboxWorkflowServiceServer
	pb.UnimplementedOTPServiceServer
	pb.UnimplementedJobServiceServer

	db                                   *gorm.DB
	jobStore                             *jobprojection.Store
	jobEvents                            *jobevents.Store
	temporal                             temporalclient.Client
	taskQueue                            string
	accountClient                        pb.AccountDatabaseServiceClient
	emailClient                          pb.EmailServiceClient
	gopayClient                          pb.GopayAppServiceClient
	defaultGoPayAddBalance               *pb.GoPayAddBalance
	defaultGoPayAddBalances              map[string]*pb.GoPayAddBalance
	goPayAddBalanceConfirmTimeoutSeconds int32
	outlookRegisterEnableOAuth2          bool
}

type ManualOTPSignal = pb.ManualOTPSignal
type ManualAddBalanceSignal = pb.ManualAddBalanceSignal

const (
	actionRegister            = contracts.ActionRegister
	actionActivate            = contracts.ActionActivate
	actionAutopay             = contracts.ActionAutopay
	actionGoPayApp            = contracts.ActionGoPayApp
	actionGoPayPayment        = contracts.ActionGoPayPayment
	actionGoPayPaymentRebind  = contracts.ActionGoPayPaymentRebind
	actionProbeAccount        = contracts.ActionProbeAccount
	actionLoginSession        = contracts.ActionLoginSession
	actionRegisterAndActivate = contracts.ActionRegisterAndActivate
	actionRegisterMailbox     = contracts.ActionRegisterMailbox
	actionMailboxOAuth        = contracts.ActionMailboxOAuth

	statusRunning   = jobstatus.Running
	statusSucceeded = jobstatus.Succeeded

	stepRegisterAccount        = "register_account"
	stepEnsureLogon            = "ensure_logon"
	stepGoPayAppAddBalance     = "gopay_app_add_balance"
	stepGoPayPayment           = "gopay_payment"
	registrationOTPParam       = "registration_otp"
	paymentOTPParam            = "payment_otp"
	manualOTPSignalName        = contracts.ManualOTPSignalName
	manualAddBalanceSignalName = contracts.ManualAddBalanceSignalName
	registrationOTPSubmit      = "registration_otp_submitted_at_unix"
	paymentOTPSubmit           = "payment_otp_submitted_at_unix"
)

const (
	registrationOTPSubmittedAtParam = registrationOTPSubmit
	paymentOTPSubmittedAtParam      = paymentOTPSubmit
)

func NewServer(cfg Config) *Server {
	return &Server{
		db:                                   cfg.DB,
		jobStore:                             cfg.JobStore,
		jobEvents:                            cfg.JobEvents,
		temporal:                             cfg.Temporal,
		taskQueue:                            cfg.TaskQueue,
		accountClient:                        cfg.AccountClient,
		emailClient:                          cfg.EmailClient,
		gopayClient:                          cfg.GoPayClient,
		defaultGoPayAddBalance:               cfg.DefaultGoPayAddBalance,
		defaultGoPayAddBalances:              cloneGoPayAddBalanceMap(cfg.DefaultGoPayAddBalances),
		goPayAddBalanceConfirmTimeoutSeconds: cfg.GoPayAddBalanceConfirmTimeoutSeconds,
		outlookRegisterEnableOAuth2:          cfg.OutlookRegisterEnableOAuth2,
	}
}

func (s *Server) workflowOptions(workflowID string) temporalclient.StartWorkflowOptions {
	return temporalclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.taskQueue,
	}
}

func (s *Server) setJobParams(ctx context.Context, jobID string, params map[string]string) error {
	return s.jobStore.SetParams(ctx, jobID, params)
}

func (s *Server) getJob(ctx context.Context, jobID string) (*db.Job, error) {
	return s.jobStore.Get(ctx, jobID)
}

func normalizeOTP(value string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "")
	return strings.TrimSpace(replacer.Replace(value))
}
