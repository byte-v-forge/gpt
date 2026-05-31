package api

import (
	"context"
	"time"

	"orchestrator/db"
	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/accountproxyusage"
	"orchestrator/internal/activities"
	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/mailboxevents"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type Config struct {
	JobStore             *jobprojection.Store
	RuntimeSecrets       runtimesecrets.Store
	Fingerprints         *accountfingerprint.Store
	AccountProxyUsages   accountProxyUsageRecorder
	GPTSettings          GPTSettingsReader
	Activities           *activities.Server
	AccountClient        pb.GPTAccountServiceClient
	PaymentClient        pb.PaymentServiceClient
	MailboxPollRequester *mailboxevents.Requester
	OTPProjection        OTPProjection
	ChannelOTPWaits      channelOTPWaitStore
}

type channelOTPWaitStore interface {
	Register(context.Context, channelotpwait.Entry, time.Duration) error
	Pending(context.Context, string, string, int64) ([]channelotpwait.Entry, error)
	Get(context.Context, string) (channelotpwait.Entry, bool, error)
	Delete(context.Context, channelotpwait.Entry) error
	Claim(context.Context, string, time.Duration) (bool, error)
	ReleaseClaim(context.Context, string) error
}

type Server struct {
	pb.UnimplementedAccountWorkflowServiceServer
	pb.UnimplementedOTPServiceServer
	pb.UnimplementedJobServiceServer

	jobStore             *jobprojection.Store
	runtimeSecrets       runtimesecrets.Store
	fingerprints         *accountfingerprint.Store
	accountProxyUsages   accountProxyUsageRecorder
	gptSettings          GPTSettingsReader
	activities           *activities.Server
	accountClient        pb.GPTAccountServiceClient
	paymentClient        pb.PaymentServiceClient
	mailboxPollRequester *mailboxevents.Requester
	otpProjection        OTPProjection
	channelOTPWaits      channelOTPWaitStore
}

type accountProxyUsageRecorder interface {
	Record(context.Context, accountproxyusage.RecordInput) error
}

type GPTSettingsReader interface {
	Get(ctx context.Context) (*pb.GPTSettings, error)
}

func NewServer(cfg Config) *Server {
	server := &Server{
		jobStore:             cfg.JobStore,
		runtimeSecrets:       cfg.RuntimeSecrets,
		fingerprints:         cfg.Fingerprints,
		accountProxyUsages:   cfg.AccountProxyUsages,
		gptSettings:          cfg.GPTSettings,
		activities:           cfg.Activities,
		accountClient:        cfg.AccountClient,
		paymentClient:        cfg.PaymentClient,
		mailboxPollRequester: cfg.MailboxPollRequester,
		otpProjection:        cfg.OTPProjection,
		channelOTPWaits:      cfg.ChannelOTPWaits,
	}
	return server
}

func (s *Server) setJobParams(ctx context.Context, jobID string, params map[string]string) error {
	return s.jobStore.SetParams(ctx, jobID, params)
}

func (s *Server) getJob(ctx context.Context, jobID string) (*db.Job, error) {
	return s.jobStore.Get(ctx, jobID)
}
