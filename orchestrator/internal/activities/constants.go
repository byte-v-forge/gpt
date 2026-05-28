package activities

import (
	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
)

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

	statusRunning           = jobstatus.Running
	statusSucceeded         = jobstatus.Succeeded
	statusFailedRecoverable = jobstatus.FailedRecoverable
	statusFailedRetryable   = jobstatus.FailedRetryable
	statusFailedFinal       = jobstatus.FailedFinal

	accountStatusRegistered        = "REGISTERED"
	accountStatusUnregistered      = "UNREGISTERED"
	accountStatusActivated         = "ACTIVATED"
	accountStatusUserAlreadyExists = "USER_ALREADY_EXISTS"

	emailStatusAvailable         = "AVAILABLE"
	emailStatusAssigned          = "ASSIGNED"
	emailStatusRegistered        = "REGISTERED"
	emailStatusOAuthPending      = "OAUTH_PENDING"
	emailStatusUserAlreadyExists = "USER_ALREADY_EXISTS"
	emailStatusRegistrationFail  = "REGISTRATION_FAILED"
	emailStatusAuthFailed        = "AUTH_FAILED"
	emailStatusNeedsManualVerify = "NEEDS_MANUAL_VERIFICATION"

	emailAuthStatusAuthorized        = "AUTHORIZED"
	emailAuthStatusOAuthPending      = "OAUTH_PENDING"
	emailAuthStatusAuthFailed        = "AUTH_FAILED"
	emailAuthStatusNeedsManualVerify = "NEEDS_MANUAL_VERIFICATION"

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

	registrationOTPParam            = "registration_otp"
	registrationOTPSubmittedAtParam = "registration_otp_submitted_at_unix"
	paymentOTPParam                 = "payment_otp"
	paymentOTPSubmittedAtParam      = "payment_otp_submitted_at_unix"
	goPayLocalSource                = "local"
	goPayAppStateKey                = goPayLocalSource
)
