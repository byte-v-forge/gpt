package workflows

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func goPaySMSPaymentWorkflow(ctx workflow.Context, input GoPayPaymentWorkflowInput) (GoPayPaymentWorkflowResult, error) {
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

	otpChannel := normalizeGoPayOTPChannel(input.GetOtpChannel())
	if otpChannel == "" {
		otpChannel = "sms"
	}
	userID := strings.TrimSpace(input.GetUserId())
	if userID == "" {
		userID = goPayLocalSource
	}
	result.UserId = userID
	addBalance := input.GetAddBalance()
	addBalanceMethod := goPayAddBalanceMethod(addBalance)
	stateJSON := "{}"
	combined := map[string]any{
		"otp_channel":        otpChannel,
		"user_id":            userID,
		"add_balance_method": addBalanceMethod,
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
		Action:    actionGoPayPayment,
		Params: map[string]string{
			"otp_channel":        otpChannel,
			"add_balance_method": addBalanceMethod,
			"user_id":            userID,
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

	var otpOpts goPayAppOTPOptions
	signup := &GoPayAppStepOutput{}
	signupAttempts := []any{}
	maxSignupAttempts := goPayAppSignupMaxPhoneAttempts

	for attempt := 1; attempt <= maxSignupAttempts; attempt++ {
		attemptData := map[string]any{"attempt": attempt, "max_attempts": maxSignupAttempts}

		var phone GoPayAppAcquireSignupPhoneOutput
		setWorkflowProgress(ctx, progress, stepGoPayAppSignupPhone)
		if err := workflow.ExecuteActivity(gopayCtx, goPayAppAcquireSignupPhoneActivityName, GoPayAppAcquireSignupPhoneInput{
			JobId:        input.GetJobId(),
			FailureCount: int32(attempt - 1),
		}).Get(ctx, &phone); err != nil {
			attemptData["signup_phone"] = protoDataMap(phone.GetData())
			attemptData["error_message"] = err.Error()
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			combined["signup_phone"] = protoDataMap(phone.GetData())
			result.ActivationId = phone.GetActivationId()
			result.Phone = phone.GetPhone()
			if attempt < maxSignupAttempts && isGoPaySignupPhoneRotatableError(err) {
				continue
			}
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSignupPhone, statusFailedRetryable, false, true, err, combined), nil
		}
		attemptData["signup_phone"] = protoDataMap(phone.GetData())
		combined["signup_phone"] = protoDataMap(phone.GetData())
		result.ActivationId = phone.GetActivationId()
		result.Phone = phone.GetPhone()

		var deviceProxy GoPayAppGenerateDeviceProxyOutput
		setWorkflowProgress(ctx, progress, stepGoPayAppGenerateDeviceProxy)
		if err := workflow.ExecuteActivity(gopayCtx, goPayAppGenerateDeviceProxyActivityName, GoPayAppGenerateDeviceProxyInput{
			JobId: input.GetJobId(),
		}).Get(ctx, &deviceProxy); err != nil {
			attemptData["device_proxy"] = protoDataMap(deviceProxy.GetData())
			attemptData["error_message"] = err.Error()
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			combined["device_proxy"] = protoDataMap(deviceProxy.GetData())
			_ = workflow.ExecuteActivity(retryCtx, goPayAppDiscardSignupPhoneActivityName, GoPayAppSMSActivationInput{
				JobId:        input.GetJobId(),
				ActivationId: phone.GetActivationId(),
				FailureCount: int32(attempt),
				Reason:       err.Error(),
			}).Get(ctx, nil)
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppGenerateDeviceProxy, statusFailedRetryable, false, true, err, combined), nil
		}
		attemptData["device_proxy"] = protoDataMap(deviceProxy.GetData())
		combined["device_proxy"] = protoDataMap(deviceProxy.GetData())
		stateJSON = strings.TrimSpace(deviceProxy.GetStateJson())
		if stateJSON == "" {
			err := fmt.Errorf("generated device proxy state_json missing")
			attemptData["error_message"] = err.Error()
			var discarded GoPayAppSMSActivationOutput
			discardErr := workflow.ExecuteActivity(retryCtx, goPayAppDiscardSignupPhoneActivityName, GoPayAppSMSActivationInput{
				JobId:        input.GetJobId(),
				ActivationId: phone.GetActivationId(),
				FailureCount: int32(attempt),
				Reason:       err.Error(),
			}).Get(ctx, &discarded)
			attemptData["discard_signup_phone"] = protoDataMap(discarded.GetData())
			if discardErr != nil {
				attemptData["discard_error_message"] = discardErr.Error()
			}
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppGenerateDeviceProxy, statusFailedRetryable, false, true, err, combined), nil
		}

		var phoneCheck GoPayAppCheckSignupPhoneOutput
		setWorkflowProgress(ctx, progress, stepGoPayAppCheckPhone)
		if err := workflow.ExecuteActivity(gopayCtx, goPayAppCheckSignupPhoneActivityName, GoPayAppCheckSignupPhoneInput{
			JobId:        input.GetJobId(),
			ActivationId: phone.GetActivationId(),
			Phone:        phone.GetPhone(),
			CountryCode:  input.GetCountryCode(),
			StateJson:    stateJSON,
		}).Get(ctx, &phoneCheck); err != nil {
			attemptData["check_phone"] = protoDataMap(phoneCheck.GetData())
			attemptData["error_message"] = err.Error()
			var discarded GoPayAppSMSActivationOutput
			discardErr := workflow.ExecuteActivity(retryCtx, goPayAppDiscardSignupPhoneActivityName, GoPayAppSMSActivationInput{
				JobId:        input.GetJobId(),
				ActivationId: phone.GetActivationId(),
				FailureCount: int32(attempt),
				Reason:       err.Error(),
			}).Get(ctx, &discarded)
			attemptData["discard_signup_phone"] = protoDataMap(discarded.GetData())
			if discardErr != nil {
				attemptData["discard_error_message"] = discardErr.Error()
			}
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			combined["check_phone"] = protoDataMap(phoneCheck.GetData())
			if isGoPaySignupRateLimitError(err) {
				attemptData["rate_limit_scope"] = "gopay_signup_probe"
				combined["signup_attempts"] = signupAttempts
				return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppCheckPhone, statusFailedRetryable, false, true, err, combined), nil
			}
			if attempt < maxSignupAttempts && isGoPaySignupPhoneRotatableError(err) {
				continue
			}
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppCheckPhone, statusFailedRetryable, false, true, err, combined), nil
		}
		attemptData["check_phone"] = protoDataMap(phoneCheck.GetData())
		combined["check_phone"] = protoDataMap(phoneCheck.GetData())
		if !goPayAppDeviceProxyMatched(deviceProxy, phoneCheck) {
			err := fmt.Errorf("generated device proxy mismatch after check_phone")
			attemptData["error_message"] = err.Error()
			attemptData["expected_proxy_hash"] = deviceProxy.GetProxyHash()
			attemptData["actual_proxy_hash"] = phoneCheck.GetProxyHash()
			attemptData["expected_device_fingerprint"] = deviceProxy.GetDeviceFingerprint()
			attemptData["actual_device_fingerprint"] = phoneCheck.GetDeviceFingerprint()
			var discarded GoPayAppSMSActivationOutput
			discardErr := workflow.ExecuteActivity(retryCtx, goPayAppDiscardSignupPhoneActivityName, GoPayAppSMSActivationInput{
				JobId:        input.GetJobId(),
				ActivationId: phone.GetActivationId(),
				FailureCount: int32(attempt),
				Reason:       err.Error(),
			}).Get(ctx, &discarded)
			attemptData["discard_signup_phone"] = protoDataMap(discarded.GetData())
			if discardErr != nil {
				attemptData["discard_error_message"] = discardErr.Error()
			}
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppCheckPhone, statusFailedRetryable, false, true, err, combined), nil
		}
		stateJSON = strings.TrimSpace(phoneCheck.GetStateJson())
		if stateJSON == "" {
			stateJSON = strings.TrimSpace(deviceProxy.GetStateJson())
		}
		if stateJSON == "" {
			stateJSON = "{}"
		}

		otpOpts = goPayAppOTPOptions{
			Phone:           phone.GetPhone(),
			OTPChannel:      otpChannel,
			SMSActivationID: phone.GetActivationId(),
			Source:          userID,
			StateJSON:       stateJSON,
			Pin:             input.GetPin(),
			CountryCode:     input.GetCountryCode(),
		}

		preSignupProbeDelay := workflowJitterDuration(ctx, 8*time.Second, 25*time.Second)
		attemptData["pre_signup_delay_seconds"] = int(preSignupProbeDelay.Seconds())
		if err := workflow.Sleep(ctx, preSignupProbeDelay); err != nil {
			attemptData["error_message"] = err.Error()
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSignup, statusFailedRetryable, false, true, err, combined), nil
		}

		setWorkflowProgress(ctx, progress, stepGoPayAppSignup)
		currentSignup, err := runGoPayAppSignupChild(ctx, input.GetJobId(), otpOpts, attempt)
		signup = currentSignup
		stateJSON = signup.GetStateJson()
		attemptData["signup"] = protoDataMap(signup.GetData())
		if err == nil {
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			combined["signup"] = protoDataMap(signup.GetData())
			break
		}

		attemptData["error_message"] = err.Error()
		if isGoPaySignupRateLimitError(err) {
			var discarded GoPayAppSMSActivationOutput
			discardErr := workflow.ExecuteActivity(retryCtx, goPayAppDiscardSignupPhoneActivityName, GoPayAppSMSActivationInput{
				JobId:        input.GetJobId(),
				ActivationId: phone.GetActivationId(),
				FailureCount: int32(attempt),
				Reason:       err.Error(),
			}).Get(ctx, &discarded)
			attemptData["discard_signup_phone"] = protoDataMap(discarded.GetData())
			if discardErr != nil {
				attemptData["discard_error_message"] = discardErr.Error()
			}
			attemptData["rate_limit_scope"] = "gopay_signup_initiate"
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			combined["signup"] = protoDataMap(signup.GetData())
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSignup, statusFailedRetryable, false, true, err, combined), nil
		}
		if isGoPaySignupOTPNotReceived(err) || isGoPaySignupPhoneRotatableError(err) {
			var discarded GoPayAppSMSActivationOutput
			discardErr := workflow.ExecuteActivity(retryCtx, goPayAppDiscardSignupPhoneActivityName, GoPayAppSMSActivationInput{
				JobId:        input.GetJobId(),
				ActivationId: phone.GetActivationId(),
				FailureCount: int32(attempt),
				Reason:       err.Error(),
			}).Get(ctx, &discarded)
			attemptData["discard_signup_phone"] = protoDataMap(discarded.GetData())
			if discardErr != nil {
				attemptData["discard_error_message"] = discardErr.Error()
			}
			signupAttempts = append(signupAttempts, attemptData)
			combined["signup_attempts"] = signupAttempts
			combined["signup"] = protoDataMap(signup.GetData())
			if attempt < maxSignupAttempts {
				continue
			}
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSignup, statusFailedRetryable, false, true, err, combined), nil
		}

		signupAttempts = append(signupAttempts, attemptData)
		combined["signup_attempts"] = signupAttempts
		combined["signup"] = protoDataMap(signup.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSignup, statusFailedRetryable, false, true, err, combined), nil
	}
	result.SignupComplete = signup.GetSignupComplete()

	ensurePINBeforeAddBalance := workflow.GetVersion(ctx, "gopay-sms-ensure-pin-setup-before-add-balance", workflow.DefaultVersion, 1) != workflow.DefaultVersion
	pinSetupComplete := false
	if ensurePINBeforeAddBalance {
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
		pinSetupComplete = true
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
			TargetPhone: result.GetPhone(),
		}).Get(ctx, &balance); err != nil {
			stateJSON = balance.GetStateJson()
			combined["add_balance"] = protoDataMap(balance.GetData())
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppAddBalance, statusFailedRetryable, false, true, err, combined), nil
		}
		stateJSON = balance.GetStateJson()
		combined["add_balance"] = protoDataMap(balance.GetData())
		result.AddBalanceMethod = balance.GetMethod()
		result.AddBalanceStatus = balance.GetStatus()
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
	paymentPrepare, err := prepareGoPayPayment(ctx, paymentCtx, GoPayActivityInput{
		JobId:           input.GetJobId(),
		AccountId:       account.GetAccountId(),
		UseAccountToken: false,
		Tokenization:    "true",
		GopayPhone:      result.GetPhone(),
		StateJson:       stateJSON,
		CountryCode:     input.GetCountryCode(),
	})
	stateJSON = paymentPrepare.GetStateJson()
	combined["gopay_payment_prepare"] = protoDataMap(paymentPrepare.GetData())
	if err != nil {
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPaymentPrepare, statusFailedRetryable, false, true, err, combined), nil
	}

	if !pinSetupComplete {
		setWorkflowProgress(ctx, progress, stepGoPayAppEnsurePINSetup)
		otpOpts.StateJSON = stateJSON
		pin, err := runGoPayAppCreatePinChild(ctx, input.GetJobId(), otpOpts)
		stateJSON = pin.GetStateJson()
		combined["ensure_pin_setup"] = protoDataMap(pin.GetData())
		if err != nil {
			cancelGoPayPayment(ctx, retryCtx, paymentPrepare.GetFlowId())
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppEnsurePINSetup, statusFailedRetryable, false, true, err, combined), nil
		}
		result.SignupPinComplete = pin.GetSignupPinComplete()
		result.AccountTokenReady = pin.GetAccountTokenReady()
		if !pin.GetAccountTokenReady() {
			cancelGoPayPayment(ctx, retryCtx, paymentPrepare.GetFlowId())
			return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppEnsurePINSetup, statusFailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after ensure pin setup"), combined), nil
		}
	}

	var payment GoPayActivityOutput
	setWorkflowProgress(ctx, progress, stepGoPayPayment)
	payment, err = runGoPayPayment(ctx, paymentCtx, retryCtx, GoPayActivityInput{
		JobId:           input.GetJobId(),
		AccountId:       account.GetAccountId(),
		UseAccountToken: true,
		Tokenization:    "true",
		PreparedFlowId:  paymentPrepare.GetFlowId(),
		GopayPhone:      result.GetPhone(),
		OtpChannel:      otpChannel,
		SmsActivationId: otpOpts.SMSActivationID,
		UserId:          userID,
		StateJson:       stateJSON,
		Pin:             input.GetPin(),
		CountryCode:     input.GetCountryCode(),
	})
	stateJSON = payment.GetStateJson()
	if err != nil {
		combined["gopay_payment"] = protoDataMap(payment.GetData())
		return failGoPayPaymentWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayPayment, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["gopay_payment"] = protoDataMap(payment.GetData())

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
	result.ChargeRef = payment.GetChargeRef()
	result.SnapToken = payment.GetSnapToken()
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func workflowJitterDuration(ctx workflow.Context, minDelay, maxDelay time.Duration) time.Duration {
	if maxDelay <= minDelay {
		return minDelay
	}
	minSeconds := int(minDelay / time.Second)
	maxSeconds := int(maxDelay / time.Second)
	if maxSeconds <= minSeconds {
		return minDelay
	}
	var seconds int
	future := workflow.SideEffect(ctx, func(workflow.Context) any {
		span := maxSeconds - minSeconds + 1
		return minSeconds + int(time.Now().UnixNano()%int64(span))
	})
	if err := future.Get(&seconds); err != nil || seconds <= 0 {
		return minDelay
	}
	return time.Duration(seconds) * time.Second
}
