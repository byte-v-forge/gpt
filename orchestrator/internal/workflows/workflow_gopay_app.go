package workflows

import (
	"fmt"
	pb "orchestrator/pb"
	"strings"
	"time"

	"go.temporal.io/sdk/workflow"
)

const (
	goPayAppWorkflowOperationProvision = "provision"
	goPayAppWorkflowOperationLogin     = "login"
	goPayAppWorkflowOperationSignup    = "signup"
	goPayAppWorkflowOperationCreatePIN = "create_pin"
	goPayAppWorkflowOperationCheckBal  = "check_balance"
	goPayAppWorkflowOperationCheckPIN  = "check_pin"
	goPayAppWorkflowOperationChange    = "change_phone"
)

type goPayAppOTPOptions struct {
	Phone           string
	OTPChannel      string
	SMSActivationID string
	Source          string
	ResetState      bool
	StateJSON       string
	Pin             string
	CountryCode     string
}

func GoPayAppWorkflow(ctx workflow.Context, input GoPayAppWorkflowInput) (GoPayAppWorkflowResult, error) {
	operation := normalizeGoPayAppWorkflowOperation(input.GetOperation())
	if operation != goPayAppWorkflowOperationProvision {
		return runGoPayAppUserOperationWorkflow(ctx, input, operation)
	}

	progress := newWorkflowProgress(ctx, "GoPayAppWorkflow", input.GetJobId())
	result := GoPayAppWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()
	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:  input.GetJobId(),
		Action: actionGoPayApp,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	stateJSON := "{}"
	opts := goPayAppOTPOptions{
		Phone:       input.GetPhone(),
		OTPChannel:  input.GetOtpChannel(),
		Pin:         input.GetPin(),
		CountryCode: input.GetCountryCode(),
		StateJSON:   stateJSON,
	}
	combined := map[string]any{}
	setWorkflowProgress(ctx, progress, stepGoPayAppLogin)
	login, err := runGoPayAppAuthChild(ctx, input.GetJobId(), opts)
	stateJSON = login.GetStateJson()
	if err != nil {
		combined["login"] = protoDataMap(login.GetData())
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppLogin, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["login"] = protoDataMap(login.GetData())

	setWorkflowProgress(ctx, progress, stepGoPayAppChangePhone)
	opts.StateJSON = stateJSON
	changePhone, err := runGoPayAppChangePhoneChild(ctx, input.GetJobId(), stateJSON, opts)
	stateJSON = changePhone.GetStateJson()
	if err != nil {
		combined["change_phone"] = protoDataMap(changePhone.GetData())
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppChangePhone, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["change_phone"] = protoDataMap(changePhone.GetData())
	result.ActivationId = changePhone.GetActivationId()
	result.ChangePhoneComplete = changePhone.GetChangePhoneComplete()

	setWorkflowProgress(ctx, progress, stepGoPayAppDeactivate)
	opts.StateJSON = stateJSON
	deactivate, err := runGoPayAppDeactivateChild(ctx, input.GetJobId(), changePhone.GetActivationId(), stateJSON, opts)
	stateJSON = deactivate.GetStateJson()
	if err != nil {
		combined["deactivate"] = protoDataMap(deactivate.GetData())
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppDeactivate, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["deactivate"] = protoDataMap(deactivate.GetData())
	result.DeactivateComplete = deactivate.GetDeactivateComplete()

	setWorkflowProgress(ctx, progress, stepGoPayAppSignup)
	opts.StateJSON = stateJSON
	signup, err := runGoPayAppSignupChild(ctx, input.GetJobId(), opts, 0)
	stateJSON = signup.GetStateJson()
	if err != nil {
		combined["signup"] = protoDataMap(signup.GetData())
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppSignup, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["signup"] = protoDataMap(signup.GetData())
	result.SignupComplete = signup.GetSignupComplete()

	setWorkflowProgress(ctx, progress, stepGoPayAppCreatePin)
	opts.StateJSON = stateJSON
	createPin, err := runGoPayAppCreatePinChild(ctx, input.GetJobId(), opts)
	stateJSON = createPin.GetStateJson()
	if err != nil {
		combined["create_pin"] = protoDataMap(createPin.GetData())
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), stepGoPayAppCreatePin, statusFailedRetryable, false, true, err, combined), nil
	}
	combined["create_pin"] = protoDataMap(createPin.GetData())
	result.SignupPinComplete = createPin.GetSignupPinComplete()
	result.AccountTokenReady = createPin.GetAccountTokenReady()

	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)

	result.Success = true
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func runGoPayAppUserOperationWorkflow(ctx workflow.Context, input GoPayAppWorkflowInput, operation string) (GoPayAppWorkflowResult, error) {
	progress := newWorkflowProgress(ctx, "GoPayAppWorkflow", input.GetJobId())
	result := GoPayAppWorkflowResult{JobId: input.GetJobId()}
	defer func() {
		finishWorkflowProgressOnError(ctx, progress, result.GetErrorMessage())
	}()

	retryCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	userID := strings.TrimSpace(input.GetUserId())
	if userID == "" {
		userID = goPayLocalSource
	}
	params := map[string]string{
		"operation": operation,
		"user_id":   userID,
	}
	if phone := strings.TrimSpace(input.GetPhone()); phone != "" {
		params["phone"] = phone
	}

	setWorkflowProgress(ctx, progress, "create_job")
	if err := workflow.ExecuteActivity(retryCtx, createJobActivityName, CreateJobInput{
		JobId:  input.GetJobId(),
		Action: actionGoPayApp,
		Params: params,
	}).Get(ctx, nil); err != nil {
		result.ErrorMessage = err.Error()
		return result, nil
	}

	var stored GoPayAppStateActivityOutput
	setWorkflowProgress(ctx, progress, "load_gopay_state")
	if err := workflow.ExecuteActivity(retryCtx, goPayAppLoadStateActivityName, GoPayAppStateActivityInput{
		JobId:  input.GetJobId(),
		UserId: userID,
		Reason: "gopay_app_" + operation,
	}).Get(ctx, &stored); err != nil {
		combined := map[string]any{"load_state": protoDataMap(stored.GetData()), "operation": operation, "user_id": userID}
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), "load_gopay_state", statusFailedRetryable, false, true, err, combined), nil
	}

	stateJSON := strings.TrimSpace(stored.GetStateJson())
	if stateJSON == "" {
		stateJSON = "{}"
	}
	opts := goPayAppOTPOptions{
		Phone:       input.GetPhone(),
		OTPChannel:  input.GetOtpChannel(),
		Source:      userID,
		StateJSON:   stateJSON,
		Pin:         input.GetPin(),
		CountryCode: input.GetCountryCode(),
	}
	combined := map[string]any{"load_state": protoDataMap(stored.GetData()), "operation": operation, "user_id": userID}
	stepName := goPayAppWorkflowOperationStep(operation)
	setWorkflowProgress(ctx, progress, stepName)

	out, err := runGoPayAppWorkflowOperationChild(ctx, retryCtx, input.GetJobId(), operation, opts)
	combined[operation] = protoDataMap(out.GetData())
	if nextStateJSON := strings.TrimSpace(out.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if saveErr := workflow.ExecuteActivity(retryCtx, goPayAppSaveStateActivityName, GoPayAppStateActivityInput{
		JobId:     input.GetJobId(),
		UserId:    userID,
		StateJson: stateJSON,
		Reason:    "gopay_app_" + operation,
	}).Get(ctx, nil); saveErr != nil && err == nil {
		err = saveErr
	}
	if err != nil {
		return failGoPayAppWorkflow(ctx, retryCtx, result, input.GetJobId(), stepName, statusFailedRetryable, false, true, err, combined), nil
	}

	result.AccountTokenReady = out.GetAccountTokenReady()
	result.SignupComplete = out.GetSignupComplete()
	result.SignupPinComplete = out.GetSignupPinComplete()
	result.ChangePhoneComplete = out.GetChangePhoneComplete()
	result.ActivationId = out.GetActivationId()
	result.Success = true
	_ = workflow.ExecuteActivity(retryCtx, markJobSucceededActivityName, JobSuccessInput{
		JobId:  input.GetJobId(),
		Result: protoData(combined),
	}).Get(ctx, nil)
	setWorkflowProgressSucceeded(ctx, progress)
	return result, nil
}

func runGoPayAppWorkflowOperationChild(ctx workflow.Context, activityCtx workflow.Context, jobID string, operation string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	switch operation {
	case goPayAppWorkflowOperationLogin:
		return runGoPayAppEnsureTokenChild(ctx, jobID, opts)
	case goPayAppWorkflowOperationCheckBal:
		return runGoPayAppCheckBalance(ctx, activityCtx, jobID, opts)
	case goPayAppWorkflowOperationCheckPIN:
		return runGoPayAppCheckPIN(ctx, activityCtx, jobID, opts)
	case goPayAppWorkflowOperationSignup:
		return runGoPayAppSignupChild(ctx, jobID, opts, 0)
	case goPayAppWorkflowOperationCreatePIN:
		return runGoPayAppCreatePinChild(ctx, jobID, opts)
	case goPayAppWorkflowOperationChange:
		return runGoPayAppUserChangePhone(ctx, jobID, opts)
	default:
		return &GoPayAppStepOutput{}, fmt.Errorf("unsupported gopay app operation: %s", operation)
	}
}

func runGoPayAppCheckBalance(ctx workflow.Context, activityCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	token, err := runGoPayAppEnsureTokenChild(ctx, jobID, opts)
	if err != nil {
		return token, err
	}
	stateJSON := strings.TrimSpace(token.GetStateJson())
	if stateJSON == "" {
		stateJSON = opts.StateJSON
	}
	var status GoPayAppStepOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppStatusActivityName, GoPayAppStepInput{
		JobId:     jobID,
		StateJson: stateJSON,
	}).Get(ctx, &status); err != nil {
		return &status, err
	}
	statusData := protoDataMap(status.GetData())
	combined := map[string]any{
		"ensure_token": protoDataMap(token.GetData()),
		"status":       statusData,
	}
	if snapshot, ok := statusData["status"].(map[string]any); ok {
		for _, key := range []string{"balance_amount", "balance_currency", "has_min_balance"} {
			if value, ok := snapshot[key]; ok {
				combined[key] = value
			}
		}
	}
	status.Data = protoData(combined)
	status.AccountTokenReady = token.GetAccountTokenReady() || status.GetAccountTokenReady()
	return &status, nil
}

