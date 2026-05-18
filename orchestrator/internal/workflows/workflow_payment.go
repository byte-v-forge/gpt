package workflows

import (
	"fmt"
	pb "orchestrator/pb"
	"strings"
	"time"

	"go.temporal.io/sdk/workflow"
)

func ActivateAccountWorkflow(ctx workflow.Context, input ActivateAccountWorkflowInput) (ActivateAccountWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "ActivateAccountWorkflow", input.GetJobId())
	result := ActivateAccountWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(15*time.Minute))
	gopayCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(5*time.Minute))
	paymentCtx := workflow.WithActivityOptions(ctx, paymentActivityOptions())
	action := input.Action
	if action == "" {
		action = actionActivate
	}

	var account AccountRef
	setWorkflowProgress(ctx, progress, "resolve_account")
	if err := workflow.ExecuteActivity(retryCtx, resolveAccountActivityName, ResolveAccountInput{
		AccountId:   input.GetAccountId(),
		SourceJobId: input.GetSourceJobId(),
	}).Get(ctx, &account); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
		Action:    action,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var probe ProbePlusTrialActivityOutput
	setWorkflowProgress(ctx, progress, stepProbePlusTrial)
	if err := workflow.ExecuteActivity(atomicCtx, probePlusTrialActivityName, ProbePlusTrialActivityInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
	}).Get(ctx, &probe); err != nil {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, err, map[string]any{"probe_plus_trial": protoDataMap(probe.GetData())}), nil
	}
	if !probe.GetChecked() {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), map[string]any{"probe_plus_trial": protoDataMap(probe.GetData())}), nil
	}
	if !probe.GetPlusTrialEligible() && !probe.GetPlusActive() {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), map[string]any{"probe_plus_trial": protoDataMap(probe.GetData())}), nil
	}

	setWorkflowProgress(ctx, progress, stepGoPayAppLogin)
	logon, err := runGoPayAppAuth(ctx, gopayCtx, retryCtx, input.GetJobId(), goPayAppOTPOptions{})
	if err != nil {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, err, map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData())}), nil
	}
	if !logon.GetAccountTokenReady() {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after login"), map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData())}), nil
	}

	var payment GoPayActivityOutput
	setWorkflowProgress(ctx, progress, stepGoPayPayment)
	payment, err = runGoPayPayment(ctx, paymentCtx, retryCtx, GoPayActivityInput{
		JobId:             input.GetJobId(),
		AccountId:         account.GetAccountId(),
		UseAccountToken:   logon.GetAccountTokenReady(),
		CheckoutUrl:       probe.GetCheckoutUrl(),
		CheckoutSessionId: probe.GetCheckoutSessionId(),
		StateJson:         logon.GetStateJson(),
	})
	if err != nil {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPayment, statusFailedRetryable, false, true, err, map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData()), "gopay_payment": protoDataMap(payment.GetData())}), nil
	}

	if err := workflow.ExecuteActivity(retryCtx, persistActivatedActivityName, PersistActivatedInput{
		AccountId:         account.GetAccountId(),
		ChargeRef:         payment.GetChargeRef(),
		PlusTrialEligible: payment.GetPlusTrialEligible(),
		PlusTrialChecked:  payment.GetPlusTrialChecked(),
		PlusActive:        payment.GetPlusActive(),
	}).Get(ctx, nil); err != nil {
		return failActivateWorkflow(ctx, retryCtx, result, input.GetJobId(), "", statusFailedRecoverable, true, false, err, protoDataMap(payment.GetData())), nil
	}

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData()), "gopay_payment": protoDataMap(payment.GetData())}),
	}).Get(ctx, nil)

	result.Success = true
	result.ChargeRef = payment.GetChargeRef()
	result.SnapToken = payment.GetSnapToken()
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}
func AutoPayWorkflow(ctx workflow.Context, input AutoPayWorkflowInput) (AutoPayWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "AutoPayWorkflow", input.GetJobId())
	result := AutoPayWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(15*time.Minute))
	gopayCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(5*time.Minute))
	paymentCtx := workflow.WithActivityOptions(ctx, paymentActivityOptions())
	tierCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))

	var account AccountRef
	setWorkflowProgress(ctx, progress, "resolve_account")
	if err := workflow.ExecuteActivity(retryCtx, resolveAccountActivityName, ResolveAccountInput{
		AccountId:   input.GetAccountId(),
		SourceJobId: input.GetSourceJobId(),
	}).Get(ctx, &account); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
		Action:    actionAutopay,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var probe ProbePlusTrialActivityOutput
	setWorkflowProgress(ctx, progress, stepProbePlusTrial)
	if err := workflow.ExecuteActivity(atomicCtx, probePlusTrialActivityName, ProbePlusTrialActivityInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
	}).Get(ctx, &probe); err != nil {
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, err, map[string]any{"probe_plus_trial": protoDataMap(probe.GetData())}), nil
	}
	if !probe.GetChecked() {
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), map[string]any{"probe_plus_trial": protoDataMap(probe.GetData())}), nil
	}
	if !probe.GetPlusTrialEligible() {
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), map[string]any{"probe_plus_trial": protoDataMap(probe.GetData())}), nil
	}

	setWorkflowProgress(ctx, progress, stepGoPayAppLogin)
	logon, err := runGoPayAppAuth(ctx, gopayCtx, retryCtx, input.GetJobId(), goPayAppOTPOptions{})
	if err != nil {
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, err, map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData())}), nil
	}
	if !logon.GetAccountTokenReady() {
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after login"), map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData())}), nil
	}

	var payment GoPayActivityOutput
	setWorkflowProgress(ctx, progress, stepGoPayPayment)
	payment, err = runGoPayPayment(ctx, paymentCtx, retryCtx, GoPayActivityInput{
		JobId:             input.GetJobId(),
		AccountId:         account.GetAccountId(),
		UseAccountToken:   logon.GetAccountTokenReady(),
		Tokenization:      "true",
		CheckoutUrl:       probe.GetCheckoutUrl(),
		CheckoutSessionId: probe.GetCheckoutSessionId(),
		StateJson:         logon.GetStateJson(),
	})
	if err != nil {
		combined := map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_payment": protoDataMap(payment.GetData())}
		combined["gopay_login"] = protoDataMap(logon.GetData())
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPayment, statusFailedRetryable, false, true, err, combined), nil
	}

	combined := map[string]any{"probe_plus_trial": protoDataMap(probe.GetData()), "gopay_login": protoDataMap(logon.GetData()), "gopay_payment": protoDataMap(payment.GetData())}

	var tier ProbeTierActivityOutput
	setWorkflowProgress(ctx, progress, stepProbeTier)
	if err := workflow.ExecuteActivity(tierCtx, probeTierActivityName, ProbeTierActivityInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
	}).Get(ctx, &tier); err != nil {
		combined["probe_tier"] = protoDataMap(tier.GetData())
		return failAutoPayWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbeTier, statusFailedRecoverable, true, false, err, combined), nil
	}
	combined["probe_tier"] = protoDataMap(tier.GetData())

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = true
	result.ChargeRef = payment.GetChargeRef()
	result.SnapToken = payment.GetSnapToken()
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}
func ProbeAccountWorkflow(ctx workflow.Context, input ProbeAccountWorkflowInput) (ProbeAccountWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "ProbeAccountWorkflow", input.GetJobId())
	result := ProbeAccountWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	plusTrialCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(5*time.Minute))
	tierCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))

	var account AccountRef
	setWorkflowProgress(ctx, progress, "resolve_account")
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
		Action:    actionProbeAccount,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	combined := map[string]any{
		"account_id":               account.GetAccountId(),
		"plus_trial_already_known": account.GetPlusTrialKnown(),
		"tier":                     account.GetTier(),
		"plus_active":              account.GetPlusActive(),
	}
	plusTrialSuccess := true
	plusTrialSkipped := shouldSkipPlusTrialProbe(account)
	var plusTrial ProbePlusTrialActivityOutput
	if plusTrialSkipped {
		combined["probe_plus_trial"] = skippedPlusTrialProbeData(account)
		result.PlusActive = account.GetPlusActive()
		result.PlanType = account.GetTier()
	} else {
		setWorkflowProgress(ctx, progress, stepProbePlusTrial)
		if err := workflow.ExecuteActivity(plusTrialCtx, probePlusTrialActivityName, ProbePlusTrialActivityInput{
			JobId:     input.GetJobId(),
			AccountId: account.GetAccountId(),
		}).Get(ctx, &plusTrial); err != nil {
			combined["probe_plus_trial"] = protoDataMap(plusTrial.GetData())
			return failProbeAccountWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbePlusTrial, statusFailedRetryable, false, true, err, combined), nil
		}
		plusTrialSuccess = plusTrial.GetSuccess()
		result.PlusTrialChecked = plusTrial.GetChecked()
		result.PlusTrialEligible = plusTrial.GetPlusTrialEligible()
		result.PlusActive = plusTrial.GetPlusActive()
		result.Amount = plusTrial.GetAmount()
		result.Currency = plusTrial.GetCurrency()
		result.Source = plusTrial.GetSource()
		result.PlanType = plusTrial.GetPlanType()
		result.CheckoutUrl = plusTrial.GetCheckoutUrl()
		combined["probe_plus_trial"] = protoDataMap(plusTrial.GetData())
	}

	var tier ProbeTierActivityOutput
	setWorkflowProgress(ctx, progress, stepProbeTier)
	if err := workflow.ExecuteActivity(tierCtx, probeTierActivityName, ProbeTierActivityInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
	}).Get(ctx, &tier); err != nil {
		combined["probe_tier"] = protoDataMap(tier.GetData())
		return failProbeAccountWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbeTier, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["probe_tier"] = protoDataMap(tier.GetData())

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = tier.GetSuccess() && plusTrialSuccess
	result.TierChecked = tier.GetChecked()
	result.Tier = tier.GetTier()
	if plusTrialSkipped && tier.GetChecked() {
		result.PlusActive = tier.GetPlusActive()
	} else if tier.GetPlusActive() {
		result.PlusActive = true
	}
	if tier.GetSource() != "" {
		result.Source = tier.GetSource()
	}
	result.ErrorMessage = tier.GetErrorMessage()
	if result.GetErrorMessage() == "" {
		setWorkflowProgressSucceeded(ctx, progress)
	}
	return result, nil
}
func shouldSkipPlusTrialProbe(account AccountRef) bool {
	return account.GetPlusActive() || normalizeTier(account.GetTier()) == "plus"
}
func skippedPlusTrialProbeData(account AccountRef) map[string]any {
	reason := "plus_active"
	if normalizeTier(account.GetTier()) == "plus" {
		reason = "tier_plus"
	}
	return map[string]any{
		"skipped":     true,
		"reason":      reason,
		"tier":        account.GetTier(),
		"plus_active": account.GetPlusActive(),
	}
}
func activationPaymentData(logonData map[string]any, paymentData map[string]any) map[string]any {
	if logonData == nil {
		return paymentData
	}
	return map[string]any{
		"ensure_logon":  logonData,
		"gopay_payment": paymentData,
	}
}

