package workflows

import (
	"strings"
	"time"

	"go.temporal.io/sdk/workflow"
)

func CodexOAuthWorkflow(ctx workflow.Context, input CodexOAuthWorkflowInput) (CodexOAuthWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthWorkflow", input.GetJobId())
	result := CodexOAuthWorkflowResult{JobId: input.GetJobId()}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	browserCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))
	account, ok := resolveCodexOAuthAccount(ctx, retryCtx, progress, input.GetJobId(), input.GetAccountId(), actionCodexOAuth, nil)
	if !ok {
		result.ErrorMessage = "resolve account failed"
		return result, nil
	}

	var run CodexOAuthRunOutput
	setWorkflowProgress(ctx, progress, stepCodexOAuthBrowser)
	if err := workflow.ExecuteActivity(browserCtx, codexOAuthRunActivityName, CodexOAuthRunInput{
		JobId:         input.GetJobId(),
		AccountId:     account.GetAccountId(),
		Label:         input.GetLabel(),
		AllowAddPhone: false,
	}).Get(ctx, &run); err != nil {
		return failCodexOAuthWorkflow(ctx, retryCtx, result, input.GetJobId(), stepCodexOAuthBrowser, statusFailedRecoverable, true, false, err, protoDataMap(run.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: run.GetData(),
	}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	result.Success = true
	result.AuthSecretKey = run.GetAuthSecretKey()
	result.AddPhoneRequired = run.GetAddPhoneRequired()
	return result, nil
}

func CodexOAuthAddPhoneWorkflow(ctx workflow.Context, input CodexOAuthAddPhoneWorkflowInput) (CodexOAuthAddPhoneWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthAddPhoneWorkflow", input.GetJobId())
	result := CodexOAuthAddPhoneWorkflowResult{JobId: input.GetJobId()}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	phoneCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	browserCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))
	account, ok := resolveCodexOAuthAccount(ctx, retryCtx, progress, input.GetJobId(), input.GetAccountId(), actionCodexOAuthAddPhone, map[string]string{
		"label":           input.GetLabel(),
		"max_reuse_count": int32String(input.GetMaxReuseCount()),
	})
	if !ok {
		result.ErrorMessage = "resolve account failed"
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
		JobId:                       input.GetJobId(),
		AccountId:                   account.GetAccountId(),
		Label:                       input.GetLabel(),
		Phone:                       &phone,
		AllowAddPhone:               true,
		MarkPhoneConfirmedOnSuccess: true,
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

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{JobId: input.GetJobId(), Result: run.GetData()}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	result.Success = true
	result.AuthSecretKey = run.GetAuthSecretKey()
	result.PhoneLabel = run.GetPhoneLabel()
	result.PhoneReuseCount = run.GetPhoneReuseCount()
	result.PhoneReuseLimit = run.GetPhoneReuseLimit()
	return result, nil
}

func CodexOAuthBatchAddPhoneWorkflow(ctx workflow.Context, input CodexOAuthBatchAddPhoneWorkflowInput) (CodexOAuthBatchAddPhoneWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthBatchAddPhoneWorkflow", input.GetJobId())
	result := CodexOAuthBatchAddPhoneWorkflowResult{JobId: input.GetJobId(), TotalCount: int32(len(input.GetAccountIds()))}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	phoneCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	browserCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))
	accountIDs := compactAccountIDs(input.GetAccountIds())
	if len(accountIDs) == 0 {
		result.ErrorMessage = "account_ids is required"
		return result, nil
	}

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:  input.GetJobId(),
		Action: actionCodexOAuthBatchAddPhone,
		Params: map[string]string{
			"label":           input.GetLabel(),
			"max_reuse_count": int32String(input.GetMaxReuseCount()),
			"account_count":   int32String(int32(len(accountIDs))),
		},
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var phone CodexOAuthPhoneLease
	setWorkflowProgress(ctx, progress, stepCodexOAuthAcquirePhone)
	if err := workflow.ExecuteActivity(phoneCtx, codexOAuthAcquirePhoneActivityName, CodexOAuthAcquirePhoneInput{
		JobId:         input.GetJobId(),
		AccountId:     accountIDs[0],
		Label:         input.GetLabel(),
		MaxReuseCount: input.GetMaxReuseCount(),
	}).Get(ctx, &phone); err != nil {
		return failCodexOAuthBatchAddPhoneWorkflow(ctx, retryCtx, result, input.GetJobId(), stepCodexOAuthAcquirePhone, statusFailedRetryable, false, true, err, nil), nil
	}
	result.PhoneLabel = phone.GetLabel()
	result.PhoneReuseCount = phone.GetReuseCount()
	result.PhoneReuseLimit = phone.GetReuseLimit()

	for _, accountID := range accountIDs {
		var run CodexOAuthRunOutput
		setWorkflowProgress(ctx, progress, stepCodexOAuthBrowser)
		err := workflow.ExecuteActivity(browserCtx, codexOAuthRunActivityName, CodexOAuthRunInput{
			JobId:                       input.GetJobId(),
			AccountId:                   accountID,
			Label:                       input.GetLabel(),
			Phone:                       &phone,
			AllowAddPhone:               true,
			MarkPhoneConfirmedOnSuccess: true,
		}).Get(ctx, &run)
		if err != nil {
			if reason := codexOAuthBatchStopReason(err.Error()); reason != "" {
				result.StoppedReason = reason
				result.Success = true
				markCodexOAuthBatchSucceeded(ctx, retryCtx, result)
				setWorkflowProgressSucceeded(ctx, progress)
				return result, nil
			}
			return failCodexOAuthBatchAddPhoneWorkflow(ctx, retryCtx, result, input.GetJobId(), stepCodexOAuthBrowser, statusFailedRetryable, false, true, err, protoDataMap(run.GetData())), nil
		}
		result.SuccessCount++
		result.ProcessedAccountIds = append(result.ProcessedAccountIds, accountID)
		if run.GetAddPhoneConfirmed() {
			result.AddPhoneCount++
		} else {
			result.DirectOauthCount++
		}
		phone.ReuseCount = run.GetPhoneReuseCount()
		phone.ReuseLimit = run.GetPhoneReuseLimit()
		result.PhoneReuseCount = phone.GetReuseCount()
		result.PhoneReuseLimit = phone.GetReuseLimit()
		if phone.GetReuseLimit() > 0 && phone.GetReuseCount() >= phone.GetReuseLimit() {
			result.StoppedReason = "phone_reuse_exhausted"
			break
		}
	}

	result.Success = true
	markCodexOAuthBatchSucceeded(ctx, retryCtx, result)
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func resolveCodexOAuthAccount(ctx workflow.Context, activityCtx workflow.Context, progress *WorkflowProgress, jobID, accountID, action string, params map[string]string) (AccountRef, bool) {
	setWorkflowProgress(ctx, progress, "resolve_account")
	var account AccountRef
	if err := workflow.ExecuteActivity(activityCtx, resolveAccountActivityName, ResolveAccountInput{AccountId: accountID}).Get(ctx, &account); err != nil {
		return account, false
	}
	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(activityCtx, createJobActivityName, CreateJobInput{JobId: jobID, AccountId: account.GetAccountId(), Action: action, Params: params}).Get(ctx, nil); err != nil {
		return account, false
	}
	return account, true
}

