package activities

import (
	"time"

	"gorm.io/gorm"
	"orchestrator/internal/jobprojection"
	"orchestrator/pb"
)

type Config struct {
	DB                                *gorm.DB
	JobStore                          *jobprojection.Store
	AccountClient                     pb.AccountDatabaseServiceClient
	BrowserClient                     pb.BrowserRegistrationClient
	PaymentClient                     pb.PaymentServiceClient
	GoPayClient                       pb.GopayAppServiceClient
	SmsClient                         pb.SmsServiceClient
	EmailClient                       pb.EmailServiceClient
	EmailAllocator                    AccountEmailAllocator
	MailboxRegisterClient             pb.MailboxRegistrationServiceClient
	OTPAddr                           string
	OTPTimeout                        int32
	RegistrationOTPTimeout            int32
	GoPayAppStepBodyLimit             int32
	GoPayAppLinkPaymentTimeout        time.Duration
	GoPayAppUnlinkTimeout             time.Duration
	ChangePhoneMaxFailures            int
	ChangePhoneDisabled               bool
	ChangePhoneOTPRetryAttempts       int
	ChangePhoneGetNumberRetryDelay    time.Duration
	ChangePhoneSMSCancelTimeout       time.Duration
	ChangePhoneSMSCancelRetryInterval time.Duration
}

type Server struct {
	db                                *gorm.DB
	jobStore                          *jobprojection.Store
	accountClient                     pb.AccountDatabaseServiceClient
	browserClient                     pb.BrowserRegistrationClient
	paymentClient                     pb.PaymentServiceClient
	gopayClient                       pb.GopayAppServiceClient
	smsClient                         pb.SmsServiceClient
	emailClient                       pb.EmailServiceClient
	emailAllocator                    AccountEmailAllocator
	mailboxRegisterClient             pb.MailboxRegistrationServiceClient
	otpAddr                           string
	otpTimeout                        int32
	regOTPTimeout                     int32
	gopayAppStepBodyLimit             int32
	gopayAppLinkPaymentTimeout        time.Duration
	gopayAppUnlinkTimeout             time.Duration
	changePhoneMaxFailures            int
	changePhoneDisabled               bool
	changePhoneOTPRetryAttempts       int
	changePhoneGetNumberRetryDelay    time.Duration
	changePhoneSMSCancelTimeout       time.Duration
	changePhoneSMSCancelRetryInterval time.Duration
}

func NewServer(cfg Config) *Server {
	return &Server{
		db:                                cfg.DB,
		jobStore:                          cfg.JobStore,
		accountClient:                     cfg.AccountClient,
		browserClient:                     cfg.BrowserClient,
		paymentClient:                     cfg.PaymentClient,
		gopayClient:                       cfg.GoPayClient,
		smsClient:                         cfg.SmsClient,
		emailClient:                       cfg.EmailClient,
		emailAllocator:                    defaultAccountEmailAllocator(cfg.EmailAllocator, cfg.AccountClient),
		mailboxRegisterClient:             cfg.MailboxRegisterClient,
		otpAddr:                           cfg.OTPAddr,
		otpTimeout:                        cfg.OTPTimeout,
		regOTPTimeout:                     cfg.RegistrationOTPTimeout,
		gopayAppStepBodyLimit:             cfg.GoPayAppStepBodyLimit,
		gopayAppLinkPaymentTimeout:        cfg.GoPayAppLinkPaymentTimeout,
		gopayAppUnlinkTimeout:             cfg.GoPayAppUnlinkTimeout,
		changePhoneMaxFailures:            cfg.ChangePhoneMaxFailures,
		changePhoneDisabled:               cfg.ChangePhoneDisabled,
		changePhoneOTPRetryAttempts:       cfg.ChangePhoneOTPRetryAttempts,
		changePhoneGetNumberRetryDelay:    cfg.ChangePhoneGetNumberRetryDelay,
		changePhoneSMSCancelTimeout:       cfg.ChangePhoneSMSCancelTimeout,
		changePhoneSMSCancelRetryInterval: cfg.ChangePhoneSMSCancelRetryInterval,
	}
}