func runGoPayAppCheckPIN(ctx workflow.Context, activityCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	token, err := runGoPayAppEnsureTokenChild(ctx, jobID, opts)
	if err != nil {
		return token, err
	}
	stateJSON := strings.TrimSpace(token.GetStateJson())
	if stateJSON == "" {
		stateJSON = opts.StateJSON
	}
	var status GoPayAppStepOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppStatusActivityName, GoPayAppStepInput{
		JobId:     jobID,
		StateJson: stateJSON,
	}).Get(ctx, &status); err != nil {
		return &status, err
	}
	combined := map[string]any{
		"ensure_token": protoDataMap(token.GetData()),
		"status":       protoDataMap(status.GetData()),
		"pin_setup":    status.GetSignupPinComplete(),
	}
	status.Data = protoData(combined)
	status.AccountTokenReady = token.GetAccountTokenReady() || status.GetAccountTokenReady()
	return &status, nil
}

func normalizeGoPayAppWorkflowOperation(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", goPayAppWorkflowOperationProvision, "full":
		return goPayAppWorkflowOperationProvision
	case "auth", "logon", goPayAppWorkflowOperationLogin:
		return goPayAppWorkflowOperationLogin
	case "balance", "check-balance", goPayAppWorkflowOperationCheckBal:
		return goPayAppWorkflowOperationCheckBal
	case "check-pin", "pin_check", goPayAppWorkflowOperationCheckPIN:
		return goPayAppWorkflowOperationCheckPIN
	case "register", goPayAppWorkflowOperationSignup:
		return goPayAppWorkflowOperationSignup
	case "pin", "set_pin", "create-pin", goPayAppWorkflowOperationCreatePIN:
		return goPayAppWorkflowOperationCreatePIN
	case "change", "rebind", "change-phone", goPayAppWorkflowOperationChange:
		return goPayAppWorkflowOperationChange
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func goPayAppWorkflowOperationStep(operation string) string {
	switch operation {
	case goPayAppWorkflowOperationLogin:
		return stepGoPayAppLogin
	case goPayAppWorkflowOperationCheckBal, goPayAppWorkflowOperationCheckPIN:
		return stepGoPayAppLogin
	case goPayAppWorkflowOperationSignup:
		return stepGoPayAppSignup
	case goPayAppWorkflowOperationCreatePIN:
		return stepGoPayAppCreatePin
	case goPayAppWorkflowOperationChange:
		return stepGoPayAppChangePhone
	default:
		return "gopay_app_" + operation
	}
}

func runGoPayAppUserChangePhone(ctx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	combined := map[string]any{}
	token, err := runGoPayAppEnsureTokenChild(ctx, jobID, opts)
	combined["ensure_token"] = protoDataMap(token.GetData())
	stateJSON := strings.TrimSpace(token.GetStateJson())
	if stateJSON == "" {
		stateJSON = opts.StateJSON
	}
	if err != nil {
		token.Data = protoData(combined)
		return token, err
	}
	changeOpts := opts
	changeOpts.StateJSON = stateJSON
	change, err := runGoPayAppChangePhoneChild(ctx, jobID, stateJSON, changeOpts)
	combined["change_phone"] = protoDataMap(change.GetData())
	change.Data = protoData(combined)
	return change, err
}

func runGoPayAppChangePhone(ctx workflow.Context, activityCtx workflow.Context, jobID string, stateJSON string, pin string, countryCode string) (GoPayAppStepOutput, error) {
	var failureCount int32
	var last GoPayAppStepOutput
	for {
		var number GoPayAppChangePhoneGetNumberOutput
		if err := workflow.ExecuteActivity(activityCtx, goPayAppChangePhoneGetNumberActivityName, GoPayAppChangePhoneGetNumberInput{
			JobId:        jobID,
			FailureCount: failureCount,
		}).Get(ctx, &number); err != nil {
			return GoPayAppStepOutput{
				ActivationId: number.GetActivationId(),
				Phone:        number.GetPhone(),
				Data:         number.GetData(),
				StateJson:    stateJSON,
			}, err
		}
		failureCount = number.GetFailureCount()
		last = GoPayAppStepOutput{
			ActivationId: number.GetActivationId(),
			Phone:        number.GetPhone(),
			Data:         number.GetData(),
			StateJson:    stateJSON,
		}

		var start GoPayAppChangePhoneStartOutput
		if err := workflow.ExecuteActivity(activityCtx, goPayAppChangePhoneStartActivityName, GoPayAppChangePhoneStartInput{
			JobId:        jobID,
			FailureCount: failureCount,
			StateJson:    stateJSON,
			ActivationId: number.GetActivationId(),
			Phone:        number.GetPhone(),
			Pin:          pin,
			CountryCode:  countryCode,
		}).Get(ctx, &start); err != nil {
			return GoPayAppStepOutput{
				ActivationId: start.GetActivationId(),
				Phone:        start.GetPhone(),
				Data:         start.GetData(),
				StateJson:    start.GetStateJson(),
			}, err
		}
		stateJSON = start.GetStateJson()
		failureCount = start.GetFailureCount()
		last = GoPayAppStepOutput{
			ActivationId: start.GetActivationId(),
			Phone:        start.GetPhone(),
			Data:         start.GetData(),
			StateJson:    stateJSON,
		}
		if start.GetErrorMessage() != "" {
			var canceled GoPayAppSMSActivationOutput
			err := workflow.ExecuteActivity(activityCtx, goPayAppSMSCancelBeforeRotationActivityName, GoPayAppSMSActivationInput{
				JobId:        jobID,
				ActivationId: start.GetActivationId(),
				FailureCount: failureCount,
				Reason:       start.GetErrorMessage(),
			}).Get(ctx, &canceled)
			failureCount = canceled.GetFailureCount()
			last.ActivationId = canceled.GetActivationId()
			last.Data = canceled.GetData()
			last.StateJson = stateJSON
			if err != nil {
				return last, err
			}
			if start.GetRetryableFailure() {
				continue
			}
			return last, fmt.Errorf("%s", start.GetErrorMessage())
		}
		if start.GetRetryableFailure() {
			continue
		}

		for otpAttempt := int32(0); otpAttempt <= start.GetOtpRetryAttempts(); otpAttempt++ {
			issuedAfterUnix := workflow.Now(ctx).Unix()
			wait, err := waitForOTP(ctx, OTPWaitInput{
				JobId:            jobID,
				StepName:         stepGoPayAppChangePhoneSMSWait,
				Target:           &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: start.GetActivationId()}},
				TimeoutSeconds:   start.GetOtpTimeoutSeconds(),
				IssuedAfterUnix:  issuedAfterUnix,
				OtpParam:         paymentOTPParam,
				SubmittedAtParam: paymentOTPSubmittedAtParam,
			})
			if err != nil {
				_ = workflow.ExecuteActivity(activityCtx, goPayAppSMSCancelBeforeRotationActivityName, GoPayAppSMSActivationInput{
					JobId:        jobID,
					ActivationId: start.GetActivationId(),
					FailureCount: failureCount,
					Reason:       err.Error(),
				}).Get(ctx, nil)
				return last, err
			}
			if wait.GetFound() {
				var complete GoPayAppChangePhoneCompleteOutput
				err = workflow.ExecuteActivity(activityCtx, goPayAppChangePhoneCompleteActivityName, GoPayAppChangePhoneCompleteInput{
					JobId:            jobID,
					ActivationId:     start.GetActivationId(),
					Code:             wait.GetCode(),
					FailureCount:     failureCount,
					StateJson:        stateJSON,
					OtpParam:         paymentOTPParam,
					SubmittedAtParam: paymentOTPSubmittedAtParam,
					IssuedAfterUnix:  issuedAfterUnix,
				}).Get(ctx, &complete)
				last = goPayAppStepFromChangePhoneComplete(complete)
				stateJSON = last.GetStateJson()
				if err != nil {
					return last, err
				}
				failureCount = complete.GetFailureCount()
				if complete.GetChangePhoneComplete() {
					return last, nil
				}
				if complete.GetRetryableFailure() {
					break
				}
				return last, fmt.Errorf("gopay change phone did not complete")
			}

			if otpAttempt < start.GetOtpRetryAttempts() {
				var retry GoPayAppChangePhoneRetryOutput
				if err := workflow.ExecuteActivity(activityCtx, goPayAppChangePhoneRetryActivityName, GoPayAppChangePhoneRetryInput{
					JobId:        jobID,
					ActivationId: start.GetActivationId(),
					OtpAttempt:   otpAttempt + 1,
					StateJson:    stateJSON,
				}).Get(ctx, &retry); err != nil {
					_ = workflow.ExecuteActivity(activityCtx, goPayAppSMSCancelBeforeRotationActivityName, GoPayAppSMSActivationInput{
						JobId:        jobID,
						ActivationId: start.GetActivationId(),
						FailureCount: failureCount,
						Reason:       err.Error(),
					}).Get(ctx, nil)
					return last, err
				}
				stateJSON = retry.GetStateJson()
				if retry.GetOtpSent() {
					continue
				}
				if retry.GetErrorMessage() != "" {
					wait.ErrorMessage = "ChangePhoneRetry: " + retry.GetErrorMessage()
				}
			}

			reason := wait.GetErrorMessage()
			if reason == "" {
				reason = "WaitCode: otp not found"
			} else {
				reason = "WaitCode: " + reason
			}
			var canceled GoPayAppSMSActivationOutput
			err = workflow.ExecuteActivity(activityCtx, goPayAppSMSCancelBeforeRotationActivityName, GoPayAppSMSActivationInput{
				JobId:        jobID,
				ActivationId: start.GetActivationId(),
				FailureCount: failureCount,
				Reason:       reason,
			}).Get(ctx, &canceled)
			failureCount = canceled.GetFailureCount()
			last.ActivationId = canceled.GetActivationId()
			last.Data = canceled.GetData()
			last.StateJson = stateJSON
			if err != nil {
				return last, err
			}
			break
		}
	}
}
func goPayAppStepFromChangePhoneComplete(output GoPayAppChangePhoneCompleteOutput) GoPayAppStepOutput {
	return GoPayAppStepOutput{
		ActivationId:        output.GetActivationId(),
		Stage:               output.GetStage(),
		Phone:               output.GetPhone(),
		ChangePhoneComplete: output.GetChangePhoneComplete(),
		Data:                output.GetData(),
		StateJson:           output.GetStateJson(),
	}
}
func runGoPayAppDeactivate(ctx workflow.Context, activityCtx workflow.Context, jobID, activationID, stateJSON string, pin string) (GoPayAppStepOutput, error) {
	var start GoPayAppDeactivateStartOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppDeactivateStartActivityName, GoPayAppDeactivateStartInput{
		JobId:        jobID,
		ActivationId: activationID,
		StateJson:    stateJSON,
		Pin:          pin,
	}).Get(ctx, &start); err != nil {
		return GoPayAppStepOutput{ActivationId: activationID, Data: start.GetData(), StateJson: start.GetStateJson()}, err
	}
	stateJSON = start.GetStateJson()
	if !start.GetOtpRequired() {
		return GoPayAppStepOutput{ActivationId: activationID, Data: start.GetData(), StateJson: stateJSON}, fmt.Errorf("gopay deactivate did not request OTP")
	}

	issuedAfterUnix := workflow.Now(ctx).Unix()
	wait, err := waitForOTP(ctx, OTPWaitInput{
		JobId:            jobID,
		StepName:         stepGoPayAppDeactivateSMSWait,
		Target:           &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: activationID}},
		TimeoutSeconds:   start.GetTimeoutSeconds(),
		IssuedAfterUnix:  issuedAfterUnix,
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	})
	if err != nil {
		_ = workflow.ExecuteActivity(activityCtx, goPayAppSMSFinishActivityName, GoPayAppSMSActivationInput{
			JobId:        jobID,
			ActivationId: activationID,
			Reason:       err.Error(),
		}).Get(ctx, nil)
		return GoPayAppStepOutput{ActivationId: activationID, Data: wait.GetData(), StateJson: stateJSON}, err
	}
	if !wait.GetFound() {
		reason := wait.GetErrorMessage()
		if reason == "" {
			reason = "otp not found"
		}
		var finished GoPayAppSMSActivationOutput
		_ = workflow.ExecuteActivity(activityCtx, goPayAppSMSFinishActivityName, GoPayAppSMSActivationInput{
			JobId:        jobID,
			ActivationId: activationID,
			Reason:       "WaitCode deactivate: " + reason,
		}).Get(ctx, &finished)
		return GoPayAppStepOutput{ActivationId: activationID, Data: finished.GetData(), StateJson: stateJSON}, fmt.Errorf("WaitCode deactivate: %s", reason)
	}

	var complete GoPayAppDeactivateCompleteOutput
	err = workflow.ExecuteActivity(activityCtx, goPayAppDeactivateCompleteActivityName, GoPayAppDeactivateCompleteInput{
		JobId:            jobID,
		ActivationId:     activationID,
		Code:             wait.GetCode(),
		StateJson:        stateJSON,
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
		IssuedAfterUnix:  issuedAfterUnix,
	}).Get(ctx, &complete)
	return goPayAppStepFromDeactivateComplete(complete), err
}
func goPayAppStepFromDeactivateComplete(output GoPayAppDeactivateCompleteOutput) GoPayAppStepOutput {
	return GoPayAppStepOutput{
		ActivationId:       output.GetActivationId(),
		DeactivateComplete: output.GetDeactivateComplete(),
		Data:               output.GetData(),
		StateJson:          output.GetStateJson(),
	}
}
func runGoPayAppSignup(ctx workflow.Context, activityCtx workflow.Context, cancelCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (GoPayAppStepOutput, error) {
	var start GoPayAppOTPOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppOTPStartActivityName, GoPayAppOTPStartInput{
		JobId:           jobID,
		Operation:       goPayAppOTPOperationSignup,
		StepName:        stepGoPayAppSignup,
		Phone:           opts.Phone,
		OtpChannel:      opts.OTPChannel,
		SmsActivationId: opts.SMSActivationID,
		ResetState:      opts.ResetState,
		StateJson:       opts.StateJSON,
		Pin:             opts.Pin,
		CountryCode:     opts.CountryCode,
	}).Get(ctx, &start); err != nil {
		return goPayAppStepFromOTP(start), err
	}
	if start.GetReady() || start.GetAccountTokenReady() || start.GetSignupComplete() {
		return goPayAppStepFromOTP(start), nil
	}
	if !start.GetOtpRequired() {
		return goPayAppStepFromOTP(start), fmt.Errorf("gopay signup did not request OTP and did not complete")
	}

	startChannel := effectiveGoPayOTPChannel(start, opts.OTPChannel)
	otp, err := waitForOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppSignup, start, startChannel, opts.SMSActivationID, opts.Source))
	if err != nil {
		if !isOTPWaitNotReceivedError(err) {
			return goPayAppStepFromOTP(start), err
		}
		otp = OTPWaitOutput{ErrorMessage: err.Error()}
	}
	if !otp.GetFound() {
		var retry GoPayAppOTPOutput
		if err := workflow.ExecuteActivity(activityCtx, goPayAppOTPRetryActivityName, GoPayAppOTPStartInput{
			JobId:           jobID,
			Operation:       goPayAppOTPOperationSignup,
			StepName:        stepGoPayAppSignupRetry,
			OtpChannel:      startChannel,
			SmsActivationId: opts.SMSActivationID,
			StateJson:       start.GetStateJson(),
			Pin:             opts.Pin,
			CountryCode:     opts.CountryCode,
		}).Get(ctx, &retry); err != nil {
			return goPayAppStepFromOTP(retry), err
		}
		if retry.GetReady() || retry.GetAccountTokenReady() || retry.GetSignupComplete() {
			return goPayAppStepFromOTP(retry), nil
		}
		if !retry.GetOtpRequired() {
			return goPayAppStepFromOTP(retry), fmt.Errorf("gopay signup retry did not request OTP")
		}
		retryChannel := effectiveGoPayOTPChannel(retry, startChannel)
		if retryChannel == "sms" {
			var requested GoPayAppSMSActivationOutput
			if err := workflow.ExecuteActivity(activityCtx, goPayAppSMSRequestAdditionalCodeActivityName, GoPayAppSMSActivationInput{
				JobId:        jobID,
				ActivationId: opts.SMSActivationID,
				Reason:       stepGoPayAppSignupRetry,
			}).Get(ctx, &requested); err != nil {
				return GoPayAppStepOutput{ActivationId: opts.SMSActivationID, Data: requested.GetData()}, err
			}
		}
		start = retry
		startChannel = retryChannel
		otp, err = waitForOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppSignup, start, startChannel, opts.SMSActivationID, opts.Source))
		if err != nil {
			if !isOTPWaitNotReceivedError(err) {
				return goPayAppStepFromOTP(start), err
			}
			otp = OTPWaitOutput{ErrorMessage: err.Error()}
		}
		if !otp.GetFound() {
			return goPayAppStepFromOTP(start), goPaySignupOTPNotReceivedError(otp)
		}
	}

	var completed GoPayAppOTPOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppOTPCompleteActivityName, GoPayAppOTPCompleteInput{
		JobId:            jobID,
		Operation:        goPayAppOTPOperationSignup,
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
		IssuedAfterUnix:  start.GetIssuedAfterUnix(),
		OtpSource:        otp.GetSource(),
		Data:             start.GetData(),
		OtpChannel:       startChannel,
		SmsActivationId:  opts.SMSActivationID,
		StateJson:        start.GetStateJson(),
		Pin:              opts.Pin,
	}).Get(ctx, &completed); err != nil {
		return goPayAppStepFromOTP(completed), err
	}
	if completed.GetSignupComplete() || completed.GetReady() || completed.GetAccountTokenReady() {
		return goPayAppStepFromOTP(completed), nil
	}
	return goPayAppStepFromOTP(completed), fmt.Errorf("gopay signup did not complete")
}

