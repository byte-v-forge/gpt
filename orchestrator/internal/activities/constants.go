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
	actionRegisterAndActivate      = contracts.ActionRegisterAndActivate

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

	stepRegisterAccount                 = "register_account"
	stepRegisterAccountStart            = "register_account_start"
	stepRegisterAccountBrowser          = "register_account_browser"
	stepRegisterAccountProtocol         = "register_account_protocol"
	stepRegisterAccountProtocolStart    = "register_account_protocol_start"
	stepRegisterAccountProtocolOTPWait  = "register_account_protocol_otp_wait"
	stepRegisterAccountProtocolComplete = "register_account_protocol_complete"
	stepRegisterAccountOTPRequest       = "register_account_otp_request"
	stepRegisterAccountOTPWait          = "register_account_otp_wait"
	stepRegisterAccountComplete         = "register_account_complete"
	stepEnsureLogon                     = "ensure_logon"
	stepGoPayAppLogin                   = "gopay_app_ensure_token_available"
	stepGoPayAppChangePhone             = "gopay_app_change_phone"
	stepGoPayAppChangePhoneGetNumber    = "gopay_app_change_phone_get_number"
	stepGoPayAppChangePhoneStart        = "gopay_app_change_phone_start"
	stepGoPayAppChangePhoneSMSWait      = "gopay_app_change_phone_sms_wait"
	stepGoPayAppChangePhoneRetry        = "gopay_app_change_phone_retry"
	stepGoPayAppChangePhoneCancel       = "gopay_app_change_phone_cancel"
	stepGoPayAppChangePhoneComplete     = "gopay_app_change_phone_complete"
	stepGoPayAppSignupPhone             = "gopay_app_signup_phone"
	stepGoPayAppGenerateDeviceProxy     = "gopay_app_generate_device_proxy"
	stepGoPayAppCheckPhone              = "gopay_app_check_phone"
	stepGoPayResolveWAPhone             = "gopay_resolve_wa_phone"
	stepGoPayAppDeactivate              = "gopay_app_deactivate"
	stepGoPayAppDeactivateStart         = "gopay_app_deactivate_start"
	stepGoPayAppDeactivateSMSWait       = "gopay_app_deactivate_sms_wait"
	stepGoPayAppDeactivateSMSFinish     = "gopay_app_deactivate_sms_finish"
	stepGoPayAppDeactivateComplete      = "gopay_app_deactivate_complete"
	stepGoPayAppSignup                  = "gopay_app_signup"
	stepGoPayAppSignupRetry             = "gopay_app_signup_retry"
	stepGoPayAppSignupPhoneCancel       = "gopay_app_signup_phone_cancel"
	stepGoPayAppStatus                  = "gopay_app_status"
	stepGoPayAppEnsurePINSetup          = "gopay_app_ensure_pin_setup"
	stepGoPayAppEnsureBalance           = "gopay_app_ensure_balance"
	stepGoPayAppEnsureBalanceConfirm    = "gopay_app_ensure_balance_confirm"
	stepGoPayAppSMSFinish               = "gopay_app_sms_finish"
	stepGoPayAppSMSRequestMore          = "gopay_app_sms_request_more"
	stepGoPayPaymentPrepare             = "gopay_payment_prepare"
	stepGoPayPaymentPrepareCheckout     = "gopay_payment_prepare_checkout"
	stepGoPayPaymentPrepareRefresh      = "gopay_payment_prepare_checkout_refresh"
	stepGoPayPaymentPrepareLink         = "gopay_payment_prepare_link"
	stepGoPayPayment                    = "gopay_payment"
	stepProbePlusTrial                  = "probe_plus_trial"
	stepProbeTier                       = "probe_tier"
	stepLoginSession                    = "login_session"
	stepLoginSessionStart               = "login_session_start"
	stepLoginSessionBrowser             = "login_session_browser"
	stepLoginSessionProtocol            = "login_session_protocol"
	stepLoginSessionProtocolStart       = "login_session_protocol_start"
	stepLoginSessionProtocolOTPWait     = "login_session_protocol_otp_wait"
	stepLoginSessionProtocolComplete    = "login_session_protocol_complete"
	stepLoginSessionOTPRequest          = "login_session_otp_request"
	stepLoginSessionOTPWait             = "login_session_otp_wait"
	stepLoginSessionComplete            = "login_session_complete"
	stepCodexOAuthAcquirePhone          = "codex_oauth_acquire_phone"
	stepCodexOAuthProtocolStart         = "codex_oauth_protocol_start"
	stepCodexOAuthProtocolDetect        = "codex_oauth_protocol_detect"
	stepCodexOAuthProtocolEmail         = "codex_oauth_protocol_email"
	stepCodexOAuthProtocolPassword      = "codex_oauth_protocol_password"
	stepCodexOAuthProtocolEmailOTP      = "codex_oauth_protocol_email_otp"
	stepCodexOAuthProtocolAddPhone      = "codex_oauth_protocol_add_phone"
	stepCodexOAuthProtocolComplete      = "codex_oauth_protocol_complete"
	stepCodexOAuthBrowserStart          = "codex_oauth_browser_start"
	stepCodexOAuthBrowserDetect         = "codex_oauth_browser_detect"
	stepCodexOAuthBrowserEmail          = "codex_oauth_browser_email"
	stepCodexOAuthBrowserPassword       = "codex_oauth_browser_password"
	stepCodexOAuthBrowserEmailOTP       = "codex_oauth_browser_email_otp"
	stepCodexOAuthBrowserAddPhone       = "codex_oauth_browser_add_phone"
	stepCodexOAuthBrowserComplete       = "codex_oauth_browser_complete"
	stepCodexOAuthReleasePhone          = "codex_oauth_release_phone"

	registrationOTPParam            = "registration_otp"
	registrationOTPSubmittedAtParam = "registration_otp_submitted_at_unix"
	paymentOTPParam                 = "payment_otp"
	paymentOTPSubmittedAtParam      = "payment_otp_submitted_at_unix"
	goPayLocalSource                = "local"
	goPayAppStateKey                = goPayLocalSource
)
