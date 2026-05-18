package activities

import pb "orchestrator/pb"

type AccountSpec = pb.AccountSpec
type CreateJobInput = pb.CreateJobInput
type JobStepStartInput = pb.JobStepStartInput
type JobStepCompleteInput = pb.JobStepCompleteInput
type EnsureAccountInput = pb.EnsureAccountInput
type AccountRef = pb.AccountRef
type ResolveAccountInput = pb.ResolveAccountInput

type RegisterActivityOutput = pb.RegisterActivityOutput
type BrowserAuthStartInput = pb.BrowserAuthStartInput
type BrowserAuthStartOutput = pb.BrowserAuthStartOutput
type BrowserAuthWaitInput = pb.BrowserAuthWaitInput
type BrowserAuthWaitOutput = pb.BrowserAuthWaitOutput
type BrowserAuthCompleteInput = pb.BrowserAuthCompleteInput
type BrowserAuthCancelInput = pb.BrowserAuthCancelInput
type OTPWaitInput = pb.OTPWaitInput
type OTPWaitOutput = pb.OTPWaitOutput
type ManualOTPSignal = pb.ManualOTPSignal
type GoPayActivityInput = pb.GoPayActivityInput
type GoPayActivityOutput = pb.GoPayActivityOutput
type GoPayPaymentPrepareOutput = pb.GoPayPaymentPrepareOutput
type GoPayPaymentStartOutput = pb.GoPayPaymentStartOutput
type GoPayPaymentOTPResendInput = pb.GoPayPaymentOTPResendInput
type GoPayPaymentOTPResendOutput = pb.GoPayPaymentOTPResendOutput
type GoPayPaymentCompleteInput = pb.GoPayPaymentCompleteInput
type GoPayPaymentCancelInput = pb.GoPayPaymentCancelInput
type GoPayResolveWAPhoneInput = pb.GoPayResolveWAPhoneInput
type GoPayResolveWAPhoneOutput = pb.GoPayResolveWAPhoneOutput
type GoPayAppStateActivityInput = pb.GoPayAppStateActivityInput
type GoPayAppStateActivityOutput = pb.GoPayAppStateActivityOutput
type GoPayPaymentRebindSourceInput = pb.GoPayPaymentRebindSourceInput
type GoPayPaymentRebindSourceOutput = pb.GoPayPaymentRebindSourceOutput

type GoPayAppStepInput = pb.GoPayAppStepInput
type GoPayAppStepOutput = pb.GoPayAppStepOutput
type GoPayAppChangePhoneGetNumberInput = pb.GoPayAppChangePhoneGetNumberInput
type GoPayAppChangePhoneGetNumberOutput = pb.GoPayAppChangePhoneGetNumberOutput
type GoPayAppChangePhoneStartInput = pb.GoPayAppChangePhoneStartInput
type GoPayAppChangePhoneStartOutput = pb.GoPayAppChangePhoneStartOutput
type GoPayAppAcquireSignupPhoneInput = pb.GoPayAppAcquireSignupPhoneInput
type GoPayAppAcquireSignupPhoneOutput = pb.GoPayAppAcquireSignupPhoneOutput
type GoPayAddBalance = pb.GoPayAddBalance
type GoPayAppAddBalanceInput = pb.GoPayAppAddBalanceInput
type GoPayAppAddBalanceOutput = pb.GoPayAppAddBalanceOutput
type ManualAddBalanceSignal = pb.ManualAddBalanceSignal
type GoPayAppChangePhoneRetryInput = pb.GoPayAppChangePhoneRetryInput
type GoPayAppChangePhoneRetryOutput = pb.GoPayAppChangePhoneRetryOutput
type GoPayAppSMSActivationInput = pb.GoPayAppSMSActivationInput
type GoPayAppSMSActivationOutput = pb.GoPayAppSMSActivationOutput
type GoPayAppChangePhoneCompleteInput = pb.GoPayAppChangePhoneCompleteInput
type GoPayAppChangePhoneCompleteOutput = pb.GoPayAppChangePhoneCompleteOutput
type GoPayAppDeactivateStartInput = pb.GoPayAppDeactivateStartInput
type GoPayAppDeactivateStartOutput = pb.GoPayAppDeactivateStartOutput
type GoPayAppDeactivateCompleteInput = pb.GoPayAppDeactivateCompleteInput
type GoPayAppDeactivateCompleteOutput = pb.GoPayAppDeactivateCompleteOutput
type GoPayAppOTPStartInput = pb.GoPayAppOTPStartInput
type GoPayAppOTPOutput = pb.GoPayAppOTPOutput
type GoPayAppOTPCompleteInput = pb.GoPayAppOTPCompleteInput
type GoPayAppCreatePinStartInput = pb.GoPayAppCreatePinStartInput
type GoPayAppCreatePinCompleteInput = pb.GoPayAppCreatePinCompleteInput

