package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/workflow"
)

func CodexOAuthProtocolWorkflow(ctx workflow.Context, input CodexOAuthWorkflowInput) (CodexOAuthWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "CodexOAuthProtocolWorkflow", input.GetJobId())
	result := CodexOAuthWorkflowResult{JobId: input.GetJobId()}
	defer func() { finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage()) }()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	protocolCtx := workflow.WithActivityOptions(ctx, heartbeatingActivityOptions(15*time.Minute, 30*time.Second))
	account, ok := resolveCodexOAuthAccount(ctx, retryCtx, progress, input.GetJobId(), input.GetAccountId(), actionCodexOAuthProtocol, nil)
	if !ok {
		result.ErrorMessage = "resolve account failed"
		return result, nil
	}

	run, failedStep, err := runCodexOAuthProtocolActivities(ctx, progress, protocolCtx, codexOAuthBrowserWorkflowInput{
		JobID:         input.GetJobId(),
		AccountID:     account.GetAccountId(),
		Label:         input.GetLabel(),
		AllowAddPhone: false,
	})
	if err != nil {
		return failCodexOAuthWorkflow(ctx, retryCtx, result, input.GetJobId(), failedStep, statusFailedRecoverable, true, false, err, run.data), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{JobId: input.GetJobId(), Result: protoData(run.data)}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	result.Success = true
	result.AuthSecretKey = run.authSecretKey
	result.AddPhoneRequired = run.addPhoneRequired
	return result, nil
}

func runCodexOAuthProtocolActivities(ctx workflow.Context, progress *WorkflowProgress, protocolCtx workflow.Context, input codexOAuthBrowserWorkflowInput) (codexOAuthBrowserRun, string, error) {
	run := codexOAuthBrowserRun{phoneLabel: input.Label, data: map[string]any{"driver": "protocol"}}
	var session *CodexOAuthBrowserSession
	cleanupBase, _ := workflow.NewDisconnectedContext(ctx)
	cleanupCtx := workflow.WithActivityOptions(cleanupBase, atomicActivityOptions(30*time.Second))
	defer func() {
		if session != nil {
			_ = workflow.ExecuteActivity(cleanupCtx, codexOAuthStopProtocolActivityName, CodexOAuthStopBrowserInput{JobId: input.JobID, Session: session, Reason: "codex oauth protocol cleanup"}).Get(cleanupCtx, nil)
		}
	}()

	proxyData, failedStep, err := runProtocolUseProxy(ctx, progress, protocolCtx, input.JobID, input.AccountID, "codex_oauth")
	mergeCodexOAuthRunData(run.data, proxyData)
	if err != nil {
		return run, failedStep, err
	}

	setWorkflowProgress(ctx, progress, stepCodexOAuthProtocolStart)
	var start CodexOAuthStartBrowserOutput
	if err := workflow.ExecuteActivity(protocolCtx, codexOAuthStartProtocolActivityName, CodexOAuthStartBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label}).Get(ctx, &start); err != nil {
		mergeCodexOAuthRunData(run.data, protoDataMap(start.GetData()))
		return run, stepCodexOAuthProtocolStart, err
	}
	mergeCodexOAuthRunData(run.data, protoDataMap(start.GetData()))
	if start.GetPhoneLabel() != "" {
		run.phoneLabel = start.GetPhoneLabel()
	}
	session = start.GetSession()
	if session == nil {
		return run, stepCodexOAuthProtocolStart, fmt.Errorf("codex oauth protocol session missing")
	}

	stage, _, failedStep, err := runCodexOAuthProtocolLoginStages(ctx, progress, protocolCtx, input, session, run.data)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "add_phone_required") || stage == "add_phone" {
			run.addPhoneRequired = true
		}
		return run, failedStep, err
	}
	if stage == "add_phone" {
		run.addPhoneRequired = true
		return run, stepCodexOAuthProtocolDetect, fmt.Errorf("codex_oauth_add_phone_required")
	}

	setWorkflowProgress(ctx, progress, stepCodexOAuthProtocolComplete)
	var complete CodexOAuthCompleteBrowserOutput
	if err := workflow.ExecuteActivity(protocolCtx, codexOAuthCompleteProtocolActivityName, CodexOAuthCompleteBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session}).Get(ctx, &complete); err != nil {
		mergeCodexOAuthRunData(run.data, protoDataMap(complete.GetData()))
		return run, stepCodexOAuthProtocolComplete, err
	}
	mergeCodexOAuthRunData(run.data, protoDataMap(complete.GetData()))
	run.authSecretKey = complete.GetAuthSecretKey()
	return run, "", nil
}

