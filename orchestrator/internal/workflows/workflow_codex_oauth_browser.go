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
	cleanupBase, _ := workflow.NewDisconnectedContext(ctx)
	releaseCtx := workflow.WithActivityOptions(cleanupBase, atomicActivityOptions(2*time.Minute))
	for attempt := 1; attempt <= codexOAuthMaxPhoneAttempts; attempt++ {
		var failedStep string
		var err error
		last, failedStep, err = runCodexOAuthAddPhoneAttempt(ctx, progress, phoneCtx, browserCtx, input, attempt)
		last.run.data["phone_attempt"] = attempt
		last.run.data["phone_max_attempts"] = codexOAuthMaxPhoneAttempts
		if err == nil {
			return last, "", nil
		}
		if reason := codexOAuthPhoneRetryReason(err.Error()); reason != "" {
			last.run.data["phone_retry_reason"] = reason
		}
		releaseCodexOAuthAttemptPhone(ctx, releaseCtx, retryCtx, input, last.phone, err)
		if reason := codexOAuthPhoneRetryReason(err.Error()); reason == "" || attempt >= codexOAuthMaxPhoneAttempts {
			return last, failedStep, err
		}
	}
	return last, stepCodexOAuthAcquirePhone, fmt.Errorf("codex oauth add phone failed after %d phone attempts", codexOAuthMaxPhoneAttempts)
}

func runCodexOAuthAddPhoneAttempt(ctx workflow.Context, progress *WorkflowProgress, phoneCtx workflow.Context, browserCtx workflow.Context, input codexOAuthAddPhoneWorkflowInput, attempt int) (codexOAuthAddPhoneAttempt, string, error) {
	run := codexOAuthBrowserRun{
		phoneLabel: input.Label,
		data: map[string]any{
			"phone_attempt":      attempt,
			"phone_max_attempts": codexOAuthMaxPhoneAttempts,
		},
	}
	result := codexOAuthAddPhoneAttempt{run: run}
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
		AllowAddPhone: true,
	}).Get(ctx, &start); err != nil {
		mergeCodexOAuthRunData(result.run.data, protoDataMap(start.GetData()))
		return result, stepCodexOAuthBrowserStart, err
	}
	mergeCodexOAuthRunData(result.run.data, protoDataMap(start.GetData()))
	if start.GetPhoneLabel() != "" {
		result.run.phoneLabel = start.GetPhoneLabel()
	}
	session = start.GetSession()
	if session == nil {
		return result, stepCodexOAuthBrowserStart, fmt.Errorf("codex oauth browser session missing")
	}

	stage, _, failedStep, err := runCodexOAuthLoginStages(ctx, progress, browserCtx, codexOAuthBrowserWorkflowInput{
		JobID:                       input.JobID,
		AccountID:                   input.AccountID,
		Label:                       input.Label,
		AllowAddPhone:               true,
		MarkPhoneConfirmedOnSuccess: true,
	}, session, result.run.data)
	if err != nil {
		return result, failedStep, err
	}

	if stage == "add_phone" {
		phone, failedStep, err := acquireCodexOAuthPhoneAfterLogin(ctx, progress, phoneCtx, input)
		result.phone = phone
		mergeCodexOAuthRunData(result.run.data, codexOAuthPhoneLeaseRunData(&phone))
		result.run.phoneReuseCount = phone.GetReuseCount()
		result.run.phoneReuseLimit = phone.GetReuseLimit()
		if err != nil {
			if reason := codexOAuthPhoneSupplyStopReason(err.Error()); reason != "" {
				result.run.data["phone_stop_reason"] = reason
			}
			return result, failedStep, err
		}
		setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserAddPhone)
		var addPhone CodexOAuthAddPhoneBrowserOutput
		if err := workflow.ExecuteActivity(browserCtx, codexOAuthAddPhoneBrowserActivityName, CodexOAuthAddPhoneBrowserInput{
			JobId:         input.JobID,
			AccountId:     input.AccountID,
			Label:         input.Label,
			Phone:         &phone,
			AllowAddPhone: true,
			Session:       session,
		}).Get(ctx, &addPhone); err != nil {
			mergeCodexOAuthRunData(result.run.data, protoDataMap(addPhone.GetData()))
			result.run.addPhoneRequired = addPhone.GetAddPhoneRequired() || strings.Contains(strings.ToLower(err.Error()), "add_phone_required")
			return result, stepCodexOAuthBrowserAddPhone, err
		}
		mergeCodexOAuthRunData(result.run.data, protoDataMap(addPhone.GetData()))
		result.run.addPhoneConfirmed = addPhone.GetAddPhoneConfirmed()
		result.run.addPhoneRequired = addPhone.GetAddPhoneRequired()
		result.run.phoneReuseCount = addPhone.GetPhoneReuseCount()
		result.run.phoneReuseLimit = addPhone.GetPhoneReuseLimit()
	}

	setWorkflowProgress(ctx, progress, stepCodexOAuthBrowserComplete)
	var complete CodexOAuthCompleteBrowserOutput
	if err := workflow.ExecuteActivity(browserCtx, codexOAuthCompleteBrowserActivityName, CodexOAuthCompleteBrowserInput{
		JobId:                       input.JobID,
		AccountId:                   input.AccountID,
		Label:                       input.Label,
		MarkPhoneConfirmedOnSuccess: true,
		Session:                     session,
	}).Get(ctx, &complete); err != nil {
		mergeCodexOAuthRunData(result.run.data, protoDataMap(complete.GetData()))
		return result, stepCodexOAuthBrowserComplete, err
	}
	mergeCodexOAuthRunData(result.run.data, protoDataMap(complete.GetData()))
	result.run.authSecretKey = complete.GetAuthSecretKey()
	return result, "", nil
}

