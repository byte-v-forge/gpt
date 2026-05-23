package workflows

import (
	"errors"
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/workflow"
)

func GoPayPaymentWorkflow(ctx workflow.Context, input GoPayPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
	if normalizeGoPayOTPChannel(input.GetOtpChannel()) == "wa" {
		return goPayAppBackedWAPaymentWorkflow(ctx, input)
	}
	return goPaySMSPaymentWorkflow(ctx, input)
}

func GoPayWAPaymentWorkflow(ctx workflow.Context, input GoPayWAPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "GoPayWAPaymentWorkflow", input.GetJobId())
	result := GoPayPaymentWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(15*time.Minute))
	gopayCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	paymentCtx := workflow.WithActivityOptions(ctx, paymentActivityOptions())

	userID := strings.TrimSpace(input.GetUserId())
	if userID == "" {
		userID = goPayLocalSource
	}
	accountID := strings.TrimSpace(input.GetAccountId())
	sourceJobID := strings.TrimSpace(input.GetSourceJobId())
	accessToken := strings.TrimSpace(input.GetAccessToken())
	if accountID == "" && sourceJobID == "" && accessToken == "" {
		result.ErrorMessage = "account_id, source_job_id, or access_token is required"
		return result, nil
	}
	result.UserId = userID
	combined := map[string]any{
		"otp_channel":         "wa",
		"user_id":             userID,
		"payment_only":        true,
		"uses_account_token":  false,
		"uses_gopay_app_flow": false,
	}
	stateJSON := "{}"

	var account AccountRef
	if accountID != "" || sourceJobID != "" {
		setWorkflowProgress(ctx, progress, "resolve_account")
		if err := workflow.ExecuteActivity(retryCtx, resolveAccountActivityName, ResolveAccountInput{
			AccountId:   accountID,
			SourceJobId: sourceJobID,
		}).Get(ctx, &account); err != nil {
			result.ErrorMessage = err.Error()
			return result, nil
		}
		accountID = account.GetAccountId()
	}
	combined["account_id"] = accountID
	combined["access_token_present"] = accessToken != ""

	setWorkflowProgress(ctx, progress, "create_job")
	params := map[string]string{
		"otp_channel":  "wa",
		"user_id":      userID,
		"payment_only": "true",
	}
	if strings.TrimSpace(input.GetWaPhone()) != "" {
		params["wa_phone"] = strings.TrimSpace(input.GetWaPhone())
	}
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: accountID,
		Action:    actionGoPayWAPayment,
		Params:    params,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var probe ProbePlusTrialActivityOutput
	setWorkflowProgress(ctx, progress, stepProbePlusTrial)
	if err := workflow.ExecuteActivity(atomicCtx, probePlusTrialActivityName, ProbePlusTrialActivityInput{
		JobId:       input.GetJobId(),
		AccountId:   accountID,
		AccessToken: accessToken,
	}).Get(ctx, &probe); err != nil {
		combined["probe_plus_trial"] = protoDataMap(probe.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["probe_plus_trial"] = protoDataMap(probe.GetData())
	if !probe.GetChecked() {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), combined), nil
	}
	if !probe.GetPlusTrialEligible() {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), combined), nil
	}

	var waPhone GoPayResolveWAPhoneOutput
	setWorkflowProgress(ctx, progress, stepGoPayResolveWAPhone)
	if err := workflow.ExecuteActivity(gopayCtx, goPayResolveWAPhoneActivityName, GoPayResolveWAPhoneInput{
		JobId:   input.GetJobId(),
		UserId:  userID,
		WaPhone: input.GetWaPhone(),
	}).Get(ctx, &waPhone); err != nil {
		combined["wa_phone"] = protoDataMap(waPhone.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayResolveWAPhone, statusFailedRetryable, false, true, err, combined), nil
	}
	userID = waPhone.GetUserId()
	result.UserId = userID
	result.WaPhone = waPhone.GetWaPhone()
	result.Phone = waPhone.GetWaPhone()
	combined["user_id"] = userID
	combined["wa_phone"] = result.GetWaPhone()
	combined["wa_phone_resolution"] = protoDataMap(waPhone.GetData())

	var paymentPrepare GoPayPaymentPrepareOutput
	setWorkflowProgress(ctx, progress, stepGoPayPaymentPrepare)
	paymentPrepare, err := prepareGoPayPayment(ctx, paymentCtx, GoPayActivityInput{
		JobId:           input.GetJobId(),
		AccountId:       accountID,
		AccessToken:     accessToken,
		UseAccountToken: false,
		Tokenization:    "true",
		GopayPhone:      result.GetWaPhone(),
		UserId:          userID,
		StateJson:       stateJSON,
		CountryCode:     input.GetCountryCode(),
	})
	stateJSON = paymentPrepare.GetStateJson()
	combined["gopay_payment_prepare"] = protoDataMap(paymentPrepare.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPaymentPrepare, statusFailedRetryable, false, true, err, combined), nil
	}

	var payment GoPayActivityOutput
	setWorkflowProgress(ctx, progress, stepGoPayPayment)
	payment, err = runGoPayPayment(ctx, paymentCtx, retryCtx, GoPayActivityInput{
		JobId:           input.GetJobId(),
		AccountId:       accountID,
		AccessToken:     accessToken,
		UseAccountToken: false,
		Tokenization:    "true",
		PreparedFlowId:  paymentPrepare.GetFlowId(),
		GopayPhone:      result.GetWaPhone(),
		OtpChannel:      "wa",
		UserId:          userID,
		StateJson:       stateJSON,
		Pin:             input.GetPin(),
		CountryCode:     input.GetCountryCode(),
	})
	combined["gopay_payment"] = protoDataMap(payment.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPayment, statusFailedRetryable, false, true, err, combined), nil
	}
	result.ChargeRef = payment.GetChargeRef()
	result.SnapToken = payment.GetSnapToken()
	paymentSettled := payment.GetChargeRef() != "" && payment.GetPlusActive()
	combined["payment_completed"] = paymentSettled
	combined["payment_async_pending"] = false
	combined["charge_ref"] = result.GetChargeRef()
	combined["snap_token"] = result.GetSnapToken()

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = true
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func finishGoPayChangePhoneSMS(ctx workflow.Context, activityCtx workflow.Context, jobID, activationID, reason string) error {
	if strings.TrimSpace(activationID) == "" {
		return fmt.Errorf("change phone activation id is missing")
	}
	return workflow.ExecuteActivity(activityCtx, goPayAppSMSFinishActivityName, GoPayAppSMSActivationInput{
		JobId:        jobID,
		ActivationId: activationID,
		Reason:       reason,
	}).Get(ctx, nil)
}