func runCodexOAuthProtocolLoginStages(ctx workflow.Context, progress *WorkflowProgress, protocolCtx workflow.Context, input codexOAuthBrowserWorkflowInput, session *CodexOAuthBrowserSession, data map[string]any) (string, int64, string, error) {
	stepInput := CodexOAuthBrowserStepInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session}
	stage, issuedAfter, failedStep, err := runCodexOAuthStageActivity(ctx, progress, protocolCtx, stepCodexOAuthProtocolDetect, codexOAuthDetectProtocolStageActivityName, stepInput, data)
	if err != nil {
		return stage, issuedAfter, failedStep, err
	}
	if stage == "email" || stage == "" {
		stage, issuedAfter, failedStep, err = runCodexOAuthStageActivity(ctx, progress, protocolCtx, stepCodexOAuthProtocolEmail, codexOAuthSubmitProtocolEmailActivityName, stepInput, data)
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "password" {
		stage, issuedAfter, failedStep, err = runCodexOAuthStageActivity(ctx, progress, protocolCtx, stepCodexOAuthProtocolPassword, codexOAuthSubmitProtocolPasswordActivityName, stepInput, data)
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "email_otp" {
		issuedAfter, err = codexOAuthEmailOTPIssuedAfter(ctx, issuedAfter)
		if err != nil {
			return stage, issuedAfter, stepCodexOAuthProtocolEmailOTP, err
		}
		setWorkflowProgress(ctx, progress, stepCodexOAuthProtocolEmailOTP)
		var otp CodexOAuthBrowserStageOutput
		err := workflow.ExecuteActivity(protocolCtx, codexOAuthSubmitProtocolEmailOTPActivityName, CodexOAuthSubmitEmailOTPInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session, IssuedAfterUnix: issuedAfter}).Get(ctx, &otp)
		mergeCodexOAuthRunData(data, protoDataMap(otp.GetData()))
		if err != nil {
			return otp.GetStage(), issuedAfter, stepCodexOAuthProtocolEmailOTP, err
		}
		stage = otp.GetStage()
	}
	if stage == "add_phone" {
		if input.AllowAddPhone {
			return stage, issuedAfter, "", nil
		}
		return stage, issuedAfter, stepCodexOAuthProtocolDetect, fmt.Errorf("codex_oauth_add_phone_required")
	}
	if !codexOAuthProtocolReadyStage(stage) {
		return stage, issuedAfter, codexOAuthProtocolStageStepName(stage), fmt.Errorf("codex oauth protocol login stage not ready: %s", stage)
	}
	return stage, issuedAfter, "", nil
}

func codexOAuthProtocolReadyStage(stage string) bool {
	switch stage {
	case "consent", "callback":
		return true
	default:
		return false
	}
}

func codexOAuthProtocolStageStepName(stage string) string {
	switch stage {
	case "email":
		return stepCodexOAuthProtocolEmail
	case "password":
		return stepCodexOAuthProtocolPassword
	case "email_otp":
		return stepCodexOAuthProtocolEmailOTP
	case "add_phone":
		return stepCodexOAuthProtocolAddPhone
	default:
		return stepCodexOAuthProtocolDetect
	}
}