func goPaySignupOTPNotReceivedError(wait OTPWaitOutput) error {
	reason := wait.GetErrorMessage()
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("gopay signup otp not received: %s", reason)
}

func runGoPayAppEnsureTokenAvailable(ctx workflow.Context, activityCtx workflow.Context, cancelCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (GoPayAppStepOutput, error) {
	var last GoPayAppOTPOutput
	stateJSON := opts.StateJSON
	for attempt := 0; attempt < 4; attempt++ {
		if err := workflow.ExecuteActivity(activityCtx, goPayAppOTPStartActivityName, GoPayAppOTPStartInput{
			JobId:           jobID,
			Operation:       goPayAppOTPOperationAuth,
			StepName:        stepGoPayAppLogin,
			Phone:           opts.Phone,
			OtpChannel:      opts.OTPChannel,
			SmsActivationId: opts.SMSActivationID,
			StateJson:       stateJSON,
			Pin:             opts.Pin,
			CountryCode:     opts.CountryCode,
		}).Get(ctx, &last); err != nil {
			return goPayAppStepFromOTP(last), err
		}
		stateJSON = last.GetStateJson()
		if last.GetReady() || last.GetAccountTokenReady() {
			return goPayAppStepFromOTP(last), nil
		}
		if !last.GetOtpRequired() {
			continue
		}

		startChannel := effectiveGoPayOTPChannel(last, opts.OTPChannel)
		otp, err := waitForOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppLogin, last, startChannel, opts.SMSActivationID, opts.Source))
		if err != nil {
			return goPayAppStepFromOTP(last), err
		}

		if err := workflow.ExecuteActivity(activityCtx, goPayAppOTPCompleteActivityName, GoPayAppOTPCompleteInput{
			JobId:            jobID,
			Operation:        goPayAppOTPOperationAuth,
			OtpParam:         paymentOTPParam,
			SubmittedAtParam: paymentOTPSubmittedAtParam,
			IssuedAfterUnix:  last.GetIssuedAfterUnix(),
			OtpSource:        otp.GetSource(),
			Data:             last.GetData(),
			OtpChannel:       startChannel,
			SmsActivationId:  opts.SMSActivationID,
			StateJson:        stateJSON,
			Pin:              opts.Pin,
		}).Get(ctx, &last); err != nil {
			return goPayAppStepFromOTP(last), err
		}
		stateJSON = last.GetStateJson()
		if last.GetReady() || last.GetAccountTokenReady() {
			return goPayAppStepFromOTP(last), nil
		}
	}
	return goPayAppStepFromOTP(last), fmt.Errorf("gopay auth did not reach token-valid state")
}

