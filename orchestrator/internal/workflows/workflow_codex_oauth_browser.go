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
			last.run.data = map[string]any{
				"phone_attempt":      attempt,
				"phone_max_attempts": codexOAuthMaxPhoneAttempts,
			}
			if reason := codexOAuthPhoneSupplyStopReason(err.Error()); reason != "" {
				last.run.data["phone_stop_reason"] = reason
				err = fmt.Errorf("%s: %s", reason, codexOAuthCleanAcquirePhoneError(err.Error()))
			}
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

	stage, issuedAfter, failedStep, err := runCodexOAuthLoginStages(ctx, progress, browserCtx, input, session, run.data)
	if err != nil {
		return run, failedStep, err
	}
	if input.Phone != nil || stage == "add_phone" {
		setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserAddPhone)
		var addPhone CodexOAuthAddPhoneBrowserOutput
		if err := workflow.ExecuteActivity(browserCtx, codexOAuthAddPhoneBrowserActivityName, CodexOAuthAddPhoneBrowserInput{
			JobId:         input.JobID,
			AccountId:     input.AccountID,
			Label:         input.Label,
			Phone:         input.Phone,
			AllowAddPhone: input.AllowAddPhone,
			Session:       session,
		}).Get(ctx, &addPhone); err != nil {
			mergeCodexOAuthRunData(run.data, protoDataMap(addPhone.GetData()))
			run.addPhoneRequired = addPhone.GetAddPhoneRequired() || strings.Contains(strings.ToLower(err.Error()), "add_phone_required")
			return run, stepCodexOAuthBrowserAddPhone, err
		}
		mergeCodexOAuthRunData(run.data, protoDataMap(addPhone.GetData()))
		run.addPhoneConfirmed = addPhone.GetAddPhoneConfirmed()
		run.addPhoneRequired = addPhone.GetAddPhoneRequired()
		run.phoneReuseCount = addPhone.GetPhoneReuseCount()
		run.phoneReuseLimit = addPhone.GetPhoneReuseLimit()
	}
	_ = issuedAfter

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

func runCodexOAuthLoginStages(ctx workflow.Context, progress *WorkflowProgress, browserCtx workflow.Context, input codexOAuthBrowserWorkflowInput, session *CodexOAuthBrowserSession, data map[string]any) (string, int64, string, error) {
	stepInput := CodexOAuthBrowserStepInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session}
	stage, issuedAfter, failedStep, err := runCodexOAuthStageActivity(ctx, progress, browserCtx, stepCodexOAuthBrowserDetect, codexOAuthDetectBrowserStageActivityName, stepInput, data)
	if err != nil {
		return stage, issuedAfter, failedStep, err
	}
	if stage == "email" {
		stage, issuedAfter, failedStep, err = runCodexOAuthStageActivity(ctx, progress, browserCtx, stepCodexOAuthBrowserEmail, codexOAuthSubmitEmailActivityName, stepInput, data)
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "password" {
		stage, issuedAfter, failedStep, err = runCodexOAuthStageActivity(ctx, progress, browserCtx, stepCodexOAuthBrowserPassword, codexOAuthSubmitPasswordActivityName, stepInput, data)
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "email_otp" {
		if issuedAfter <= 0 {
			issuedAfter = workflow.Now(ctx).Add(-time.Second).Unix()
		}
		setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserEmailOTP)
		var otp CodexOAuthBrowserStageOutput
		err := workflow.ExecuteActivity(browserCtx, codexOAuthSubmitEmailOTPActivityName, CodexOAuthSubmitEmailOTPInput{
			JobId:           input.JobID,
			AccountId:       input.AccountID,
			Label:           input.Label,
			Session:         session,
			IssuedAfterUnix: issuedAfter,
		}).Get(ctx, &otp)
		mergeCodexOAuthRunData(data, protoDataMap(otp.GetData()))
		if err != nil {
			return otp.GetStage(), issuedAfter, stepCodexOAuthBrowserEmailOTP, err
		}
		stage = otp.GetStage()
	}
	return stage, issuedAfter, "", nil
}

func runCodexOAuthStageActivity(ctx workflow.Context, progress *WorkflowProgress, browserCtx workflow.Context, stepName, activityName string, input CodexOAuthBrowserStepInput, data map[string]any) (string, int64, string, error) {
	setWorkflowProgress(ctx, progress, stepName)
	var out CodexOAuthBrowserStageOutput
	err := workflow.ExecuteActivity(browserCtx, activityName, input).Get(ctx, &out)
	mergeCodexOAuthRunData(data, protoDataMap(out.GetData()))
	if err != nil {
		return out.GetStage(), out.GetEmailOtpIssuedAfterUnix(), stepName, err
	}
	return out.GetStage(), out.GetEmailOtpIssuedAfterUnix(), "", nil
}

func mergeCodexOAuthRunData(dst map[string]any, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}