func prepareGoPayPayment(ctx workflow.Context, paymentCtx workflow.Context, input GoPayActivityInput) (GoPayPaymentPrepareOutput, error) {
	var prepare GoPayPaymentPrepareOutput
	err := workflow.ExecuteActivity(paymentCtx, goPayPaymentPrepareActivityName, input).Get(ctx, &prepare)
	return prepare, err
}

func cancelGoPayPayment(ctx workflow.Context, cancelCtx workflow.Context, flowID string) {
	if strings.TrimSpace(flowID) == "" {
		return
	}
	_ = workflow.ExecuteActivity(cancelCtx, goPayPaymentCancelActivityName, GoPayPaymentCancelInput{FlowId: flowID}).Get(ctx, nil)
}

func resendGoPayPaymentOTP(ctx workflow.Context, paymentCtx workflow.Context, start GoPayPaymentStartOutput, input GoPayActivityInput) (GoPayPaymentStartOutput, error) {
	var resend GoPayPaymentOTPResendOutput
	err := workflow.ExecuteActivity(paymentCtx, goPayPaymentOTPResendActivityName, GoPayPaymentOTPResendInput{
		JobId:     input.GetJobId(),
		AccountId: input.GetAccountId(),
		FlowId:    start.GetFlowId(),
		Data:      start.GetData(),
	}).Get(ctx, &resend)
	if resend.GetData() != nil {
		start.Data = resend.GetData()
	}
	if resend.GetIssuedAfterUnix() > 0 {
		start.IssuedAfterUnix = resend.GetIssuedAfterUnix()
	}
	return start, err
}