func runGoPayAppEnsurePinSettled(ctx workflow.Context, activityCtx workflow.Context, cancelCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (GoPayAppStepOutput, error) {
	return runGoPayAppCreatePin(ctx, activityCtx, cancelCtx, jobID, opts)
}

func runGoPayAppAuth(ctx workflow.Context, activityCtx workflow.Context, cancelCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (GoPayAppStepOutput, error) {
	token, err := runGoPayAppEnsureTokenAvailable(ctx, activityCtx, cancelCtx, jobID, opts)
	if err != nil {
		return token, err
	}
	pinOpts := opts
	pinOpts.StateJSON = token.GetStateJson()
	pin, err := runGoPayAppEnsurePinSettled(ctx, activityCtx, cancelCtx, jobID, pinOpts)
	if err != nil {
		return pin, err
	}
	if pin.GetReady() || pin.GetAccountTokenReady() {
		return pin, nil
	}
	return pin, fmt.Errorf("gopay auth did not reach token-valid state after pin settled")
}
func runGoPayAppCreatePin(ctx workflow.Context, activityCtx workflow.Context, cancelCtx workflow.Context, jobID string, opts goPayAppOTPOptions) (GoPayAppStepOutput, error) {
	var start GoPayAppOTPOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppCreatePinStartActivityName, GoPayAppCreatePinStartInput{
		JobId:           jobID,
		OtpChannel:      opts.OTPChannel,
		SmsActivationId: opts.SMSActivationID,
		StateJson:       opts.StateJSON,
		Pin:             opts.Pin,
	}).Get(ctx, &start); err != nil {
		return goPayAppStepFromOTP(start), err
	}
	if start.GetReady() || start.GetAccountTokenReady() || start.GetSignupPinComplete() {
		return goPayAppStepFromOTP(start), nil
	}
	if !start.GetOtpRequired() {
		return goPayAppStepFromOTP(start), fmt.Errorf("gopay create pin did not request OTP and did not become ready")
	}
	startChannel := effectiveGoPayOTPChannel(start, opts.OTPChannel)
	var otp OTPWaitOutput
	for attempt := 0; attempt < 2; attempt++ {
		if startChannel == "sms" {
			var requested GoPayAppSMSActivationOutput
			reason := stepGoPayAppCreatePin
			if attempt > 0 {
				reason = stepGoPayAppCreatePin + "_retry"
			}
			if err := workflow.ExecuteActivity(activityCtx, goPayAppSMSRequestAdditionalCodeActivityName, GoPayAppSMSActivationInput{
				JobId:        jobID,
				ActivationId: opts.SMSActivationID,
				Reason:       reason,
			}).Get(ctx, &requested); err != nil {
				return GoPayAppStepOutput{ActivationId: opts.SMSActivationID, Data: requested.GetData()}, err
			}
		}

		current, err := waitForOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppCreatePin, start, startChannel, opts.SMSActivationID, opts.Source))
		otp = current
		if err != nil {
			if !isOTPWaitNotReceivedError(err) {
				return goPayAppStepFromOTP(start), err
			}
			otp = OTPWaitOutput{ErrorMessage: err.Error(), Data: current.GetData()}
		}
		if otp.GetFound() {
			break
		}
		if attempt == 1 {
			return goPayAppStepFromOTP(start), goPayCreatePinOTPNotReceivedError(otp)
		}
		var retry GoPayAppOTPOutput
		if err := workflow.ExecuteActivity(activityCtx, goPayAppCreatePinRetryActivityName, GoPayAppCreatePinStartInput{
			JobId:           jobID,
			OtpChannel:      startChannel,
			Data:            start.GetData(),
			SmsActivationId: opts.SMSActivationID,
			StateJson:       start.GetStateJson(),
			Pin:             opts.Pin,
		}).Get(ctx, &retry); err != nil {
			return goPayAppStepFromOTP(retry), err
		}
		if !retry.GetOtpRequired() {
			return goPayAppStepFromOTP(retry), fmt.Errorf("gopay create pin retry did not request OTP")
		}
		start = retry
	}

	var completed GoPayAppOTPOutput
	if err := workflow.ExecuteActivity(activityCtx, goPayAppCreatePinCompleteActivityName, GoPayAppCreatePinCompleteInput{
		JobId:            jobID,
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
		IssuedAfterUnix:  start.GetIssuedAfterUnix(),
		OtpSource:        otp.GetSource(),
		Data:             start.GetData(),
		OtpChannel:       startChannel,
		SmsActivationId:  opts.SMSActivationID,
		StateJson:        start.GetStateJson(),
		Pin:              opts.Pin,
	}).Get(ctx, &completed); err != nil {
		return goPayAppStepFromOTP(completed), err
	}
	return goPayAppStepFromOTP(completed), nil
}

func goPayCreatePinOTPNotReceivedError(wait OTPWaitOutput) error {
	reason := wait.GetErrorMessage()
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("gopay create pin otp not received: %s", reason)
}

func goPayAppStepFromOTP(output GoPayAppOTPOutput) GoPayAppStepOutput {
	return GoPayAppStepOutput{
		Ready:             output.GetReady(),
		Stage:             output.GetStage(),
		Phone:             output.GetPhone(),
		AccountTokenReady: output.GetAccountTokenReady(),
		SignupComplete:    output.GetSignupComplete(),
		SignupPinComplete: output.GetSignupPinComplete(),
		Data:              output.GetData(),
		StateJson:         output.GetStateJson(),
	}
}
