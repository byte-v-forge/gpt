package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	"gorm.io/gorm"
	"orchestrator/internal/gopayotp"
	"orchestrator/internal/jobprojection"
	"orchestrator/pb"
)

type Config struct {
	DB                             *gorm.DB
	JobStore                       *jobprojection.Store
	AccountClient                  pb.AccountDatabaseServiceClient
	BrowserAutomationClient        browserautomationv1.BrowserAutomationServiceClient
	BrowserAuth                    BrowserAuthConfig
	PaymentClient                  pb.PaymentServiceClient
	OTPRelay                       *gopayotp.Relay
	GoPayClient                    pb.GopayAppServiceClient
	SmsClient                      smsv1.SmsActivationServiceClient
	MailboxClient                  pb.MailboxServiceClient
	EmailAllocator                 AccountEmailAllocator
	OTPTimeout                     int32
	RegistrationOTPTimeout         int32
	GoPayAppStepBodyLimit          int32
	GoPayAppLinkPaymentTimeout     time.Duration
	GoPayAppUnlinkTimeout          time.Duration
	ChangePhoneMaxFailures         int
	ChangePhoneDisabled            bool
	ChangePhoneOTPRetryAttempts    int
	ChangePhoneGetNumberRetryDelay time.Duration
}

type Server struct {
	db                             *gorm.DB
	jobStore                       *jobprojection.Store
	accountClient                  pb.AccountDatabaseServiceClient
	browserAutomationClient        browserautomationv1.BrowserAutomationServiceClient
	browserAuthConfig              BrowserAuthConfig
	browserAuthFlows               *browserAuthFlowStore
	paymentClient                  pb.PaymentServiceClient
	otpRelay                       *gopayotp.Relay
	gopayClient                    pb.GopayAppServiceClient
	smsClient                      smsv1.SmsActivationServiceClient
	mailboxClient                  pb.MailboxServiceClient
	emailAllocator                 AccountEmailAllocator
	otpTimeout                     int32
	regOTPTimeout                  int32
	gopayAppStepBodyLimit          int32
	gopayAppLinkPaymentTimeout     time.Duration
	gopayAppUnlinkTimeout          time.Duration
	changePhoneMaxFailures         int
	changePhoneDisabled            bool
	changePhoneOTPRetryAttempts    int
	changePhoneGetNumberRetryDelay time.Duration
}

func NewServer(cfg Config) *Server {
	return &Server{
		db:                             cfg.DB,
		jobStore:                       cfg.JobStore,
		accountClient:                  cfg.AccountClient,
		browserAutomationClient:        cfg.BrowserAutomationClient,
		browserAuthConfig:              cfg.BrowserAuth.withDefaults(),
		browserAuthFlows:               newBrowserAuthFlowStore(),
		paymentClient:                  cfg.PaymentClient,
		otpRelay:                       cfg.OTPRelay,
		gopayClient:                    cfg.GoPayClient,
		smsClient:                      cfg.SmsClient,
		mailboxClient:                  cfg.MailboxClient,
		emailAllocator:                 defaultAccountEmailAllocator(cfg.EmailAllocator, cfg.AccountClient),
		otpTimeout:                     cfg.OTPTimeout,
		regOTPTimeout:                  cfg.RegistrationOTPTimeout,
		gopayAppStepBodyLimit:          cfg.GoPayAppStepBodyLimit,
		gopayAppLinkPaymentTimeout:     cfg.GoPayAppLinkPaymentTimeout,
		gopayAppUnlinkTimeout:          cfg.GoPayAppUnlinkTimeout,
		changePhoneMaxFailures:         cfg.ChangePhoneMaxFailures,
		changePhoneDisabled:            cfg.ChangePhoneDisabled,
		changePhoneOTPRetryAttempts:    cfg.ChangePhoneOTPRetryAttempts,
		changePhoneGetNumberRetryDelay: cfg.ChangePhoneGetNumberRetryDelay,
	}
}
