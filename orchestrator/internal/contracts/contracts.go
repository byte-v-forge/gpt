package contracts

import "strings"

const (
	ActionRegister                 = "REGISTER"
	ActionActivate                 = "ACTIVATE"
	ActionAutopay                  = "AUTOPAY"
	ActionGoPayApp                 = "GOPAY_APP"
	ActionGoPayPayment             = "GOPAY_PAYMENT"
	ActionGoPayQRISPaymentActivate = "GOPAY_QRIS_PAYMENT_ACTIVATE"
	ActionGoPayWAPayment           = "GOPAY_WA_PAYMENT"
	ActionGoPayPaymentRebind       = "GOPAY_PAYMENT_REBIND"
	ActionProbeAccount             = "PROBE_ACCOUNT"
	ActionLoginSession             = "LOGIN_SESSION"
	ActionCodexOAuth               = "CODEX_OAUTH"
	ActionCodexOAuthAddPhone       = "CODEX_OAUTH_ADD_PHONE"
	ActionCodexOAuthBatchAddPhone  = "CODEX_OAUTH_BATCH_ADD_PHONE"
	ActionRegisterAndActivate      = "REGISTER_AND_ACTIVATE"
)

const (
	TaskQueueDefault = "byte-v-forge-gpt-service"

	CreateJobActivityName                        = "CreateJobActivity"
	StartJobStepActivityName                     = "StartJobStepActivity"
	CompleteJobStepActivityName                  = "CompleteJobStepActivity"
	EnsureAccountActivityName                    = "EnsureAccountActivity"
	ResolveAccountActivityName                   = "ResolveAccountFromJobActivity"
	BrowserAuthStartActivityName                 = "BrowserAuthStartActivity"
	BrowserAuthWaitActivityName                  = "BrowserAuthWaitActivity"
	BrowserAuthResendOTPActivityName             = "BrowserAuthResendOTPActivity"
	BrowserAuthCompleteActivityName              = "BrowserAuthCompleteActivity"
	BrowserAuthCancelActivityName                = "BrowserAuthCancelActivity"
	CodexOAuthAcquirePhoneActivityName           = "CodexOAuthAcquirePhoneActivity"
	CodexOAuthStartBrowserActivityName           = "CodexOAuthStartBrowserActivity"
	CodexOAuthLoginBrowserActivityName           = "CodexOAuthLoginBrowserActivity"
	CodexOAuthCompleteBrowserActivityName        = "CodexOAuthCompleteBrowserActivity"
	CodexOAuthStopBrowserActivityName            = "CodexOAuthStopBrowserActivity"
	CodexOAuthReleasePhoneActivityName           = "CodexOAuthReleasePhoneActivity"
	WaitOTPActivityName                          = "OTPWaitActivity"
	FetchManualOTPActivityName                   = "FetchManualOTPActivity"
	EnsureLogonActivityName                      = "EnsureLogonActivity"
	GoPayPaymentPrepareActivityName              = "GoPayPaymentPrepareActivity"
	GoPayPaymentPrepareCheckoutActivityName      = "GoPayPaymentPrepareCheckoutActivity"
	GoPayPaymentPrepareRefreshActivityName       = "GoPayPaymentPrepareRefreshActivity"
	GoPayPaymentPrepareLinkActivityName          = "GoPayPaymentPrepareLinkActivity"
	GoPayPaymentStartActivityName                = "GoPayPaymentStartActivity"
	GoPayPaymentOTPResendActivityName            = "GoPayPaymentOTPResendActivity"
	GoPayPaymentCompleteActivityName             = "GoPayPaymentCompleteActivity"
	GoPayPaymentManualConfirmActivityName        = "GoPayPaymentManualConfirmActivity"
	GoPayPaymentCancelActivityName               = "GoPayPaymentCancelActivity"
	GoPayResolveWAPhoneActivityName              = "GoPayResolveWAPhoneActivity"
	GoPayAppLoadStateActivityName                = "GoPayAppLoadStateActivity"
	GoPayAppSaveStateActivityName                = "GoPayAppSaveStateActivity"
	GoPayAppDeleteStateActivityName              = "GoPayAppDeleteStateActivity"
	GoPayPaymentRebindSourceActivityName         = "GoPayPaymentRebindSourceActivity"
	GoPayAppOTPStartActivityName                 = "GoPayAppOTPStartActivity"
	GoPayAppOTPCompleteActivityName              = "GoPayAppOTPCompleteActivity"
	GoPayAppOTPRetryActivityName                 = "GoPayAppOTPRetryActivity"
	GoPayAppStatusActivityName                   = "GoPayAppStatusActivity"
	GoPayAppCreatePinStartActivityName           = "GoPayAppCreatePinStartActivity"
	GoPayAppCreatePinRetryActivityName           = "GoPayAppCreatePinRetryActivity"
	GoPayAppCreatePinCompleteActivityName        = "GoPayAppCreatePinCompleteActivity"
	GoPayAppAcquireSignupPhoneActivityName       = "GoPayAppAcquireSignupPhoneActivity"
	GoPayAppGenerateDeviceProxyActivityName      = "GoPayAppGenerateDeviceProxyActivity"
	GoPayAppCheckSignupPhoneActivityName         = "GoPayAppCheckSignupPhoneActivity"
	GoPayAppDiscardSignupPhoneActivityName       = "GoPayAppDiscardSignupPhoneActivity"
	GoPayAppAddBalanceActivityName               = "GoPayAppAddBalanceActivity"
	GoPayAppBalanceCheckActivityName             = "GoPayAppBalanceCheckActivity"
	GoPayAppChangePhoneGetNumberActivityName     = "GoPayAppChangePhoneGetNumberActivity"
	GoPayAppChangePhoneStartActivityName         = "GoPayAppChangePhoneStartActivity"
	GoPayAppChangePhoneRetryActivityName         = "GoPayAppChangePhoneRetryActivity"
	GoPayAppSMSCancelBeforeRotationActivityName  = "GoPayAppSMSCancelBeforeRotationActivity"
	GoPayAppSMSFinishActivityName                = "GoPayAppSMSFinishActivity"
	GoPayAppChangePhoneCompleteActivityName      = "GoPayAppChangePhoneCompleteActivity"
	GoPayAppDeactivateStartActivityName          = "GoPayAppDeactivateStartActivity"
	GoPayAppDeactivateCompleteActivityName       = "GoPayAppDeactivateCompleteActivity"
	GoPayAppSMSRequestAdditionalCodeActivityName = "GoPayAppSMSRequestAdditionalCodeActivity"
	ProbePlusTrialActivityName                   = "ProbePlusTrialAtomicActivity"
	ProbeTierActivityName                        = "ProbeTierAtomicActivity"
	PersistRegisteredActivityName                = "PersistRegisteredActivity"
	PersistActivatedActivityName                 = "PersistActivatedActivity"
	MarkJobFailedActivityName                    = "MarkJobFailedActivity"
	MarkJobSucceededActivityName                 = "MarkJobSucceededActivity"

	ManualOTPSignalName                = "manual_otp_available"
	OTPResendSignalName                = "otp_resend_requested"
	ManualAddBalanceSignalName         = "manual_add_balance_confirmed"
	GoPayAddBalanceSelectionSignalName = "gopay_add_balance_selected"
	ManualGoPayPaymentSignalName       = "manual_gopay_payment_confirmed"
	WorkflowProgressQueryName          = "progress"
)