func markCodexOAuthBatchSucceeded(ctx workflow.Context, activityCtx workflow.Context, result CodexOAuthBatchAddPhoneWorkflowResult) {
	_ = workflow.ExecuteActivity(activityCtx, markJobSucceededActivityName, JobSuccessInput{JobId: result.GetJobId(), Result: protoData(map[string]any{
		"total_count":           result.GetTotalCount(),
		"success_count":         result.GetSuccessCount(),
		"add_phone_count":       result.GetAddPhoneCount(),
		"direct_oauth_count":    result.GetDirectOauthCount(),
		"stopped_reason":        result.GetStoppedReason(),
		"phone_label":           result.GetPhoneLabel(),
		"phone_reuse_count":     result.GetPhoneReuseCount(),
		"phone_reuse_limit":     result.GetPhoneReuseLimit(),
		"processed_account_ids": result.GetProcessedAccountIds(),
	})}).Get(ctx, nil)
}

func failCodexOAuthWorkflow(ctx workflow.Context, activityCtx workflow.Context, result CodexOAuthWorkflowResult, jobID, stepName, status string, recoverable, retryable bool, err error, data map[string]any) CodexOAuthWorkflowResult {
	result.ErrorMessage = err.Error()
	if strings.Contains(strings.ToLower(err.Error()), "add_phone_required") {
		result.AddPhoneRequired = true
	}
	markWorkflowFailure(ctx, activityCtx, jobID, stepName, status, recoverable, retryable, err, data)
	return result
}

func failCodexOAuthAddPhoneWorkflow(ctx workflow.Context, activityCtx workflow.Context, result CodexOAuthAddPhoneWorkflowResult, jobID, stepName, status string, recoverable, retryable bool, err error, data map[string]any) CodexOAuthAddPhoneWorkflowResult {
	result.ErrorMessage = err.Error()
	markWorkflowFailure(ctx, activityCtx, jobID, stepName, status, recoverable, retryable, err, data)
	return result
}

func failCodexOAuthBatchAddPhoneWorkflow(ctx workflow.Context, activityCtx workflow.Context, result CodexOAuthBatchAddPhoneWorkflowResult, jobID, stepName, status string, recoverable, retryable bool, err error, data map[string]any) CodexOAuthBatchAddPhoneWorkflowResult {
	result.ErrorMessage = err.Error()
	markWorkflowFailure(ctx, activityCtx, jobID, stepName, status, recoverable, retryable, err, data)
	return result
}

func compactAccountIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		accountID := strings.TrimSpace(value)
		if accountID == "" || seen[accountID] {
			continue
		}
		seen[accountID] = true
		out = append(out, accountID)
	}
	return out
}

func codexOAuthBatchStopReason(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "phone_reuse_exhausted") || strings.Contains(text, "phone_reuse_exceeded") || strings.Contains(text, "maximum number") || strings.Contains(text, "too many"):
		return "phone_reuse_exhausted"
	case strings.Contains(text, "phone_expired"):
		return "phone_expired"
	case strings.Contains(text, "phone_sms_timeout") || strings.Contains(text, "otp not found"):
		return "phone_sms_timeout"
	default:
		return ""
	}
}
