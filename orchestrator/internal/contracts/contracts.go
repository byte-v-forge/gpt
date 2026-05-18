package contracts

import "strings"

const (
	ActionRegister            = "REGISTER"
	ActionActivate            = "ACTIVATE"
	ActionAutopay             = "AUTOPAY"
	ActionGoPayApp            = "GOPAY_APP"
	ActionGoPayPayment        = "GOPAY_PAYMENT"
	ActionGoPayPaymentRebind  = "GOPAY_PAYMENT_REBIND"
	ActionProbeAccount        = "PROBE_ACCOUNT"
	ActionLoginSession        = "LOGIN_SESSION"
	ActionRegisterAndActivate = "REGISTER_AND_ACTIVATE"
	ActionRegisterMailbox     = "REGISTER_MAILBOX"
	ActionMailboxOAuth        = "MAILBOX_OAUTH"
)

const (
	TaskQueueDefault = "nb-register-orchestrator"

	CreateJobActivityName                        = "CreateJobActivity"
	StartJobStepActivityName                     = "StartJobStepActivity"
	CompleteJobStepActivityName                  = "CompleteJobStepActivity"
	EnsureAccountActivityName                    = "EnsureAccountActivity"
	ResolveAccountActivityName                   = "ResolveAccountFromJobActivity"
	BrowserAuthStartActivityName                 = "BrowserAuthStartActivity"
	BrowserAuthWaitActivityName                  = "BrowserAuthWaitActivity"
	BrowserAuthCompleteActivityName              = "BrowserAuthCompleteActivity"
	BrowserAuthCancelActivityName                = "BrowserAuthCancelActivity"
	WaitOTPActivityName                          = "OTPWaitActivity"
	FetchManualOTPActivityName                   = "FetchManualOTPActivity"
	EnsureLogonActivityName                      = "EnsureLogonActivity"
	GoPayPaymentPrepareActivityName              = "GoPayPaymentPrepareActivity"
	GoPayPaymentStartActivityName                = "GoPayPaymentStartActivity"
	GoPayPaymentOTPResendActivityName            = "GoPayPaymentOTPResendActivity"
	GoPayPaymentCompleteActivityName             = "GoPayPaymentCompleteActivity"
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
	GoPayAppDiscardSignupPhoneActivityName       = "GoPayAppDiscardSignupPhoneActivity"
	GoPayAppAddBalanceActivityName               = "GoPayAppAddBalanceActivity"
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
	RegisterMailboxActivityName                  = "RegisterMailboxAtomicActivity"
	MailboxOAuthActivityName                     = "MailboxOAuthAtomicActivity"
	PersistRegisteredActivityName                = "PersistRegisteredActivity"
	PersistActivatedActivityName                 = "PersistActivatedActivity"
	MarkJobFailedActivityName                    = "MarkJobFailedActivity"
	MarkJobSucceededActivityName                 = "MarkJobSucceededActivity"

	ManualOTPSignalName                = "manual_otp_available"
	ManualAddBalanceSignalName         = "manual_add_balance_confirmed"
	GoPayAddBalanceSelectionSignalName = "gopay_add_balance_selected"
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
	case ActionGoPayPaymentRebind:
		return "gopay-payment-rebind-" + jobID, true
	case ActionProbeAccount:
		return "probe-" + jobID, true
	case ActionLoginSession:
		return "login-session-" + jobID, true
	case ActionRegisterAndActivate:
		return "register-activate-" + jobID, true
	case ActionRegisterMailbox:
		return "register-mailbox-" + jobID, true
	case ActionMailboxOAuth:
		return "mailbox-oauth-" + jobID, true
	default:
		return "", false
	}
}

func ManualOTPWorkflowID(action string, jobID string) (string, bool) {
	switch strings.TrimSpace(action) {
	case ActionRegister, ActionActivate, ActionAutopay, ActionGoPayApp, ActionGoPayPayment, ActionGoPayPaymentRebind, ActionRegisterAndActivate, ActionLoginSession:
		return WorkflowID(action, jobID)
	default:
		return "", false
	}
}