func requestGoPayPaymentSMSCode(ctx workflow.Context, activityCtx workflow.Context, input GoPayActivityInput, reason string) error {
	if normalizeGoPayOTPChannel(input.GetOtpChannel()) != "sms" || strings.TrimSpace(input.GetSmsActivationId()) == "" {
		return nil
	}
	var requested GoPayAppSMSActivationOutput
	return workflow.ExecuteActivity(activityCtx, goPayAppSMSRequestAdditionalCodeActivityName, GoPayAppSMSActivationInput{
		JobId:        input.GetJobId(),
		ActivationId: input.GetSmsActivationId(),
		Reason:       reason,
	}).Get(ctx, &requested)
}

func waitForGoPayPaymentOTP(ctx workflow.Context, input GoPayActivityInput, timeoutSeconds int32, issuedAfterUnix int64) (OTPWaitOutput, error) {
	waitInput := OTPWaitInput{
		JobId:            input.GetJobId(),
		StepName:         stepGoPayPayment,
		TimeoutSeconds:   timeoutSeconds,
		IssuedAfterUnix:  issuedAfterUnix,
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
	if normalizeGoPayOTPChannel(input.GetOtpChannel()) == "sms" && strings.TrimSpace(input.GetSmsActivationId()) != "" {
		waitInput.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: input.GetSmsActivationId()}}
	} else {
		source := strings.TrimSpace(input.GetUserId())
		if source == "" {
			source = goPayLocalSource
		}
		waitInput.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: source}}
	}
	otp, err := waitForOTP(ctx, waitInput)
	if err != nil {
		return otp, err
	}
	if !otp.GetFound() {
		return otp, goPayPaymentOTPNotReceivedError(timeoutSeconds, otp)
	}
	return otp, nil
}

