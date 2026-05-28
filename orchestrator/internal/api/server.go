package api

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/activities"
	"orchestrator/internal/contracts"
	"orchestrator/internal/jobevents"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobstatus"
	"orchestrator/internal/mailboxevents"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type Config struct {
	DB                                   *gorm.DB
	JobStore                             *jobprojection.Store
	JobEvents                            *jobevents.Store
	RuntimeSecrets                       runtimesecrets.Store
	Fingerprints                         *accountfingerprint.Store
	GPTSettings                          GPTSettingsReader
	Activities                           *activities.Server
	AccountClient                        pb.GPTAccountServiceClient
	PaymentClient                        pb.PaymentServiceClient
	MailboxPollRequester                 *mailboxevents.Requester
	OTPProjection                        OTPProjection
	RegisterProtocolOTPWaits             registerProtocolOTPWaitStore
	GoPayClient                          pb.GopayAppServiceClient
	DefaultGoPayAddBalance               *pb.GoPayAddBalance
	DefaultGoPayAddBalances              map[string]*pb.GoPayAddBalance
	GoPayAddBalanceConfirmTimeoutSeconds int32
}

type Server struct {
	pb.UnimplementedAccountWorkflowServiceServer
	pb.UnimplementedPaymentWorkflowServiceServer
	pb.UnimplementedGoPayAppWorkflowServiceServer
	pb.UnimplementedOTPServiceServer
	pb.UnimplementedJobServiceServer

	db                                   *gorm.DB
	jobStore                             *jobprojection.Store
	jobEvents                            *jobevents.Store
	runtimeSecrets                       runtimesecrets.Store
	fingerprints                         *accountfingerprint.Store
	gptSettings                          GPTSettingsReader
	activities                           *activities.Server
	accountClient                        pb.GPTAccountServiceClient
	paymentClient                        pb.PaymentServiceClient
	mailboxPollRequester                 *mailboxevents.Requester
	otpProjection                        OTPProjection
	registerProtocolOTPWaits             registerProtocolOTPWaitStore
	gopayClient                          pb.GopayAppServiceClient
	defaultGoPayAddBalance               *pb.GoPayAddBalance
	defaultGoPayAddBalances              map[string]*pb.GoPayAddBalance
	goPayAddBalanceConfirmTimeoutSeconds int32
}

type GPTSettingsReader interface {
	Get(ctx context.Context) (*pb.GPTSettings, error)
}