func goPayAppDeviceProxyMatched(expected GoPayAppGenerateDeviceProxyOutput, actual GoPayAppCheckSignupPhoneOutput) bool {
	expectedProxy := strings.TrimSpace(expected.GetProxyHash())
	actualProxy := strings.TrimSpace(actual.GetProxyHash())
	expectedDevice := strings.TrimSpace(expected.GetDeviceFingerprint())
	actualDevice := strings.TrimSpace(actual.GetDeviceFingerprint())
	return expectedProxy != "" &&
		actualProxy != "" &&
		expectedDevice != "" &&
		actualDevice != "" &&
		expectedProxy == actualProxy &&
		expectedDevice == actualDevice
}

func isGoPaySignupPhoneRotatableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "signup phone already registered") ||
		strings.Contains(message, "signup phone unavailable") ||
		strings.Contains(message, "status 429") ||
		strings.Contains(message, "ratelimit") ||
		strings.Contains(message, "rate_limited") ||
		strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "client.timeout") ||
		strings.Contains(message, "awaiting headers") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "eof") ||
		strings.Contains(message, "connection reset")
}

func isGoPaySignupRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "scp-cvs:error:ratelimit:init_verification") ||
		strings.Contains(message, "ratelimit:init_verification") ||
		strings.Contains(message, "status 429") ||
		strings.Contains(message, "rate_limited")
}

func startGoPayPaymentRebindSideEffect(ctx workflow.Context, sourceJobID string, accountID string, userID string, pin string, countryCode string, combined map[string]any) {
	sourceJobID = strings.TrimSpace(sourceJobID)
	if sourceJobID == "" {
		return
	}
	rebindJobID := sourceJobID + "-rebind"
	workflowID := "gopay-payment-rebind-" + rebindJobID
	data := map[string]any{
		"job_id":        rebindJobID,
		"workflow_id":   workflowID,
		"source_job_id": sourceJobID,
		"account_id":    accountID,
		"user_id":       userID,
	}
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID:        workflowID,
		ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
	})
	future := workflow.ExecuteChildWorkflow(childCtx, GoPayPaymentRebindWorkflow, GoPayPaymentRebindWorkflowInput{
		JobId:       rebindJobID,
		SourceJobId: sourceJobID,
		AccountId:   accountID,
		UserId:      userID,
		Pin:         pin,
		CountryCode: countryCode,
	})
	err := future.GetChildWorkflowExecution().Get(ctx, nil)
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			data["started"] = true
			data["already_started"] = true
		} else {
			data["started"] = false
			data["error_message"] = err.Error()
			workflow.GetLogger(ctx).Warn("failed to start gopay payment rebind side effect", "source_job_id", sourceJobID, "error", err)
		}
	} else {
		data["started"] = true
	}
	combined["rebind"] = data
	combined["rebind_job_id"] = rebindJobID
	combined["rebind_started"] = data["started"]
}
