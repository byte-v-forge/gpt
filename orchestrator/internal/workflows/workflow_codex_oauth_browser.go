package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/workflow"
)

type codexOAuthBrowserWorkflowInput struct {
	JobID                       string
	AccountID                   string
	Label                       string
	Phone                       *CodexOAuthPhoneLease
	AllowAddPhone               bool
	MarkPhoneConfirmedOnSuccess bool
}

type codexOAuthBrowserRun struct {
	authSecretKey     string
	phoneLabel        string
	phoneReuseCount   int32
	phoneReuseLimit   int32
	addPhoneConfirmed bool
	addPhoneRequired  bool
	data              map[string]any
}

type codexOAuthAddPhoneWorkflowInput struct {
	JobID         string
	AccountID     string
	Label         string
	MaxReuseCount int32
}

type codexOAuthAddPhoneAttempt struct {
	run   codexOAuthBrowserRun
	phone CodexOAuthPhoneLease
}

func runCodexOAuthAddPhoneWithRotation(ctx workflow.Context, progress *WorkflowProgress, phoneCtx workflow.Context, browserCtx workflow.Context, retryCtx workflow.Context, input codexOAuthAddPhoneWorkflowInput) (codexOAuthAddPhoneAttempt, string, error) {
	var last codexOAuthAddPhoneAttempt
	for attempt := 1; attempt <= codexOAuthMaxPhoneAttempts; attempt++ {
		setWorkflowProgress(ctx, progress, stepCodexOAuthAcquirePhone)
		var phone CodexOAuthPhoneLease
		if err := workflow.ExecuteActivity(phoneCtx, codexOAuthAcquirePhoneActivityName, CodexOAuthAcquirePhoneInput{
			JobId:         input.JobID,
			AccountId:     input.AccountID,
			Label:         input.Label,
			MaxReuseCount: input.MaxReuseCount,
		}).Get(ctx, &phone); err != nil {
			last.phone = phone
			return last, stepCodexOAuthAcquirePhone, err
		}
		last.phone = phone
		run, failedStep, err := runCodexOAuthBrowserActivities(ctx, progress, browserCtx, codexOAuthBrowserWorkflowInput{
			JobID:                       input.JobID,
			AccountID:                   input.AccountID,
			Label:                       input.Label,
			Phone:                       &phone,
			AllowAddPhone:               true,
			MarkPhoneConfirmedOnSuccess: true,
		})
		last.run = run
		last.run.data["phone_attempt"] = attempt
		last.run.data["phone_max_attempts"] = codexOAuthMaxPhoneAttempts
		if err == nil {
			return last, "", nil
		}
		if reason := codexOAuthPhoneRetryReason(err.Error()); reason != "" {
			last.run.data["phone_retry_reason"] = reason
		}
		_ = workflow.ExecuteActivity(retryCtx, codexOAuthReleasePhoneActivityName, CodexOAuthReleasePhoneInput{
			JobId:        input.JobID,
			AccountId:    input.AccountID,
			ActivationId: phone.GetActivationId(),
			Label:        input.Label,
			ErrorMessage: err.Error(),
		}).Get(ctx, nil)
		if reason := codexOAuthPhoneRetryReason(err.Error()); reason == "" || attempt >= codexOAuthMaxPhoneAttempts {
			return last, failedStep, err
		}
	}
	return last, stepCodexOAuthAcquirePhone, fmt.Errorf("codex oauth add phone failed after %d phone attempts", codexOAuthMaxPhoneAttempts)
}

func runCodexOAuthBrowserActivities(ctx workflow.Context, progress *WorkflowProgress, browserCtx workflow.Context, input codexOAuthBrowserWorkflowInput) (codexOAuthBrowserRun, string, error) {
	run := codexOAuthBrowserRun{
		phoneLabel:      input.Label,
		phoneReuseCount: input.Phone.GetReuseCount(),
		phoneReuseLimit: input.Phone.GetReuseLimit(),
		data:            map[string]any{},
	}
	var session *CodexOAuthBrowserSession
	cleanupBase, _ := workflow.NewDisconnectedContext(ctx)
	cleanupCtx := workflow.WithActivityOptions(cleanupBase, atomicActivityOptions(30*time.Second))
	defer func() {
		if session != nil {
			_ = workflow.ExecuteActivity(cleanupCtx, codexOAuthStopBrowserActivityName, CodexOAuthStopBrowserInput{
				JobId:   input.JobID,
				Session: session,
				Reason:  "codex oauth browser cleanup",
			}).Get(cleanupCtx, nil)
		}
	}()

	setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserStart)
	var start CodexOAuthStartBrowserOutput
	if err := workflow.ExecuteActivity(browserCtx, codexOAuthStartBrowserActivityName, CodexOAuthStartBrowserInput{
		JobId:         input.JobID,
		AccountId:     input.AccountID,
		Label:         input.Label,
		Phone:         input.Phone,
		AllowAddPhone: input.AllowAddPhone,
	}).Get(ctx, &start); err != nil {
		mergeCodexOAuthRunData(run.data, protoDataMap(start.GetData()))
		return run, stepCodexOAuthBrowserStart, err
	}
	mergeCodexOAuthRunData(run.data, protoDataMap(start.GetData()))
	if start.GetPhoneLabel() != "" {
		run.phoneLabel = start.GetPhoneLabel()
	}
	session = start.GetSession()
	if session == nil {
		return run, stepCodexOAuthBrowserStart, fmt.Errorf("codex oauth browser session missing")
	}

	setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserLogin)
	var login CodexOAuthLoginBrowserOutput
	if err := workflow.ExecuteActivity(browserCtx, codexOAuthLoginBrowserActivityName, CodexOAuthLoginBrowserInput{
		JobId:         input.JobID,
		AccountId:     input.AccountID,
		Label:         input.Label,
		Phone:         input.Phone,
		AllowAddPhone: input.AllowAddPhone,
		Session:       session,
	}).Get(ctx, &login); err != nil {
		mergeCodexOAuthRunData(run.data, protoDataMap(login.GetData()))
		run.addPhoneRequired = login.GetAddPhoneRequired() || strings.Contains(strings.ToLower(err.Error()), "add_phone_required")
		return run, stepCodexOAuthBrowserLogin, err
	}
	mergeCodexOAuthRunData(run.data, protoDataMap(login.GetData()))
	run.addPhoneConfirmed = login.GetAddPhoneConfirmed()
	run.addPhoneRequired = login.GetAddPhoneRequired()
	run.phoneReuseCount = login.GetPhoneReuseCount()
	run.phoneReuseLimit = login.GetPhoneReuseLimit()

	setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserComplete)
	var complete CodexOAuthCompleteBrowserOutput
	if err := workflow.ExecuteActivity(browserCtx, codexOAuthCompleteBrowserActivityName, CodexOAuthCompleteBrowserInput{
		JobId:                       input.JobID,
		AccountId:                   input.AccountID,
		Label:                       input.Label,
		MarkPhoneConfirmedOnSuccess: input.MarkPhoneConfirmedOnSuccess,
		Session:                     session,
	}).Get(ctx, &complete); err != nil {
		mergeCodexOAuthRunData(run.data, protoDataMap(complete.GetData()))
		return run, stepCodexOAuthBrowserComplete, err
	}
	mergeCodexOAuthRunData(run.data, protoDataMap(complete.GetData()))
	run.authSecretKey = complete.GetAuthSecretKey()
	return run, "", nil
}

func mergeCodexOAuthRunData(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}