const (
	actionRegister                 = contracts.ActionRegister
	actionActivate                 = contracts.ActionActivate
	actionAutopay                  = contracts.ActionAutopay
	actionGoPayApp                 = contracts.ActionGoPayApp
	actionGoPayPayment             = contracts.ActionGoPayPayment
	actionGoPayQRISPaymentActivate = contracts.ActionGoPayQRISPaymentActivate
	actionGoPayWAPayment           = contracts.ActionGoPayWAPayment
	actionGoPayPaymentRebind       = contracts.ActionGoPayPaymentRebind
	actionProbeAccount             = contracts.ActionProbeAccount
	actionLoginSession             = contracts.ActionLoginSession
	actionRegisterProtocol         = contracts.ActionRegisterProtocol
	actionLoginSessionProtocol     = contracts.ActionLoginSessionProtocol
	actionCodexOAuth               = contracts.ActionCodexOAuth
	actionCodexOAuthProtocol       = contracts.ActionCodexOAuthProtocol
	actionCodexOAuthAddPhone       = contracts.ActionCodexOAuthAddPhone
	actionCodexOAuthBatchAddPhone  = contracts.ActionCodexOAuthBatchAddPhone
	actionRegisterAndActivate      = contracts.ActionRegisterAndActivate

	statusRunning                       = jobstatus.Running
	statusSucceeded                     = jobstatus.Succeeded
	stepRegisterAccount                 = contracts.StepRegisterAccount
	stepRegisterAccountStart            = contracts.StepRegisterAccountStart
	stepRegisterAccountBrowser          = contracts.StepRegisterAccountBrowser
	stepProtocolUseProxy                = contracts.StepProtocolUseProxy
	stepDynamicIPCreateSession          = contracts.StepDynamicIPCreateSession
	stepDynamicIPExitGeo                = contracts.StepDynamicIPExitGeo
	stepDynamicIPPreflight              = contracts.StepDynamicIPPreflight
	stepRegisterAccountProtocol         = contracts.StepRegisterAccountProtocol
	stepRegisterAccountProtocolStart    = contracts.StepRegisterAccountProtocolStart
	stepRegisterAccountProtocolOTPWait  = contracts.StepRegisterAccountProtocolOTPWait
	stepRegisterAccountProtocolComplete = contracts.StepRegisterAccountProtocolComplete
	stepRegisterAccountOTPRequest       = contracts.StepRegisterAccountOTPRequest
	stepRegisterAccountOTPWait          = contracts.StepRegisterAccountOTPWait
	stepRegisterAccountComplete         = contracts.StepRegisterAccountComplete
	stepEnsureLogon                     = contracts.StepEnsureLogon
	stepGoPayAppLogin                   = contracts.StepGoPayAppLogin
	stepGoPayAppChangePhone             = contracts.StepGoPayAppChangePhone
	stepGoPayAppChangePhoneGetNumber    = contracts.StepGoPayAppChangePhoneGetNumber
	stepGoPayAppChangePhoneStart        = contracts.StepGoPayAppChangePhoneStart
	stepGoPayAppChangePhoneSMSWait      = contracts.StepGoPayAppChangePhoneSMSWait
	stepGoPayAppChangePhoneRetry        = contracts.StepGoPayAppChangePhoneRetry
	stepGoPayAppChangePhoneCancel       = contracts.StepGoPayAppChangePhoneCancel
	stepGoPayAppChangePhoneComplete     = contracts.StepGoPayAppChangePhoneComplete
	stepGoPayAppSignupPhone             = contracts.StepGoPayAppSignupPhone
	stepGoPayAppGenerateDeviceProxy     = contracts.StepGoPayAppGenerateDeviceProxy
	stepGoPayAppCheckPhone              = contracts.StepGoPayAppCheckPhone
	stepGoPayResolveWAPhone             = contracts.StepGoPayResolveWAPhone
	stepGoPayAppDeactivate              = contracts.StepGoPayAppDeactivate
	stepGoPayAppDeactivateStart         = contracts.StepGoPayAppDeactivateStart
	stepGoPayAppDeactivateSMSWait       = contracts.StepGoPayAppDeactivateSMSWait
	stepGoPayAppDeactivateSMSFinish     = contracts.StepGoPayAppDeactivateSMSFinish
	stepGoPayAppDeactivateComplete      = contracts.StepGoPayAppDeactivateComplete
	stepGoPayAppSignup                  = contracts.StepGoPayAppSignup
	stepGoPayAppSignupRetry             = contracts.StepGoPayAppSignupRetry
	stepGoPayAppSignupPhoneCancel       = contracts.StepGoPayAppSignupPhoneCancel
	stepGoPayAppStatus                  = contracts.StepGoPayAppStatus
	stepGoPayAppEnsurePINSetup          = contracts.StepGoPayAppEnsurePINSetup
	stepGoPayAppEnsureBalance           = contracts.StepGoPayAppEnsureBalance
	stepGoPayAppEnsureBalanceConfirm    = contracts.StepGoPayAppEnsureBalanceConfirm
	stepGoPayAppSMSFinish               = contracts.StepGoPayAppSMSFinish
	stepGoPayAppSMSRequestMore          = contracts.StepGoPayAppSMSRequestMore
	stepGoPayPaymentPrepare             = contracts.StepGoPayPaymentPrepare
	stepGoPayPaymentPrepareCheckout     = contracts.StepGoPayPaymentPrepareCheckout
	stepGoPayPaymentPrepareRefresh      = contracts.StepGoPayPaymentPrepareRefresh
	stepGoPayPaymentPrepareLink         = contracts.StepGoPayPaymentPrepareLink
	stepGoPayPayment                    = contracts.StepGoPayPayment
	stepProbePlusTrial                  = contracts.StepProbePlusTrial
	stepProbeTier                       = contracts.StepProbeTier
	stepLoginSession                    = contracts.StepLoginSession
	stepLoginSessionStart               = contracts.StepLoginSessionStart
	stepLoginSessionBrowser             = contracts.StepLoginSessionBrowser
	stepLoginSessionProtocol            = contracts.StepLoginSessionProtocol
	stepLoginSessionProtocolStart       = contracts.StepLoginSessionProtocolStart
	stepLoginSessionProtocolOTPWait     = contracts.StepLoginSessionProtocolOTPWait
	stepLoginSessionProtocolComplete    = contracts.StepLoginSessionProtocolComplete
	stepLoginSessionOTPRequest          = contracts.StepLoginSessionOTPRequest
	stepLoginSessionOTPWait             = contracts.StepLoginSessionOTPWait
	stepLoginSessionComplete            = contracts.StepLoginSessionComplete
	stepCodexOAuthAcquirePhone          = contracts.StepCodexOAuthAcquirePhone
	stepCodexOAuthProtocolStart         = contracts.StepCodexOAuthProtocolStart
	stepCodexOAuthProtocolDetect        = contracts.StepCodexOAuthProtocolDetect
	stepCodexOAuthProtocolEmail         = contracts.StepCodexOAuthProtocolEmail
	stepCodexOAuthProtocolPassword      = contracts.StepCodexOAuthProtocolPassword
	stepCodexOAuthProtocolEmailOTP      = contracts.StepCodexOAuthProtocolEmailOTP
	stepCodexOAuthProtocolAddPhone      = contracts.StepCodexOAuthProtocolAddPhone
	stepCodexOAuthProtocolComplete      = contracts.StepCodexOAuthProtocolComplete
	stepCodexOAuthBrowserStart          = contracts.StepCodexOAuthBrowserStart
	stepCodexOAuthBrowserDetect         = contracts.StepCodexOAuthBrowserDetect
	stepCodexOAuthBrowserEmail          = contracts.StepCodexOAuthBrowserEmail
	stepCodexOAuthBrowserPassword       = contracts.StepCodexOAuthBrowserPassword
	stepCodexOAuthBrowserEmailOTP       = contracts.StepCodexOAuthBrowserEmailOTP
	stepCodexOAuthBrowserAddPhone       = contracts.StepCodexOAuthBrowserAddPhone
	stepCodexOAuthBrowserComplete       = contracts.StepCodexOAuthBrowserComplete
	stepCodexOAuthReleasePhone          = contracts.StepCodexOAuthReleasePhone
	registrationOTPParam                = "registration_otp"
	paymentOTPParam                     = "payment_otp"
	manualAddBalanceConfirmParam        = contracts.ManualAddBalanceConfirmationParam
	goPayAddBalanceSelectionParam       = contracts.GoPayAddBalanceSelectionParam
	manualGoPayPaymentConfirmParam      = contracts.ManualGoPayPaymentConfirmationParam
	registrationOTPSubmit               = "registration_otp_submitted_at_unix"
	paymentOTPSubmit                    = "payment_otp_submitted_at_unix"
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
		runtimeSecrets:                       cfg.RuntimeSecrets,
		fingerprints:                         cfg.Fingerprints,
		gptSettings:                          cfg.GPTSettings,
		activities:                           cfg.Activities,
		accountClient:                        cfg.AccountClient,
		paymentClient:                        cfg.PaymentClient,
		mailboxPollRequester:                 cfg.MailboxPollRequester,
		otpProjection:                        cfg.OTPProjection,
		registerProtocolOTPWaits:             cfg.RegisterProtocolOTPWaits,
		gopayClient:                          cfg.GoPayClient,
		defaultGoPayAddBalance:               cfg.DefaultGoPayAddBalance,
		defaultGoPayAddBalances:              cloneGoPayAddBalanceMap(cfg.DefaultGoPayAddBalances),
		goPayAddBalanceConfirmTimeoutSeconds: cfg.GoPayAddBalanceConfirmTimeoutSeconds,
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
