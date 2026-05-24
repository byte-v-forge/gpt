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

	run, failedStep, err := runCodexOAuthBrowserActivities(ctx, progress, browserCtx, codexOAuthBrowserWorkflowInput{
		JobID:         input.GetJobId(),
		AccountID:     account.GetAccountId(),
		Label:         input.GetLabel(),
		AllowAddPhone: false,
	})
	if err != nil {
		return failCodexOAuthWorkflow(ctx, retryCtx, result, input.GetJobId(), failedStep, statusFailedRecoverable, true, false, err, run.data), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(run.data),
	}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	result.Success = true
	result.AuthSecretKey = run.authSecretKey
	result.AddPhoneRequired = run.addPhoneRequired
	return result, nil
}

func CodexOAuthAddPhoneWorkflow(ctx workflow.Context, input CodexOAuthAddPhoneWorkflowInput) (CodexOAuthAddPhoneWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthAddPhoneWorkflow", input.GetJobId())
	result := CodexOAuthAddPhoneWorkflowResult{JobId: input.GetJobId()}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	phoneCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	protocolCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))
	account, ok := resolveCodexOAuthAccount(ctx, retryCtx, progress, input.GetJobId(), input.GetAccountId(), actionCodexOAuthAddPhone, map[string]string{
		"label":           input.GetLabel(),
		"max_reuse_count": int32String(input.GetMaxReuseCount()),
	})
	if !ok {
		result.ErrorMessage = "resolve account failed"
		return result, nil
	}

	attempt, failedStep, err := runCodexOAuthAddPhoneWithRotation(ctx, progress, phoneCtx, protocolCtx, retryCtx, codexOAuthAddPhoneWorkflowInput{
		JobID:         input.GetJobId(),
		AccountID:     account.GetAccountId(),
		Label:         input.GetLabel(),
		MaxReuseCount: input.GetMaxReuseCount(),
	})
	if err != nil {
		return failCodexOAuthAddPhoneWorkflow(ctx, retryCtx, result, input.GetJobId(), failedStep, statusFailedRetryable, false, true, err, attempt.run.data), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{JobId: input.GetJobId(), Result: protoData(attempt.run.data)}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	result.Success = true
	result.AuthSecretKey = attempt.run.authSecretKey
	result.PhoneLabel = attempt.run.phoneLabel
	result.PhoneReuseCount = attempt.run.phoneReuseCount
	result.PhoneReuseLimit = attempt.run.phoneReuseLimit
	return result, nil
}

func CodexOAuthBatchAddPhoneWorkflow(ctx workflow.Context, input CodexOAuthBatchAddPhoneWorkflowInput) (CodexOAuthBatchAddPhoneWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthBatchAddPhoneWorkflow", input.GetJobId())
	result := CodexOAuthBatchAddPhoneWorkflowResult{JobId: input.GetJobId(), TotalCount: int32(len(input.GetAccountIds()))}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	phoneCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	protocolCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))
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

	for _, accountID := range accountIDs {
		attempt, failedStep, err := runCodexOAuthAddPhoneWithRotation(ctx, progress, phoneCtx, protocolCtx, retryCtx, codexOAuthAddPhoneWorkflowInput{
			JobID:         input.GetJobId(),
			AccountID:     accountID,
			Label:         input.GetLabel(),
			MaxReuseCount: input.GetMaxReuseCount(),
		})
		if err != nil {
			if reason := codexOAuthBatchStopReason(err.Error()); reason != "" {
				result.StoppedReason = reason
				result.Success = true
				markCodexOAuthBatchSucceeded(ctx, retryCtx, result)
				setWorkflowProgressSucceeded(ctx, progress)
				return result, nil
			}
			return failCodexOAuthBatchAddPhoneWorkflow(ctx, retryCtx, result, input.GetJobId(), failedStep, statusFailedRetryable, false, true, err, attempt.run.data), nil
		}
		result.SuccessCount++
		result.ProcessedAccountIds = append(result.ProcessedAccountIds, accountID)
		if attempt.run.addPhoneConfirmed {
			result.AddPhoneCount++
		} else {
			result.DirectOauthCount++
		}
		result.PhoneLabel = attempt.run.phoneLabel
		result.PhoneReuseCount = attempt.run.phoneReuseCount
		result.PhoneReuseLimit = attempt.run.phoneReuseLimit
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
	if reason := codexOAuthPhoneSupplyStopReason(message); reason != "" {
		return reason
	}
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

func codexOAuthPhoneSupplyStopReason(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "sms_error_code_no_number_available") ||
		strings.Contains(text, "no upstream number available") ||
		strings.Contains(text, "no number available"):
		return "phone_no_number_available"
	case strings.Contains(text, "sms_error_code_supply_unavailable") ||
		strings.Contains(text, "supply unavailable"):
		return "phone_supply_unavailable"
	case strings.Contains(text, "sms_error_code_price_limit_exceeded") ||
		strings.Contains(text, "price limit exceeded"):
		return "phone_price_limit_exceeded"
	case strings.Contains(text, "sms_error_code_insufficient_balance") ||
		strings.Contains(text, "insufficient balance"):
		return "phone_insufficient_balance"
	case strings.Contains(text, "sms_error_code_route_not_found") ||
		strings.Contains(text, "route not found") ||
		strings.Contains(text, "no matching route"):
		return "phone_route_not_found"
	default:
		return ""
	}
}

func codexOAuthCleanAcquirePhoneError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "acquire phone failed"
	}
	if idx := strings.LastIndex(strings.ToLower(message), "sms_error_code_"); idx >= 0 {
		tail := strings.TrimSpace(message[idx:])
		if sep := strings.LastIndex(tail, ":"); sep >= 0 && sep+1 < len(tail) {
			if detail := strings.TrimSpace(tail[sep+1:]); detail != "" {
				return detail
			}
		}
		return tail
	}
	if idx := strings.LastIndex(message, "AcquireNumber:"); idx >= 0 {
		if detail := strings.TrimSpace(message[idx+len("AcquireNumber:"):]); detail != "" {
			return detail
		}
	}
	return message
}

func codexOAuthPhoneRetryReason(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "phone_rate_limited") ||
		strings.Contains(text, "too many phone verification requests") ||
		strings.Contains(text, "too many verification requests") ||
		strings.Contains(text, "try again later"):
		return ""
	case strings.Contains(text, "phone_reuse_exhausted") ||
		strings.Contains(text, "phone_reuse_exceeded") ||
		strings.Contains(text, "already linked to the maximum number") ||
		strings.Contains(text, "used too many") ||
		strings.Contains(text, "maximum number") ||
		strings.Contains(text, "too many"):
		return "phone_reuse_exhausted"
	case strings.Contains(text, "phone_rejected") ||
		strings.Contains(text, "phone_otp_input_missing") ||
		strings.Contains(text, "try a different phone") ||
		strings.Contains(text, "try another phone") ||
		strings.Contains(text, "cannot use this phone") ||
		strings.Contains(text, "can't use this phone") ||
		strings.Contains(text, "invalid phone") ||
		strings.Contains(text, "unsupported phone") ||
		strings.Contains(text, "rejected"):
		return "phone_rejected"
	case strings.Contains(text, "phone_expired"):
		return "phone_expired"
	case strings.Contains(text, "phone_sms_timeout") ||
		strings.Contains(text, "sms_error_code_timeout") ||
		strings.Contains(text, "waitforcode") ||
		strings.Contains(text, "otp not found") ||
		strings.Contains(text, "empty code"):
		return "phone_sms_timeout"
	default:
		return ""
	}
}