type ProbePlusTrialActivityInput = pb.ProbePlusTrialActivityInput
type ProbeTierActivityInput = pb.ProbeTierActivityInput
type ProbePlusTrialActivityOutput = pb.ProbePlusTrialActivityOutput
type ProbeTierActivityOutput = pb.ProbeTierActivityOutput
type LoginSessionActivityOutput = pb.LoginSessionActivityOutput

type PersistRegisteredInput = pb.PersistRegisteredInput
type PersistActivatedInput = pb.PersistActivatedInput
type JobFailureInput = pb.JobFailureInput
type JobSuccessInput = pb.JobSuccessInput
type WorkflowProgress = pb.WorkflowProgress

type MailboxRegistrationActivityInput = pb.MailboxRegistrationActivityInput
type MailboxRegistrationActivityOutput = pb.MailboxRegistrationActivityOutput
type RegisteredMailboxResult = pb.RegisteredMailboxResult
type MailboxOAuthActivityInput = pb.MailboxOAuthActivityInput
type MailboxOAuthActivityOutput = pb.MailboxOAuthActivityOutput

type RegisterAccountWorkflowInput = pb.RegisterAccountWorkflowInput
type RegisterAccountWorkflowResult = pb.RegisterAccountWorkflowResult
type ActivateAccountWorkflowInput = pb.ActivateAccountWorkflowInput
type ActivateAccountWorkflowResult = pb.ActivateAccountWorkflowResult
type AutoPayWorkflowInput = pb.AutoPayWorkflowInput
type AutoPayWorkflowResult = pb.AutoPayWorkflowResult
type GoPayAppWorkflowInput = pb.GoPayAppWorkflowInput
type GoPayAppWorkflowResult = pb.GoPayAppWorkflowResult
type GoPayPaymentWorkflowInput = pb.GoPayPaymentWorkflowInput
type GoPayPaymentWorkflowResult = pb.GoPayPaymentWorkflowResult
type GoPayPaymentRebindWorkflowInput = pb.GoPayPaymentRebindWorkflowInput
type GoPayPaymentRebindWorkflowResult = pb.GoPayPaymentRebindWorkflowResult
type ProbeAccountWorkflowInput = pb.ProbeAccountWorkflowInput
type LoginSessionWorkflowInput = pb.LoginSessionWorkflowInput
type LoginSessionWorkflowResult = pb.LoginSessionWorkflowResult
type ProbeAccountWorkflowResult = pb.ProbeAccountWorkflowResult
type RegisterAndActivateWorkflowInput = pb.RegisterAndActivateWorkflowInput
type RegisterAndActivateWorkflowResult = pb.RegisterAndActivateWorkflowResult
type RegisterMailboxWorkflowInput = pb.RegisterMailboxWorkflowInput
type RegisterMailboxWorkflowResult = pb.RegisterMailboxWorkflowResult
type MailboxOAuthWorkflowInput = pb.MailboxOAuthWorkflowInput
type MailboxOAuthWorkflowResult = pb.MailboxOAuthWorkflowResult
