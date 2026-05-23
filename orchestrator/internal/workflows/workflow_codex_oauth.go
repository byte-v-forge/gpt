package workflows

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

func CodexOAuthAddPhoneWorkflow(ctx workflow.Context, input CodexOAuthAddPhoneWorkflowInput) (CodexOAuthAddPhoneWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthAddPhoneWorkflow", input.GetJobId())
	result := CodexOAuthAddPhoneWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	phoneCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	browserCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))

	setWorkflowProgress(ctx, progress, "resolve_account")
	var account AccountRef
	if err := workflow.ExecuteActivity(retryCtx, resolveAccountActivityName, ResolveAccountInput{
		AccountId: input.GetAccountId(),
	}).Get(ctx, &account); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
		Action:    actionCodexOAuthAddPhone,
		Params: map[string]string{
			"label":           input.GetLabel(),
			"max_reuse_count": int32String(input.GetMaxReuseCount()),
		},
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var phone CodexOAuthPhoneLease
	setWorkflowProgress(ctx, progress, stepCodexOAuthAcquirePhone)
	if err := workflow.ExecuteActivity(phoneCtx, codexOAuthAcquirePhoneActivityName, CodexOAuthAcquirePhoneInput{
		JobId:         input.GetJobId(),
		AccountId:     account.GetAccountId(),
		Label:         input.GetLabel(),
		MaxReuseCount: input.GetMaxReuseCount(),
	}).Get(ctx, &phone); err != nil {
		return failCodexOAuthAddPhoneWorkflow(ctx, retryCtx, result, input.GetJobId(), stepCodexOAuthAcquirePhone, statusFailedRetryable, false, true, err, nil), nil
	}

	var run CodexOAuthRunOutput
	setWorkflowProgress(ctx, progress, stepCodexOAuthBrowser)
	if err := workflow.ExecuteActivity(browserCtx, codexOAuthRunActivityName, CodexOAuthRunInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
		Label:     input.GetLabel(),
		Phone:     &phone,
	}).Get(ctx, &run); err != nil {
		_ = workflow.ExecuteActivity(retryCtx, codexOAuthReleasePhoneActivityName, CodexOAuthReleasePhoneInput{
			JobId:        input.GetJobId(),
			AccountId:    account.GetAccountId(),
			ActivationId: phone.GetActivationId(),
			Label:        input.GetLabel(),
			ErrorMessage: err.Error(),
		}).Get(ctx, nil)
		return failCodexOAuthAddPhoneWorkflow(ctx, retryCtx, result, input.GetJobId(), stepCodexOAuthBrowser, statusFailedRetryable, false, true, err, protoDataMap(run.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: run.GetData(),
	}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)

	result.Success = true
	result.AuthSecretKey = run.GetAuthSecretKey()
	result.PhoneLabel = run.GetPhoneLabel()
	result.PhoneReuseCount = run.GetPhoneReuseCount()
	result.PhoneReuseLimit = run.GetPhoneReuseLimit()
	return result, nil
}

func failCodexOAuthAddPhoneWorkflow(ctx workflow.Context, activityCtx workflow.Context, result CodexOAuthAddPhoneWorkflowResult, jobID, stepName, status string, recoverable, retryable bool, err error, data map[string]any) CodexOAuthAddPhoneWorkflowResult {
	result.ErrorMessage = err.Error()
	markWorkflowFailure(ctx, activityCtx, jobID, stepName, status, recoverable, retryable, err, data)
	return result
}
