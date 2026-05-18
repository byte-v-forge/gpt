package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func RegisterMailboxWorkflow(ctx workflow.Context, input RegisterMailboxWorkflowInput) (RegisterMailboxWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "RegisterMailboxWorkflow", input.GetJobId())
	result := RegisterMailboxWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:  input.GetJobId(),
		Action: actionRegisterMailbox,
		Params: map[string]string{
			"import_only": boolString(input.GetImportOnly()),
		},
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var registration MailboxRegistrationActivityOutput
	setWorkflowProgress(ctx, progress, stepRegisterMailbox)
	if err := workflow.ExecuteActivity(atomicCtx, registerMailboxActivityName, MailboxRegistrationActivityInput{
		JobId:      input.GetJobId(),
		Enabled:    !input.GetImportOnly(),
		ImportOnly: input.GetImportOnly(),
	}).Get(ctx, &registration); err != nil {
		return failRegisterMailboxWorkflow(ctx, retryCtx, result, input.GetJobId(), stepRegisterMailbox, statusFailedRetryable, false, true, err, protoDataMap(registration.GetData())), nil
	}
	if !registration.GetSuccess() {
		err := temporal.NewApplicationError(registration.GetErrorMessage(), "MailboxRegistrationFailed")
		return failRegisterMailboxWorkflow(ctx, retryCtx, result, input.GetJobId(), stepRegisterMailbox, statusFailedRetryable, false, true, err, protoDataMap(registration.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: registration.GetData(),
	}).Get(ctx, nil)

	result.Success = registration.GetSuccess()
	result.ExitCode = registration.GetExitCode()
	result.Mailboxes = registration.GetMailboxes()
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func MailboxOAuthWorkflow(ctx workflow.Context, input MailboxOAuthWorkflowInput) (MailboxOAuthWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "MailboxOAuthWorkflow", input.GetJobId())
	result := MailboxOAuthWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:  input.GetJobId(),
		Action: actionMailboxOAuth,
		Params: map[string]string{
			"email_address": input.GetEmailAddress(),
			"only_missing":  boolString(input.GetOnlyMissing()),
			"limit":         int32String(input.GetLimit()),
		},
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var oauth MailboxOAuthActivityOutput
	setWorkflowProgress(ctx, progress, stepMailboxOAuth)
	if err := workflow.ExecuteActivity(atomicCtx, mailboxOAuthActivityName, MailboxOAuthActivityInput{
		JobId:        input.GetJobId(),
		EmailAddress: input.GetEmailAddress(),
		OnlyMissing:  input.GetOnlyMissing(),
		Limit:        input.GetLimit(),
	}).Get(ctx, &oauth); err != nil {
		return failMailboxOAuthWorkflow(ctx, retryCtx, result, input.GetJobId(), stepMailboxOAuth, statusFailedRetryable, false, true, err, protoDataMap(oauth.GetData())), nil
	}
	if !oauth.GetSuccess() {
		msg := oauth.GetErrorMessage()
		if msg == "" {
			msg = "mailbox OAuth failed"
		}
		err := temporal.NewApplicationError(msg, "MailboxOAuthFailed")
		return failMailboxOAuthWorkflow(ctx, retryCtx, result, input.GetJobId(), stepMailboxOAuth, statusFailedRetryable, false, true, err, protoDataMap(oauth.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: oauth.GetData(),
	}).Get(ctx, nil)

	result.Success = oauth.GetSuccess()
	result.Processed = oauth.GetProcessed()
	result.Succeeded = oauth.GetSucceeded()
	result.Failed = oauth.GetFailed()
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}
