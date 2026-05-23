package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func goPayAppBackedWAPaymentWorkflow(ctx workflow.Context, input GoPayPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
	return runGoPayAppBackedWAPaymentWorkflow(ctx, input)
}

func GoPayQRISPaymentActivateWorkflow(ctx workflow.Context, input GoPayPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
	return runGoPayQRISPaymentOnlyWorkflow(ctx, input)
}

func runGoPayQRISPaymentOnlyWorkflow(ctx workflow.Context, input GoPayPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "GoPayQRISPaymentActivateWorkflow", input.GetJobId())
	result := GoPayPaymentWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(15*time.Minute))
	paymentCtx := workflow.WithActivityOptions(ctx, paymentActivityOptions())
	tierCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	stateJSON := "{}"
	combined := map[string]any{
		"action":                actionGoPayQRISPaymentActivate,
		"activation_mode":       "qris_payment",
		"payment_type":          "qris",
		"tokenization":          "qris",
		"otp_channel":           "not_required",
		"uses_wa":               false,
		"uses_gopay_app_flow":   false,
		"uses_gopay_app_token":  false,
		"manual_confirmation":   true,
		"manual_payment_button": true,
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
	combined["account_id"] = account.GetAccountId()

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
		Action:    actionGoPayQRISPaymentActivate,
		Params: map[string]string{
			"activation_mode":       "qris_payment",
			"payment_type":          "qris",
			"tokenization":          "qris",
			"otp_channel":           "not_required",
			"uses_wa":               "false",
			"uses_gopay_app_flow":   "false",
			"manual_confirmation":   "true",
			"manual_payment_button": "true",
		},
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

	var paymentPrepare GoPayPaymentPrepareOutput
	setWorkflowProgress(ctx, progress, stepGoPayPaymentPrepare)
	paymentPrepare, err := prepareGoPayPayment(ctx, paymentCtx, GoPayActivityInput{
		JobId:           input.GetJobId(),
		AccountId:       account.GetAccountId(),
		UseAccountToken: false,
		Tokenization:    "qris",
		StateJson:       stateJSON,
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
		AccountId:       account.GetAccountId(),
		UseAccountToken: false,
		Tokenization:    "qris",
		PreparedFlowId:  paymentPrepare.GetFlowId(),
		StateJson:       stateJSON,
	})
	stateJSON = payment.GetStateJson()
	combined["gopay_payment"] = protoDataMap(payment.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPayment, statusFailedRetryable, false, true, err, combined), nil
	}
	result.ChargeRef = payment.GetChargeRef()
	result.SnapToken = payment.GetSnapToken()
	combined["payment_completed"] = true
	combined["charge_ref"] = result.GetChargeRef()
	combined["snap_token"] = result.GetSnapToken()

	var tier ProbeTierActivityOutput
	setWorkflowProgress(ctx, progress, stepProbeTier)
	if err := workflow.ExecuteActivity(tierCtx, probeTierActivityName, ProbeTierActivityInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
	}).Get(ctx, &tier); err != nil {
		combined["probe_tier"] = protoDataMap(tier.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbeTier, statusFailedRecoverable, true, false, err, combined), nil
	}
	combined["probe_tier"] = protoDataMap(tier.GetData())

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = true
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func runGoPayAppBackedWAPaymentWorkflow(ctx workflow.Context, input GoPayPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "GoPayPaymentWorkflow", input.GetJobId())
	result := GoPayPaymentWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	atomicCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(15*time.Minute))
	gopayCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	paymentCtx := workflow.WithActivityOptions(ctx, paymentActivityOptions())
	tierCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(2*time.Minute))
	userID := strings.TrimSpace(input.GetUserId())
	if userID == "" {
		userID = goPayLocalSource
	}
	paymentTokenization := "true"
	stateReasonPrefix := "wa_payment"
	result.UserId = userID
	addBalance := input.GetAddBalance()
	addBalanceMethod := goPayAddBalanceMethod(addBalance)
	stateJSON := "{}"
	combined := map[string]any{
		"otp_channel":        "wa",
		"user_id":            userID,
		"add_balance_method": addBalanceMethod,
		"action":             actionGoPayPayment,
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
	combined["account_id"] = account.GetAccountId()

	setWorkflowProgress(ctx, progress, "create_job")
	params := map[string]string{
		"otp_channel":        "wa",
		"user_id":            userID,
		"add_balance_method": addBalanceMethod,
	}
	if strings.TrimSpace(input.GetWaPhone()) != "" {
		params["wa_phone"] = strings.TrimSpace(input.GetWaPhone())
	}
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
		Action:    actionGoPayPayment,
		Params:    params,
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
	combined["wa_phone"] = result.GetWaPhone()
	combined["wa_phone_resolution"] = protoDataMap(waPhone.GetData())

	var stored GoPayAppStateActivityOutput
	setWorkflowProgress(ctx, progress, "load_gopay_state")
	if err := workflow.ExecuteActivity(retryCtx, goPayAppLoadStateActivityName, GoPayAppStateActivityInput{
		JobId:  input.GetJobId(),
		UserId: userID,
		Reason: stateReasonPrefix + "_start",
	}).Get(ctx, &stored); err != nil {
		combined["load_state"] = protoDataMap(stored.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), "load_gopay_state", statusFailedRetryable, false, true, err, combined), nil
	}
	stateJSON = strings.TrimSpace(stored.GetStateJson())
	if stateJSON == "" {
		stateJSON = "{}"
	}
	combined["load_state"] = protoDataMap(stored.GetData())

	otpOpts := goPayAppOTPOptions{
		Phone:       result.GetWaPhone(),
		OTPChannel:  "wa",
		Source:      userID,
		StateJSON:   stateJSON,
		Pin:         input.GetPin(),
		CountryCode: input.GetCountryCode(),
	}

	setWorkflowProgress(ctx, progress, stepGoPayAppLogin)
	token, err := runGoPayAppEnsureTokenChild(ctx, input.GetJobId(), otpOpts)
	stateJSON = token.GetStateJson()
	combined["ensure_token_available"] = protoDataMap(token.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, err, combined), nil
	}
	result.SignupComplete = token.GetSignupComplete()
	result.AccountTokenReady = token.GetAccountTokenReady()
	if err := workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
		JobId:     input.GetJobId(),
		UserId:    userID,
		StateJson: stateJSON,
		Reason:    stateReasonPrefix + "_token_available",
	}).Get(ctx, nil); err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, err, combined), nil
	}

	setWorkflowProgress(ctx, progress, stepGoPayAppEnsurePINSetup)
	otpOpts.StateJSON = stateJSON
	pin, err := runGoPayAppEnsurePINSetupChild(ctx, input.GetJobId(), otpOpts)
	stateJSON = pin.GetStateJson()
	combined["ensure_pin_setup"] = protoDataMap(pin.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppEnsurePINSetup, statusFailedRetryable, false, true, err, combined), nil
	}
	result.SignupPinComplete = pin.GetSignupPinComplete()
	result.AccountTokenReady = pin.GetAccountTokenReady()
	if !pin.GetAccountTokenReady() {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppEnsurePINSetup, statusFailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after ensure pin setup"), combined), nil
	}
	if err := workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
		JobId:     input.GetJobId(),
		UserId:    userID,
		StateJson: stateJSON,
		Reason:    stateReasonPrefix + "_pin_setup",
	}).Get(ctx, nil); err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppEnsurePINSetup, statusFailedRetryable, false, true, err, combined), nil
	}

	if addBalance == nil {
		setWorkflowProgress(ctx, progress, stepGoPayAppAddBalance)
		selectedAddBalance, nextStateJSON, balanceData, balanceReady, err := waitForGoPayAddBalanceSelection(ctx, retryCtx, input.GetJobId(), stateJSON, input.GetAddBalanceConfirmTimeoutSeconds())
		stateJSON = nextStateJSON
		if err != nil {
			combined["add_balance"] = map[string]any{
				"status":  "awaiting_selection",
				"methods": goPayAddBalanceMethodOptions(),
			}
			if len(balanceData) > 0 {
				combined["add_balance_check"] = balanceData
			}
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppAddBalance, statusFailedRetryable, false, true, err, combined), nil
		}
		if balanceReady {
			combined["add_balance"] = balanceData
			result.AddBalanceStatus = "balance_ready"
			result.AddBalanceComplete = true
		} else {
			addBalance = selectedAddBalance
			addBalanceMethod = goPayAddBalanceMethod(addBalance)
			combined["add_balance_method"] = addBalanceMethod
			result.AddBalanceMethod = addBalanceMethod
		}
	}

	if !result.GetAddBalanceComplete() {
		var balance GoPayAppAddBalanceOutput
		setWorkflowProgress(ctx, progress, stepGoPayAppAddBalance)
		addBalanceCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 5 * time.Minute,
			RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 1},
		})
		if err := workflow.ExecuteActivity(addBalanceCtx, goPayAppAddBalanceActivityName, GoPayAppAddBalanceInput{
			JobId:       input.GetJobId(),
			StateJson:   stateJSON,
			AddBalance:  addBalance,
			TargetPhone: result.GetWaPhone(),
		}).Get(ctx, &balance); err != nil {
			stateJSON = balance.GetStateJson()
			combined["add_balance"] = protoDataMap(balance.GetData())
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppAddBalance, statusFailedRetryable, false, true, err, combined), nil
		}
		stateJSON = balance.GetStateJson()
		combined["add_balance"] = protoDataMap(balance.GetData())
		result.AddBalanceMethod = balance.GetMethod()
		result.AddBalanceStatus = balance.GetStatus()
		if err := workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
			JobId:     input.GetJobId(),
			UserId:    userID,
			StateJson: stateJSON,
			Reason:    stateReasonPrefix + "_add_balance",
		}).Get(ctx, nil); err != nil {
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppAddBalance, statusFailedRetryable, false, true, err, combined), nil
		}
		if balance.GetMethod() == "manual_transfer" {
			setWorkflowProgress(ctx, progress, stepGoPayAppAddBalanceConfirm)
			nextStateJSON, confirmation, err := waitForManualAddBalance(ctx, retryCtx, input.GetJobId(), stateJSON, input.GetAddBalanceConfirmTimeoutSeconds())
			stateJSON = nextStateJSON
			if err != nil {
				combined["add_balance_confirmation"] = map[string]any{
					"confirmed": false,
					"method":    "manual_transfer",
				}
				if len(confirmation) > 0 {
					combined["add_balance_check"] = confirmation
				}
				return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppAddBalanceConfirm, statusFailedRetryable, false, true, err, combined), nil
			}
			combined["add_balance_confirmation"] = confirmation
			result.AddBalanceStatus = "confirmed"
		}
		result.AddBalanceComplete = true
	}

	var paymentPrepare GoPayPaymentPrepareOutput
	setWorkflowProgress(ctx, progress, stepGoPayPaymentPrepare)
	paymentPrepare, err = prepareGoPayPayment(ctx, paymentCtx, GoPayActivityInput{
		JobId:           input.GetJobId(),
		AccountId:       account.GetAccountId(),
		UseAccountToken: false,
		Tokenization:    paymentTokenization,
		GopayPhone:      result.GetWaPhone(),
		UserId:          userID,
		StateJson:       stateJSON,
		Pin:             input.GetPin(),
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
		AccountId:       account.GetAccountId(),
		UseAccountToken: true,
		Tokenization:    paymentTokenization,
		PreparedFlowId:  paymentPrepare.GetFlowId(),
		GopayPhone:      result.GetWaPhone(),
		OtpChannel:      "wa",
		UserId:          userID,
		StateJson:       stateJSON,
		Pin:             input.GetPin(),
		CountryCode:     input.GetCountryCode(),
	})
	stateJSON = payment.GetStateJson()
	combined["gopay_payment"] = protoDataMap(payment.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPayment, statusFailedRetryable, false, true, err, combined), nil
	}
	result.ChargeRef = payment.GetChargeRef()
	result.SnapToken = payment.GetSnapToken()
	combined["payment_completed"] = true
	combined["charge_ref"] = result.GetChargeRef()
	combined["snap_token"] = result.GetSnapToken()
	_ = workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
		JobId:     input.GetJobId(),
		UserId:    userID,
		StateJson: stateJSON,
		Reason:    stateReasonPrefix + "_completed",
	}).Get(ctx, nil)
	combined["rebind_required"] = true

	var tier ProbeTierActivityOutput
	setWorkflowProgress(ctx, progress, stepProbeTier)
	if err := workflow.ExecuteActivity(tierCtx, probeTierActivityName, ProbeTierActivityInput{
		JobId:     input.GetJobId(),
		AccountId: account.GetAccountId(),
	}).Get(ctx, &tier); err != nil {
		combined["probe_tier"] = protoDataMap(tier.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepProbeTier, statusFailedRecoverable, true, false, err, combined), nil
	}
	combined["probe_tier"] = protoDataMap(tier.GetData())

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)
	startGoPayPaymentRebindSideEffect(ctx, input.GetJobId(), account.GetAccountId(), userID, input.GetPin(), input.GetCountryCode(), combined)
	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = true
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}
