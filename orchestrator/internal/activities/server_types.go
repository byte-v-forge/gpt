package activities

import (
	"context"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	"gorm.io/gorm"
	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/actionregistry"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type OTPProjection interface {
	WaitSMSCode(ctx context.Context, activationID string, issuedAfterUnix int64, timeout time.Duration, interval time.Duration) (string, bool, error)
	WaitWACode(ctx context.Context, e164Number string, issuedAfterUnix int64, timeout time.Duration, interval time.Duration) (string, bool, error)
	WaitMailboxSignal(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64, timeout time.Duration, interval time.Duration) (*mailboxv1.EmailInboxMessage, string, bool, error)
}

type MailboxPollRequester interface {
	RequestMailboxEmailPoll(ctx context.Context, email string, kind mailboxv1.EmailSignalKind, issuedAfterUnix int64, timeout time.Duration, reason string) error
}

type GPTSettingsReader interface {
	Get(ctx context.Context) (*pb.GPTSettings, error)
}

type Config struct {
	DB                      *gorm.DB
	OTPProjection           OTPProjection
	JobStore                *jobprojection.Store
	RuntimeSecrets          runtimesecrets.Store
	Fingerprints            *accountfingerprint.Store
	AccountClient           pb.GPTAccountServiceClient
	BrowserAutomationClient browserautomationv1.BrowserAutomationServiceClient
	BrowserAuth             BrowserAuthConfig
	CodexOAuth              CodexOAuthConfig
	PaymentClient           pb.PaymentServiceClient
	SmsClient               smsv1.SmsOrderServiceClient
	SmsCatalogClient        smsv1.SmsCatalogServiceClient
	MailboxPollRequester    MailboxPollRequester
	GPTSettings             GPTSettingsReader
	EmailAllocator          AccountEmailAllocator
	ActionRegistry          *actionregistry.Registry
}

type Server struct {
	db                      *gorm.DB
	otpProjection           OTPProjection
	jobStore                *jobprojection.Store
	runtimeSecrets          runtimesecrets.Store
	fingerprints            *accountfingerprint.Store
	accountClient           pb.GPTAccountServiceClient
	browserAutomationClient browserautomationv1.BrowserAutomationServiceClient
	browserAuthConfig       BrowserAuthConfig
	codexOAuthConfig        CodexOAuthConfig
	paymentClient           pb.PaymentServiceClient
	smsClient               smsv1.SmsOrderServiceClient
	smsCatalogClient        smsv1.SmsCatalogServiceClient
	mailboxPollRequester    MailboxPollRequester
	gptSettings             GPTSettingsReader
	emailAllocator          AccountEmailAllocator
	actionRegistry          *actionregistry.Registry
}

func NewServer(cfg Config) *Server {
	actionRegistry := cfg.ActionRegistry
	if actionRegistry == nil {
		panic("activities: action registry is required")
	}
	return &Server{
		db:                      cfg.DB,
		otpProjection:           cfg.OTPProjection,
		jobStore:                cfg.JobStore,
		runtimeSecrets:          cfg.RuntimeSecrets,
		fingerprints:            cfg.Fingerprints,
		accountClient:           cfg.AccountClient,
		browserAutomationClient: cfg.BrowserAutomationClient,
		browserAuthConfig:       cfg.BrowserAuth.withDefaults(),
		codexOAuthConfig:        cfg.CodexOAuth.withDefaults(),
		paymentClient:           cfg.PaymentClient,
		smsClient:               cfg.SmsClient,
		smsCatalogClient:        cfg.SmsCatalogClient,
		mailboxPollRequester:    cfg.MailboxPollRequester,
		gptSettings:             cfg.GPTSettings,
		emailAllocator:          defaultAccountEmailAllocator(cfg.EmailAllocator, cfg.AccountClient),
		actionRegistry:          actionRegistry,
	}
}