func WorkflowID(action string, jobID string) (string, bool) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return "", false
	}
	switch strings.TrimSpace(action) {
	case ActionRegister:
		return "register-" + jobID, true
	case ActionActivate:
		return "activate-" + jobID, true
	case ActionAutopay:
		return "autopay-" + jobID, true
	case ActionGoPayApp:
		return "gopay-app-" + jobID, true
	case ActionGoPayPayment:
		return "gopay-payment-" + jobID, true
	case ActionGoPayQRISPaymentActivate:
		return "gopay-qris-payment-activate-" + jobID, true
	case ActionGoPayWAPayment:
		return "gopay-wa-payment-" + jobID, true
	case ActionGoPayPaymentRebind:
		return "gopay-payment-rebind-" + jobID, true
	case ActionProbeAccount:
		return "probe-" + jobID, true
	case ActionLoginSession:
		return "login-session-" + jobID, true
	case ActionCodexOAuth:
		return "codex-oauth-" + jobID, true
	case ActionCodexOAuthAddPhone:
		return "codex-oauth-add-phone-" + jobID, true
	case ActionCodexOAuthBatchAddPhone:
		return "codex-oauth-batch-add-phone-" + jobID, true
	case ActionRegisterAndActivate:
		return "register-activate-" + jobID, true
	default:
		return "", false
	}
}

func ManualOTPWorkflowID(action string, jobID string) (string, bool) {
	switch strings.TrimSpace(action) {
	case ActionRegister, ActionActivate, ActionAutopay, ActionGoPayApp, ActionGoPayPayment, ActionGoPayQRISPaymentActivate, ActionGoPayWAPayment, ActionGoPayPaymentRebind, ActionRegisterAndActivate, ActionLoginSession:
		return WorkflowID(action, jobID)
	default:
		return "", false
	}
}
