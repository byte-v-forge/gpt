package workflows

import (
	"time"

	pb "orchestrator/pb"

	"go.temporal.io/sdk/workflow"
)

func RegisterAccountProtocolWorkflow(ctx workflow.Context, input RegisterAccountWorkflowInput) (RegisterAccountWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "RegisterAccountProtocolWorkflow", input.GetJobId())
	result := RegisterAccountWorkflowResult{JobId: input.GetJobId()}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	protocolCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(5*time.Minute, 30*time.Second))

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId: input.GetJobId(), AccountId: input.GetAccount().GetAccountId(), Action: actionRegisterProtocol,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var account AccountRef
	setWorkflowProgress(ctx, progress, "ensure_account")
	if err := workflow.ExecuteActivity(retryCtx, ensureAccountActivityName, EnsureAccountInput{Account: input.Account}).Get(ctx, &account); err != nil {
		return failRegisterWorkflow(ctx, retryCtx, result, input.GetJobId(), "", statusFailedRecoverable, true, false, err, nil), nil
	}

	register, failedStep, err := runProtocolRegister(ctx, progress, retryCtx, protocolCtx, input.GetJobId(), account.GetAccountId())
	if err != nil {
		status, recoverable, retryable := registerFailurePolicy(err)
		return failRegisterWorkflow(ctx, retryCtx, result, input.GetJobId(), failedStep, status, recoverable, retryable, err, protoDataMap(register.GetData())), nil
	}
	if err := workflow.ExecuteActivity(retryCtx, persistRegisteredActivityName, PersistRegisteredInput{
		AccountId: account.GetAccountId(), SessionToken: register.GetSessionToken(), AccessToken: register.GetAccessToken(),
		PlusTrialEligible: register.GetPlusTrialEligible(), PlusTrialChecked: register.GetPlusTrialChecked(),
	}).Get(ctx, nil); err != nil {
		return failRegisterWorkflow(ctx, retryCtx, result, input.GetJobId(), "", statusFailedRecoverable, true, false, err, protoDataMap(register.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{JobId: input.GetJobId(), Result: register.GetData()}).Get(ctx, nil)
	startRegisteredAccountProbeSideEffects(ctx, input.GetJobId(), account.GetAccountId())
	setWorkflowProgressSucceeded(ctx, progress)
	result.SessionToken = register.GetSessionToken()
	result.AccessToken = register.GetAccessToken()
	result.PlusTrialEligible = register.GetPlusTrialEligible()
	result.CheckoutUrl = register.GetCheckoutUrl()
	return result, nil
}

func LoginSessionProtocolWorkflow(ctx workflow.Context, input LoginSessionWorkflowInput) (LoginSessionWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "LoginSessionProtocolWorkflow", input.GetJobId())
	result := LoginSessionWorkflowResult{JobId: input.GetJobId()}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	protocolCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(5*time.Minute, 30*time.Second))

	var account AccountRef
	setWorkflowProgress(ctx, progress, "resolve_account")
	if err := workflow.ExecuteActivity(retryCtx, resolveAccountActivityName, ResolveAccountInput{AccountId: input.GetAccountId()}).Get(ctx, &account); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}
	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId: input.GetJobId(), AccountId: account.GetAccountId(), Action: actionLoginSessionProtocol,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	login, failedStep, err := runProtocolLogin(ctx, progress, retryCtx, protocolCtx, input.GetJobId(), account.GetAccountId())
	if err != nil {
		return failLoginSessionWorkflow(ctx, retryCtx, result, input.GetJobId(), failedStep, statusFailedRetryable, false, true, err, protoDataMap(login.GetData())), nil
	}
	if err := workflow.ExecuteActivity(retryCtx, persistRegisteredActivityName, PersistRegisteredInput{
		AccountId: account.GetAccountId(), SessionToken: login.GetSessionToken(), AccessToken: login.GetAccessToken(),
	}).Get(ctx, nil); err != nil {
		return failLoginSessionWorkflow(ctx, retryCtx, result, input.GetJobId(), "", statusFailedRecoverable, true, false, err, protoDataMap(login.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{JobId: input.GetJobId(), Result: login.GetData()}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	result.SessionToken = login.GetSessionToken()
	result.AccessToken = login.GetAccessToken()
	return result, nil
}

func runProtocolRegister(ctx workflow.Context, progress *WorkflowProgress, retryCtx workflow.Context, protocolCtx workflow.Context, jobID string, accountID string) (RegisterActivityOutput, string, error) {
	var start BrowserAuthStartOutput
	setWorkflowProgress(ctx, progress, stepRegisterAccountProtocolStart)
	if err := workflow.ExecuteActivity(protocolCtx, protocolAuthStartActivityName, BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: browserAuthModeRegister}).Get(ctx, &start); err != nil {
		return RegisterActivityOutput{Data: start.GetData()}, stepRegisterAccountProtocolStart, err
	}
	register := RegisterActivityOutput{}
	if start.GetResult() != nil {
		register = *start.GetResult()
	}
	var wait BrowserAuthWaitOutput
	if start.GetResult() == nil {
		setWorkflowProgress(ctx, progress, stepRegisterAccountProtocol)
		if err := workflow.ExecuteActivity(protocolCtx, protocolAuthWaitActivityName, BrowserAuthWaitInput{JobId: jobID, AccountId: accountID, FlowId: start.GetFlowId(), Mode: browserAuthModeRegister, Email: start.GetEmail()}).Get(ctx, &wait); err != nil {
			_ = workflow.ExecuteActivity(retryCtx, protocolAuthCancelActivityName, BrowserAuthCancelInput{JobId: jobID, FlowId: start.GetFlowId(), Mode: browserAuthModeRegister}).Get(ctx, nil)
			return RegisterActivityOutput{Data: wait.GetData()}, stepRegisterAccountProtocol, err
		}
		if wait.GetResult() != nil {
			register = *wait.GetResult()
		}
	}
	if wait.GetOtpRequired() {
		otpInput := protocolEmailOTPWaitInput(jobID, stepRegisterAccountProtocolOTPWait, wait.GetEmail(), wait.GetOtpTimeoutSeconds(), wait.GetOtpIssuedAfterUnix())
		otp, err := waitForOTPInStep(ctx, retryCtx, stepRegisterAccountProtocolOTPWait, otpInput)
		if err != nil {
			_ = workflow.ExecuteActivity(retryCtx, protocolAuthCancelActivityName, BrowserAuthCancelInput{JobId: jobID, FlowId: start.GetFlowId(), Mode: browserAuthModeRegister}).Get(ctx, nil)
			return RegisterActivityOutput{Data: protoData(otpWaitStepResultData(otpInput, otp))}, stepRegisterAccountProtocolOTPWait, err
		}
		setWorkflowProgress(ctx, progress, stepRegisterAccountProtocolComplete)
		if err := workflow.ExecuteActivity(protocolCtx, protocolAuthCompleteActivityName, BrowserAuthCompleteInput{JobId: jobID, AccountId: accountID, FlowId: start.GetFlowId(), Mode: browserAuthModeRegister, OtpParam: registrationOTPParam, SubmittedAtParam: registrationOTPSubmittedAtParam, OtpIssuedAfterUnix: wait.GetOtpIssuedAfterUnix(), OtpSource: otp.GetSource()}).Get(ctx, &register); err != nil {
			_ = workflow.ExecuteActivity(retryCtx, protocolAuthCancelActivityName, BrowserAuthCancelInput{JobId: jobID, FlowId: start.GetFlowId(), Mode: browserAuthModeRegister}).Get(ctx, nil)
			return register, stepRegisterAccountProtocolComplete, err
		}
	}
	return register, "", nil
}

func runProtocolLogin(ctx workflow.Context, progress *WorkflowProgress, retryCtx workflow.Context, protocolCtx workflow.Context, jobID string, accountID string) (LoginSessionActivityOutput, string, error) {
	register, failedStep, err := runProtocolLoginRegisterOutput(ctx, progress, retryCtx, protocolCtx, jobID, accountID)
	return LoginSessionActivityOutput{SessionToken: register.GetSessionToken(), AccessToken: register.GetAccessToken(), DeviceId: register.GetDeviceId(), Data: register.GetData()}, failedStep, err
}

func runProtocolLoginRegisterOutput(ctx workflow.Context, progress *WorkflowProgress, retryCtx workflow.Context, protocolCtx workflow.Context, jobID string, accountID string) (RegisterActivityOutput, string, error) {
	var start BrowserAuthStartOutput
	setWorkflowProgress(ctx, progress, stepLoginSessionProtocolStart)
	if err := workflow.ExecuteActivity(protocolCtx, protocolAuthStartActivityName, BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: browserAuthModeLogin}).Get(ctx, &start); err != nil {
		return RegisterActivityOutput{Data: start.GetData()}, stepLoginSessionProtocolStart, err
	}
	login := RegisterActivityOutput{}
	if start.GetResult() != nil {
		login = *start.GetResult()
	}
	var wait BrowserAuthWaitOutput
	if start.GetResult() == nil {
		setWorkflowProgress(ctx, progress, stepLoginSessionProtocol)
		if err := workflow.ExecuteActivity(protocolCtx, protocolAuthWaitActivityName, BrowserAuthWaitInput{JobId: jobID, AccountId: accountID, FlowId: start.GetFlowId(), Mode: browserAuthModeLogin, Email: start.GetEmail()}).Get(ctx, &wait); err != nil {
			_ = workflow.ExecuteActivity(retryCtx, protocolAuthCancelActivityName, BrowserAuthCancelInput{JobId: jobID, FlowId: start.GetFlowId(), Mode: browserAuthModeLogin}).Get(ctx, nil)
			return RegisterActivityOutput{Data: wait.GetData()}, stepLoginSessionProtocol, err
		}
		if wait.GetResult() != nil {
			login = *wait.GetResult()
		}
	}
	if wait.GetOtpRequired() {
		otpInput := protocolEmailOTPWaitInput(jobID, stepLoginSessionProtocolOTPWait, wait.GetEmail(), wait.GetOtpTimeoutSeconds(), wait.GetOtpIssuedAfterUnix())
		otp, err := waitForOTPInStep(ctx, retryCtx, stepLoginSessionProtocolOTPWait, otpInput)
		if err != nil {
			_ = workflow.ExecuteActivity(retryCtx, protocolAuthCancelActivityName, BrowserAuthCancelInput{JobId: jobID, FlowId: start.GetFlowId(), Mode: browserAuthModeLogin}).Get(ctx, nil)
			return RegisterActivityOutput{Data: protoData(otpWaitStepResultData(otpInput, otp))}, stepLoginSessionProtocolOTPWait, err
		}
		setWorkflowProgress(ctx, progress, stepLoginSessionProtocolComplete)
		if err := workflow.ExecuteActivity(protocolCtx, protocolAuthCompleteActivityName, BrowserAuthCompleteInput{JobId: jobID, AccountId: accountID, FlowId: start.GetFlowId(), Mode: browserAuthModeLogin, OtpParam: registrationOTPParam, SubmittedAtParam: registrationOTPSubmittedAtParam, OtpIssuedAfterUnix: wait.GetOtpIssuedAfterUnix(), OtpSource: otp.GetSource()}).Get(ctx, &login); err != nil {
			_ = workflow.ExecuteActivity(retryCtx, protocolAuthCancelActivityName, BrowserAuthCancelInput{JobId: jobID, FlowId: start.GetFlowId(), Mode: browserAuthModeLogin}).Get(ctx, nil)
			return login, stepLoginSessionProtocolComplete, err
		}
	}
	return login, "", nil
}

func protocolEmailOTPWaitInput(jobID string, stepName string, email string, timeoutSeconds int32, issuedAfterUnix int64) OTPWaitInput {
	return OTPWaitInput{
		JobId: jobID, StepName: stepName,
		Target:           &pb.OTPWaitInput_Email{Email: &pb.OTPWaitEmailTarget{Email: email}},
		TimeoutSeconds:   timeoutSeconds,
		IssuedAfterUnix:  issuedAfterUnix,
		OtpParam:         registrationOTPParam,
		SubmittedAtParam: registrationOTPSubmittedAtParam,
	}
}