func acquireCodexOAuthPhoneAfterLogin(ctx workflow.Context, progress *WorkflowProgress, phoneCtx workflow.Context, input codexOAuthAddPhoneWorkflowInput) (CodexOAuthPhoneLease, string, error) {
	var phone CodexOAuthPhoneLease
	var lastErr error
	for attempt := 1; attempt <= codexOAuthMaxAcquireAttempts; attempt++ {
		setWorkflowProgress(ctx, progress, stepCodexOAuthAcquirePhone)
		err := workflow.ExecuteActivity(phoneCtx, codexOAuthAcquirePhoneActivityName, CodexOAuthAcquirePhoneInput{
			JobId:         input.JobID,
			AccountId:     input.AccountID,
			Label:         input.Label,
			MaxReuseCount: input.MaxReuseCount,
		}).Get(ctx, &phone)
		if err == nil {
			return phone, "", nil
		}
		lastErr = err
		if reason := codexOAuthPhoneSupplyStopReason(err.Error()); reason == "" || attempt >= codexOAuthMaxAcquireAttempts {
			break
		}
		workflow.Sleep(ctx, time.Duration(5*attempt)*time.Second)
	}
	if reason := codexOAuthPhoneSupplyStopReason(lastErr.Error()); reason != "" {
		return phone, stepCodexOAuthAcquirePhone, fmt.Errorf("%s: %s", reason, codexOAuthCleanAcquirePhoneError(lastErr.Error()))
	}
	return phone, stepCodexOAuthAcquirePhone, lastErr
}

func releaseCodexOAuthAttemptPhone(ctx workflow.Context, releaseCtx workflow.Context, fallbackCtx workflow.Context, input codexOAuthAddPhoneWorkflowInput, phone CodexOAuthPhoneLease, err error) {
	if err == nil || strings.TrimSpace(phone.GetActivationId()) == "" {
		return
	}
	activityInput := CodexOAuthReleasePhoneInput{
		JobId:        input.JobID,
		AccountId:    input.AccountID,
		ActivationId: phone.GetActivationId(),
		Label:        input.Label,
		ErrorMessage: err.Error(),
	}
	if releaseErr := workflow.ExecuteActivity(releaseCtx, codexOAuthReleasePhoneActivityName, activityInput).Get(releaseCtx, nil); releaseErr == nil {
		return
	}
	_ = workflow.ExecuteActivity(fallbackCtx, codexOAuthReleasePhoneActivityName, activityInput).Get(ctx, nil)
}

func codexOAuthPhoneLeaseRunData(phone *CodexOAuthPhoneLease) map[string]any {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" {
		return nil
	}
	data := map[string]any{
		"profile_key":           phone.GetProfileKey(),
		"phone_reused":          phone.GetReused(),
		"phone_reuse_count":     phone.GetReuseCount(),
		"phone_reuse_limit":     phone.GetReuseLimit(),
		"phone_expires_at_unix": phone.GetExpiresAtUnix(),
		"phone_activation_id":   phone.GetActivationId(),
		"phone_country_iso2":    phone.GetCountryIso2(),
		"phone_country_code":    phone.GetCountryCallingCode(),
	}
	if strings.TrimSpace(phone.GetCountryIso2()) != "" {
		data["verification_channel"] = "sms"
	}
	if masked := codexOAuthMaskPhone(phone.GetPhoneE164(), phone.GetPhoneNational()); masked != "" {
		data["phone_mask"] = masked
	}
	return data
}

func codexOAuthMaskPhone(e164, national string) string {
	value := strings.TrimSpace(e164)
	if value == "" {
		value = strings.TrimSpace(national)
	}
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
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
	if !codexOAuthBrowserReadyStage(stage) {
		return stage, issuedAfter, codexOAuthBrowserStageStepName(stage), fmt.Errorf("codex oauth login stage not ready: %s", stage)
	}
	return stage, issuedAfter, "", nil
}

func codexOAuthBrowserReadyStage(stage string) bool {
	switch stage {
	case "add_phone", "consent", "callback":
		return true
	default:
		return false
	}
}

func codexOAuthBrowserStageStepName(stage string) string {
	switch stage {
	case "email":
		return stepCodexOAuthBrowserEmail
	case "password":
		return stepCodexOAuthBrowserPassword
	case "email_otp":
		return stepCodexOAuthBrowserEmailOTP
	default:
		return stepCodexOAuthBrowserDetect
	}
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
