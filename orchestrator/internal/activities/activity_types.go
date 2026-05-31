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
type BrowserAuthResendOTPInput = pb.BrowserAuthResendOTPInput
type BrowserAuthResendOTPOutput = pb.BrowserAuthResendOTPOutput
type BrowserAuthCompleteInput = pb.BrowserAuthCompleteInput
type BrowserAuthCancelInput = pb.BrowserAuthCancelInput
type ProtocolAuthStartInput = pb.ProtocolAuthStartInput
type ProtocolAuthStartOutput = pb.ProtocolAuthStartOutput
type ProtocolAuthWaitInput = pb.ProtocolAuthWaitInput
type ProtocolAuthWaitOutput = pb.ProtocolAuthWaitOutput
type ProtocolAuthCompleteInput = pb.ProtocolAuthCompleteInput
type ProtocolAuthCancelInput = pb.ProtocolAuthCancelInput
type CodexOAuthAcquirePhoneInput = pb.CodexOAuthAcquirePhoneInput
type CodexOAuthPhoneLease = pb.CodexOAuthPhoneLease
type CodexOAuthBrowserSession = pb.CodexOAuthBrowserSession
type CodexOAuthStartBrowserInput = pb.CodexOAuthStartBrowserInput
type CodexOAuthStartBrowserOutput = pb.CodexOAuthStartBrowserOutput
type CodexOAuthBrowserStepInput = pb.CodexOAuthBrowserStepInput
type CodexOAuthSubmitEmailOTPInput = pb.CodexOAuthSubmitEmailOTPInput
type CodexOAuthBrowserStageOutput = pb.CodexOAuthBrowserStageOutput
type CodexOAuthAddPhoneBrowserInput = pb.CodexOAuthAddPhoneBrowserInput
type CodexOAuthAddPhoneBrowserOutput = pb.CodexOAuthAddPhoneBrowserOutput
type CodexOAuthCompleteBrowserInput = pb.CodexOAuthCompleteBrowserInput
type CodexOAuthCompleteBrowserOutput = pb.CodexOAuthCompleteBrowserOutput
type CodexOAuthStopBrowserInput = pb.CodexOAuthStopBrowserInput
type CodexOAuthReleasePhoneInput = pb.CodexOAuthReleasePhoneInput
type ProbePlusTrialActivityInput = pb.ProbePlusTrialActivityInput
type ProbeTierActivityInput = pb.ProbeTierActivityInput
type ProbePlusTrialActivityOutput = pb.ProbePlusTrialActivityOutput
type ProbeTierActivityOutput = pb.ProbeTierActivityOutput

type PersistRegisteredInput = pb.PersistRegisteredInput
type JobFailureInput = pb.JobFailureInput
type JobSuccessInput = pb.JobSuccessInput
type WorkflowProgress = pb.WorkflowProgress
