package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/workflow"
)

func GoPayPaymentRebindWorkflow(ctx workflow.Context, input GoPayPaymentRebindWorkflowInput) (GoPayPaymentRebindWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "GoPayPaymentRebindWorkflow", input.GetJobId())
	result := GoPayPaymentRebindWorkflowResult{
		JobId:       input.GetJobId(),
		SourceJobId: input.GetSourceJobId(),
	}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	gopayCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))

	userID := strings.TrimSpace(input.GetUserId())
	if userID == "" {
		userID = goPayLocalSource
	}
	combined := map[string]any{
		"source_job_id": input.GetSourceJobId(),
		"user_id":       userID,
	}

	var source GoPayPaymentRebindSourceOutput
	setWorkflowProgress(ctx, progress, "resolve_rebind_source")
	if err := workflow.ExecuteActivity(retryCtx, goPayPaymentRebindSourceActivityName, GoPayPaymentRebindSourceInput{
		JobId:       input.GetJobId(),
		SourceJobId: input.GetSourceJobId(),
		AccountId:   input.GetAccountId(),
		UserId:      input.GetUserId(),
	}).Get(ctx, &source); err != nil {
		combined["rebind_source"] = protoDataMap(source.GetData())
		return failGoPayPaymentRebindWorkflow(ctx, retryCtx, result, input.GetJobId(), "resolve_rebind_source", statusFailedRetryable, false, true, err, combined), nil
	}
	combined["rebind_source"] = protoDataMap(source.GetData())
	userID = source.GetUserId()
	result.UserId = userID
	result.AccountId = source.GetAccountId()
	result.WaPhone = source.GetWaPhone()

	setWorkflowProgress(ctx, progress, "create_job")
	params := map[string]string{
		"source_job_id": source.GetSourceJobId(),
		"user_id":       userID,
	}
	if strings.TrimSpace(source.GetWaPhone()) != "" {
		params["wa_phone"] = source.GetWaPhone()
	}
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: source.GetAccountId(),
		Action:    actionGoPayPaymentRebind,
		Params:    params,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var stored GoPayAppStateActivityOutput
	setWorkflowProgress(ctx, progress, "load_gopay_state")
	if err := workflow.ExecuteActivity(retryCtx, goPayAppLoadStateActivityName, GoPayAppStateActivityInput{
		JobId:  input.GetJobId(),
		UserId: userID,
		Reason: "payment_rebind_retry",
	}).Get(ctx, &stored); err != nil {
		combined["load_state"] = protoDataMap(stored.GetData())
		return failGoPayPaymentRebindWorkflow(ctx, retryCtx, result, input.GetJobId(), "load_gopay_state", statusFailedRetryable, false, true, err, combined), nil
	}
	stateJSON := stored.GetStateJson()
	if strings.TrimSpace(stateJSON) == "" {
		stateJSON = "{}"
	}
	combined["load_state"] = protoDataMap(stored.GetData())

	setWorkflowProgress(ctx, progress, stepGoPayAppLogin)
	auth, err := runGoPayAppAuth(ctx, gopayCtx, retryCtx, input.GetJobId(), goPayAppOTPOptions{
		Phone:      source.GetWaPhone(),
		OTPChannel: "wa",
		Source:     userID,
		StateJSON:  stateJSON,
	})
	combined["login"] = protoDataMap(auth.GetData())
	if nextStateJSON := strings.TrimSpace(auth.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if err != nil {
		return failGoPayPaymentRebindWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, err, combined), nil
	}
	_ = workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
		JobId:     input.GetJobId(),
		UserId:    userID,
		StateJson: stateJSON,
		Reason:    "payment_rebind_login_ready",
	}).Get(ctx, nil)

	setWorkflowProgress(ctx, progress, stepGoPayAppChangePhone)
	changePhone, err := runGoPayAppChangePhone(ctx, gopayCtx, input.GetJobId(), stateJSON)
	combined["change_phone"] = protoDataMap(changePhone.GetData())
	result.ActivationId = changePhone.GetActivationId()
	result.BoundPhone = changePhone.GetPhone()
	result.ChangePhoneComplete = changePhone.GetChangePhoneComplete()
	nextStateJSON := strings.TrimSpace(changePhone.GetStateJson())
	if nextStateJSON != "" {
		stateJSON = nextStateJSON
		_ = workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
			JobId:     input.GetJobId(),
			UserId:    userID,
			StateJson: stateJSON,
			Reason:    "payment_rebind_attempt",
		}).Get(ctx, nil)
	}
	if err != nil {
		return failGoPayPaymentRebindWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppChangePhone, statusFailedRetryable, false, true, err, combined), nil
	}
	if !result.GetChangePhoneComplete() {
		err := fmt.Errorf("gopay payment rebind did not complete")
		return failGoPayPaymentRebindWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppChangePhone, statusFailedRetryable, false, true, err, combined), nil
	}
	if err := finishGoPayChangePhoneSMS(ctx, retryCtx, input.GetJobId(), result.GetActivationId(), "payment_rebind_retry_complete"); err != nil {
		return failGoPayPaymentRebindWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSMSFinish, statusFailedRetryable, false, true, err, combined), nil
	}
	_ = workflow.ExecuteActivity(retryCtx, goPayAppDeleteStateActivityName, GoPayAppStateActivityInput{
		JobId:  input.GetJobId(),
		UserId: userID,
		Reason: "payment_rebind_complete",
	}).Get(ctx, nil)

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = true
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}
