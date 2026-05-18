package workflows

import "orchestrator/internal/contracts"

const (
	taskQueueDefault = contracts.TaskQueueDefault

	createJobActivityName                        = contracts.CreateJobActivityName
	startJobStepActivityName                     = contracts.StartJobStepActivityName
	completeJobStepActivityName                  = contracts.CompleteJobStepActivityName
	ensureAccountActivityName                    = contracts.EnsureAccountActivityName
	resolveAccountActivityName                   = contracts.ResolveAccountActivityName
	browserAuthStartActivityName                 = contracts.BrowserAuthStartActivityName
	browserAuthWaitActivityName                  = contracts.BrowserAuthWaitActivityName
	browserAuthCompleteActivityName              = contracts.BrowserAuthCompleteActivityName
	browserAuthCancelActivityName                = contracts.BrowserAuthCancelActivityName
	waitOTPActivityName                          = contracts.WaitOTPActivityName
	fetchManualOTPActivityName                   = contracts.FetchManualOTPActivityName
	ensureLogonActivityName                      = contracts.EnsureLogonActivityName
	goPayPaymentPrepareActivityName              = contracts.GoPayPaymentPrepareActivityName
	goPayPaymentStartActivityName                = contracts.GoPayPaymentStartActivityName
	goPayPaymentOTPResendActivityName            = contracts.GoPayPaymentOTPResendActivityName
	goPayPaymentCompleteActivityName             = contracts.GoPayPaymentCompleteActivityName
	goPayPaymentCancelActivityName               = contracts.GoPayPaymentCancelActivityName
	goPayResolveWAPhoneActivityName              = contracts.GoPayResolveWAPhoneActivityName
	goPayAppLoadStateActivityName                = contracts.GoPayAppLoadStateActivityName
	goPayAppSaveStateActivityName                = contracts.GoPayAppSaveStateActivityName
	goPayAppDeleteStateActivityName              = contracts.GoPayAppDeleteStateActivityName
	goPayPaymentRebindSourceActivityName         = contracts.GoPayPaymentRebindSourceActivityName
	goPayAppOTPStartActivityName                 = contracts.GoPayAppOTPStartActivityName
	goPayAppOTPCompleteActivityName              = contracts.GoPayAppOTPCompleteActivityName
	goPayAppOTPRetryActivityName                 = contracts.GoPayAppOTPRetryActivityName
	goPayAppStatusActivityName                   = contracts.GoPayAppStatusActivityName
	goPayAppCreatePinStartActivityName           = contracts.GoPayAppCreatePinStartActivityName
	goPayAppCreatePinRetryActivityName           = contracts.GoPayAppCreatePinRetryActivityName
	goPayAppCreatePinCompleteActivityName        = contracts.GoPayAppCreatePinCompleteActivityName
	goPayAppAcquireSignupPhoneActivityName       = contracts.GoPayAppAcquireSignupPhoneActivityName
	goPayAppDiscardSignupPhoneActivityName       = contracts.GoPayAppDiscardSignupPhoneActivityName
	goPayAppAddBalanceActivityName               = contracts.GoPayAppAddBalanceActivityName
	goPayAppChangePhoneGetNumberActivityName     = contracts.GoPayAppChangePhoneGetNumberActivityName
	goPayAppChangePhoneStartActivityName         = contracts.GoPayAppChangePhoneStartActivityName
	goPayAppChangePhoneRetryActivityName         = contracts.GoPayAppChangePhoneRetryActivityName
	goPayAppSMSCancelBeforeRotationActivityName  = contracts.GoPayAppSMSCancelBeforeRotationActivityName
	goPayAppSMSFinishActivityName                = contracts.GoPayAppSMSFinishActivityName
	goPayAppSMSRequestAdditionalCodeActivityName = contracts.GoPayAppSMSRequestAdditionalCodeActivityName
	goPayAppChangePhoneCompleteActivityName      = contracts.GoPayAppChangePhoneCompleteActivityName
	goPayAppDeactivateStartActivityName          = contracts.GoPayAppDeactivateStartActivityName
	goPayAppDeactivateCompleteActivityName       = contracts.GoPayAppDeactivateCompleteActivityName
	probePlusTrialActivityName                   = contracts.ProbePlusTrialActivityName
	probeTierActivityName                        = contracts.ProbeTierActivityName
	registerMailboxActivityName                  = contracts.RegisterMailboxActivityName
	mailboxOAuthActivityName                     = contracts.MailboxOAuthActivityName
	persistRegisteredActivityName                = contracts.PersistRegisteredActivityName
	persistActivatedActivityName                 = contracts.PersistActivatedActivityName
	markJobFailedActivityName                    = contracts.MarkJobFailedActivityName
	markJobSucceededActivityName                 = contracts.MarkJobSucceededActivityName

	manualOTPSignalName                = contracts.ManualOTPSignalName
	manualAddBalanceSignalName         = contracts.ManualAddBalanceSignalName
	goPayAddBalanceSelectionSignalName = contracts.GoPayAddBalanceSelectionSignalName
	workflowProgressQueryName          = contracts.WorkflowProgressQueryName
)