func goPayPaymentOTPNotReceivedError(timeoutSeconds int32, wait OTPWaitOutput) error {
	reason := strings.TrimSpace(wait.GetErrorMessage())
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("payment otp not received after %ds: %s", timeoutSeconds, reason)
}

func shouldRetryGoPayPaymentOTP(input GoPayActivityInput, err error) bool {
	return normalizeGoPayOTPChannel(input.GetOtpChannel()) == "sms" && isOTPWaitNotReceivedError(err)
}

func runGoPayPayment(ctx workflow.Context, paymentCtx workflow.Context, cancelCtx workflow.Context, input GoPayActivityInput) (GoPayActivityOutput, error) {
	var start GoPayPaymentStartOutput
	if err := workflow.ExecuteActivity(paymentCtx, goPayPaymentStartActivityName, input).Get(ctx, &start); err != nil {
		cancelGoPayPayment(ctx, cancelCtx, paymentFlowID(start.GetFlowId(), input.GetPreparedFlowId()))
		return GoPayActivityOutput{Data: start.GetData(), StateJson: start.GetStateJson()}, err
	}

	otpSource := "not_required"
	var err error
	if start.GetOtpRequired() {
		if err := requestGoPayPaymentSMSCode(ctx, paymentCtx, input, stepGoPayPayment); err != nil {
			cancelGoPayPayment(ctx, cancelCtx, start.GetFlowId())
			return GoPayActivityOutput{Data: start.GetData(), StateJson: start.GetStateJson()}, err
		}
		otp, err := waitForGoPayPaymentOTP(ctx, input, start.GetOtpTimeoutSeconds(), start.GetIssuedAfterUnix())
		if shouldRetryGoPayPaymentOTP(input, err) {
			start, err = resendGoPayPaymentOTP(ctx, paymentCtx, start, input)
			if err == nil {
				err = requestGoPayPaymentSMSCode(ctx, paymentCtx, input, stepGoPayPayment+"_retry")
			}
			if err == nil {
				otp, err = waitForGoPayPaymentOTP(ctx, input, start.GetOtpTimeoutSeconds(), start.GetIssuedAfterUnix())
			}
		}
		if err != nil {
			cancelGoPayPayment(ctx, cancelCtx, start.GetFlowId())
			return GoPayActivityOutput{Data: start.GetData(), StateJson: start.GetStateJson()}, err
		}
		otpSource = otp.GetSource()
	}

	var payment GoPayActivityOutput
	err = workflow.ExecuteActivity(paymentCtx, goPayPaymentCompleteActivityName, GoPayPaymentCompleteInput{
		JobId:              input.GetJobId(),
		AccountId:          input.GetAccountId(),
		FlowId:             start.GetFlowId(),
		OtpParam:           paymentOTPParam,
		SubmittedAtParam:   paymentOTPSubmittedAtParam,
		OtpIssuedAfterUnix: start.GetIssuedAfterUnix(),
		OtpSource:          otpSource,
		UseAccountToken:    start.GetUseAccountToken(),
		Data:               start.GetData(),
		StateJson:          start.GetStateJson(),
	}).Get(ctx, &payment)
	return payment, err
}

func paymentFlowID(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
