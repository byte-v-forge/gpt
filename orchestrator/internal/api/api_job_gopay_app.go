package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	goPayAppPinSecretKey = "gopay_app_pin:"

	goPayAppOperationProvision      = "provision"
	goPayAppOperationLogin          = "login"
	goPayAppOperationSignup         = "signup"
	goPayAppOperationEnsurePINSetup = "ensure_pin_setup"
	goPayAppOperationCheckBalance   = "check_balance"
	goPayAppOperationCheckPIN       = "check_pin"
	goPayAppOperationChangePhone    = "change_phone"
)

func (s *Server) runGoPayAppAction(ctx context.Context, jobID string, params map[string]string) error {
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["pin_secret_key"])

	operation := normalizeGoPayAppOperation(params["operation"])
	opts := goPayAppActionOptions{
		Phone:       params["phone"],
		OTPChannel:  params["otp_channel"],
		Source:      goPayAppUserID(params["user_id"]),
		StateJSON:   "{}",
		Pin:         pin,
		CountryCode: params["country_code"],
	}
	if operation == goPayAppOperationProvision {
		return s.runGoPayAppProvisionAction(ctx, jobID, opts)
	}
	return s.runGoPayAppUserOperationAction(ctx, jobID, operation, opts)
}

func (s *Server) runGoPayAppProvisionAction(ctx context.Context, jobID string, opts goPayAppActionOptions) error {
	stateJSON := "{}"
	opts.StateJSON = stateJSON
	combined := map[string]any{"operation": goPayAppOperationProvision}

	login, err := s.runGoPayAppAuth(ctx, jobID, opts)
	mergeActionData(combined, "login", structMap(login.GetData()))
	if nextStateJSON := strings.TrimSpace(login.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppLogin, jobstatus.FailedRetryable, false, true, err, combined)
	}

	changePhone, err := s.runGoPayAppChangePhone(ctx, jobID, stateJSON, opts.Pin, opts.CountryCode)
	mergeActionData(combined, "change_phone", structMap(changePhone.GetData()))
	if nextStateJSON := strings.TrimSpace(changePhone.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppChangePhone, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["activation_id"] = changePhone.GetActivationId()
	combined["change_phone_complete"] = changePhone.GetChangePhoneComplete()

	deactivate, err := s.runGoPayAppDeactivate(ctx, jobID, changePhone.GetActivationId(), stateJSON, opts.Pin)
	mergeActionData(combined, "deactivate", structMap(deactivate.GetData()))
	if nextStateJSON := strings.TrimSpace(deactivate.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppDeactivate, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["deactivate_complete"] = deactivate.GetDeactivateComplete()

	signupOpts := opts
	signupOpts.StateJSON = stateJSON
	signup, err := s.runGoPayAppSignup(ctx, jobID, signupOpts)
	mergeActionData(combined, "signup", structMap(signup.GetData()))
	if nextStateJSON := strings.TrimSpace(signup.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["signup_complete"] = signup.GetSignupComplete()

	pinOpts := opts
	pinOpts.StateJSON = stateJSON
	pin, err := s.runGoPayAppEnsurePINSetup(ctx, jobID, pinOpts)
	mergeActionData(combined, "ensure_pin_setup", structMap(pin.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["signup_pin_complete"] = pin.GetSignupPinComplete()
	combined["account_token_ready"] = pin.GetAccountTokenReady()
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) runGoPayAppUserOperationAction(ctx context.Context, jobID string, operation string, opts goPayAppActionOptions) error {
	userID := goPayAppUserID(opts.Source)
	stored, err := s.activities.GoPayAppLoadStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "gopay_app_" + operation})
	combined := map[string]any{"operation": operation, "user_id": userID}
	mergeActionData(combined, "load_state", structMap(stored.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_gopay_state", jobstatus.FailedRetryable, false, true, err, combined)
	}
	stateJSON := strings.TrimSpace(stored.GetStateJson())
	if stateJSON == "" {
		stateJSON = "{}"
	}
	opts.Source = userID
	opts.StateJSON = stateJSON

	out, err := s.runGoPayAppOperation(ctx, jobID, operation, opts)
	mergeActionData(combined, operation, structMap(out.GetData()))
	if nextStateJSON := strings.TrimSpace(out.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if _, saveErr := s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, StateJson: stateJSON, Reason: "gopay_app_" + operation}); saveErr != nil && err == nil {
		err = saveErr
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, goPayAppOperationStep(operation), jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["account_token_ready"] = out.GetAccountTokenReady()
	combined["signup_complete"] = out.GetSignupComplete()
	combined["signup_pin_complete"] = out.GetSignupPinComplete()
	combined["change_phone_complete"] = out.GetChangePhoneComplete()
	combined["activation_id"] = out.GetActivationId()
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) runGoPayAppOperation(ctx context.Context, jobID string, operation string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	switch operation {
	case goPayAppOperationLogin:
		return s.runGoPayAppEnsureTokenAvailable(ctx, jobID, opts)
	case goPayAppOperationCheckBalance:
		return s.runGoPayAppCheckBalance(ctx, jobID, opts)
	case goPayAppOperationCheckPIN:
		return s.runGoPayAppCheckPIN(ctx, jobID, opts)
	case goPayAppOperationSignup:
		return s.runGoPayAppSignup(ctx, jobID, opts)
	case goPayAppOperationEnsurePINSetup:
		return s.runGoPayAppEnsurePINSetup(ctx, jobID, opts)
	case goPayAppOperationChangePhone:
		return s.runGoPayAppUserChangePhone(ctx, jobID, opts)
	default:
		return pb.GoPayAppStepOutput{}, fmt.Errorf("unsupported gopay app operation: %s", operation)
	}
}

func (s *Server) runGoPayAppCheckBalance(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	token, err := s.runGoPayAppEnsureTokenAvailable(ctx, jobID, opts)
	if err != nil {
		return token, err
	}
	stateJSON := firstNonEmpty(token.GetStateJson(), opts.StateJSON)
	status, err := s.activities.GoPayAppStatusActivity(ctx, pb.GoPayAppStepInput{JobId: jobID, StateJson: stateJSON})
	combined := map[string]any{"ensure_token": structMap(token.GetData()), "status": structMap(status.GetData())}
	statusData := structMap(status.GetData())
	if snapshot, ok := statusData["status"].(map[string]any); ok {
		for _, key := range []string{"balance_amount", "balance_currency", "has_min_balance"} {
			if value, ok := snapshot[key]; ok {
				combined[key] = value
			}
		}
	}
	status.Data = structData(combined)
	status.AccountTokenReady = token.GetAccountTokenReady() || status.GetAccountTokenReady()
	return status, err
}

func (s *Server) runGoPayAppCheckPIN(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	token, err := s.runGoPayAppEnsureTokenAvailable(ctx, jobID, opts)
	if err != nil {
		return token, err
	}
	stateJSON := firstNonEmpty(token.GetStateJson(), opts.StateJSON)
	status, err := s.activities.GoPayAppStatusActivity(ctx, pb.GoPayAppStepInput{JobId: jobID, StateJson: stateJSON})
	status.Data = structData(map[string]any{"ensure_token": structMap(token.GetData()), "status": structMap(status.GetData()), "pin_setup": status.GetSignupPinComplete()})
	status.AccountTokenReady = token.GetAccountTokenReady() || status.GetAccountTokenReady()
	return status, err
}

func (s *Server) runGoPayAppUserChangePhone(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	combined := map[string]any{}
	token, err := s.runGoPayAppEnsureTokenAvailable(ctx, jobID, opts)
	mergeActionData(combined, "ensure_token", structMap(token.GetData()))
	stateJSON := firstNonEmpty(token.GetStateJson(), opts.StateJSON)
	if err != nil {
		token.Data = structData(combined)
		return token, err
	}
	change, err := s.runGoPayAppChangePhone(ctx, jobID, stateJSON, opts.Pin, opts.CountryCode)
	mergeActionData(combined, "change_phone", structMap(change.GetData()))
	change.Data = structData(combined)
	return change, err
}

func (s *Server) runGoPayAppSignup(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	start, err := s.activities.GoPayAppOTPStartActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "signup", StepName: stepGoPayAppSignup, Phone: opts.Phone, OtpChannel: opts.OTPChannel, SmsActivationId: opts.SMSActivationID, ResetState: opts.ResetState, StateJson: opts.StateJSON, Pin: opts.Pin, CountryCode: opts.CountryCode, SkipPhoneProbe: opts.SkipPhoneProbe})
	if err != nil {
		return goPayAppStepFromOTP(start), err
	}
	if start.GetReady() || start.GetAccountTokenReady() || start.GetSignupComplete() {
		return goPayAppStepFromOTP(start), nil
	}
	if !start.GetOtpRequired() {
		return goPayAppStepFromOTP(start), fmt.Errorf("gopay signup did not request OTP and did not complete")
	}

	startChannel := effectiveGoPayOTPChannel(start, opts.OTPChannel)
	otp, err := s.waitGoPayOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppSignup, start, startChannel, opts.SMSActivationID, opts.Source))
	if err != nil {
		if !isOTPWaitNotReceivedError(err) {
			return goPayAppStepFromOTP(start), err
		}
		otp = pb.OTPWaitOutput{ErrorMessage: err.Error()}
	}
	if !otp.GetFound() {
		retry, err := s.activities.GoPayAppOTPRetryActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "signup", StepName: stepGoPayAppSignupRetry, OtpChannel: startChannel, SmsActivationId: opts.SMSActivationID, StateJson: start.GetStateJson(), Pin: opts.Pin, CountryCode: opts.CountryCode})
		if err != nil {
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
			if _, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: opts.SMSActivationID, Reason: stepGoPayAppSignupRetry}); err != nil {
				return pb.GoPayAppStepOutput{ActivationId: opts.SMSActivationID}, err
			}
		}
		start = retry
		startChannel = retryChannel
		otp, err = s.waitGoPayOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppSignup, start, startChannel, opts.SMSActivationID, opts.Source))
		if err != nil {
			if !isOTPWaitNotReceivedError(err) {
				return goPayAppStepFromOTP(start), err
			}
			otp = pb.OTPWaitOutput{ErrorMessage: err.Error()}
		}
		if !otp.GetFound() {
			return goPayAppStepFromOTP(start), goPaySignupOTPNotReceivedError(otp)
		}
	}

	completed, err := s.activities.GoPayAppOTPCompleteActivity(ctx, pb.GoPayAppOTPCompleteInput{JobId: jobID, Operation: "signup", OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: start.GetIssuedAfterUnix(), OtpSource: otp.GetSource(), Data: start.GetData(), OtpChannel: startChannel, SmsActivationId: opts.SMSActivationID, StateJson: start.GetStateJson(), Pin: opts.Pin})
	if err != nil {
		return goPayAppStepFromOTP(completed), err
	}
	if completed.GetSignupComplete() || completed.GetReady() || completed.GetAccountTokenReady() {
		return goPayAppStepFromOTP(completed), nil
	}
	return goPayAppStepFromOTP(completed), fmt.Errorf("gopay signup did not complete")
}

func (s *Server) runGoPayAppDeactivate(ctx context.Context, jobID, activationID, stateJSON string, pin string) (pb.GoPayAppStepOutput, error) {
	start, err := s.activities.GoPayAppDeactivateStartActivity(ctx, pb.GoPayAppDeactivateStartInput{JobId: jobID, ActivationId: activationID, StateJson: stateJSON, Pin: pin})
	if err != nil {
		return pb.GoPayAppStepOutput{ActivationId: activationID, Data: start.GetData(), StateJson: start.GetStateJson()}, err
	}
	stateJSON = start.GetStateJson()
	if !start.GetOtpRequired() {
		return pb.GoPayAppStepOutput{ActivationId: activationID, Data: start.GetData(), StateJson: stateJSON}, fmt.Errorf("gopay deactivate did not request OTP")
	}
	issuedAfterUnix := time.Now().Unix()
	wait, err := s.waitGoPayOTP(ctx, pb.OTPWaitInput{JobId: jobID, StepName: stepGoPayAppDeactivateSMSWait, Target: &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: activationID}}, TimeoutSeconds: start.GetTimeoutSeconds(), IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam})
	if err != nil {
		_, _ = s.activities.GoPayAppSMSFinishActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: activationID, Reason: err.Error()})
		return pb.GoPayAppStepOutput{ActivationId: activationID, Data: wait.GetData(), StateJson: stateJSON}, err
	}
	complete, err := s.activities.GoPayAppDeactivateCompleteActivity(ctx, pb.GoPayAppDeactivateCompleteInput{JobId: jobID, ActivationId: activationID, Code: wait.GetCode(), StateJson: stateJSON, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix})
	return goPayAppStepFromDeactivateComplete(complete), err
}

func normalizeGoPayAppOperation(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", goPayAppOperationProvision, "full":
		return goPayAppOperationProvision
	case "auth", "logon", goPayAppOperationLogin:
		return goPayAppOperationLogin
	case "balance", "check-balance", goPayAppOperationCheckBalance:
		return goPayAppOperationCheckBalance
	case "check-pin", "pin_check", goPayAppOperationCheckPIN:
		return goPayAppOperationCheckPIN
	case "register", goPayAppOperationSignup:
		return goPayAppOperationSignup
	case "pin", "set_pin", "create-pin", "create_pin", "ensure-pin-setup", goPayAppOperationEnsurePINSetup:
		return goPayAppOperationEnsurePINSetup
	case "change", "rebind", "change-phone", goPayAppOperationChangePhone:
		return goPayAppOperationChangePhone
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func goPayAppOperationStep(operation string) string {
	switch operation {
	case goPayAppOperationLogin, goPayAppOperationCheckBalance, goPayAppOperationCheckPIN:
		return stepGoPayAppLogin
	case goPayAppOperationSignup:
		return stepGoPayAppSignup
	case goPayAppOperationEnsurePINSetup:
		return stepGoPayAppEnsurePINSetup
	case goPayAppOperationChangePhone:
		return stepGoPayAppChangePhone
	default:
		return "gopay_app_" + operation
	}
}

func goPayAppUserID(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return goPayLocalSource
}

func goPayAppStepFromDeactivateComplete(output pb.GoPayAppDeactivateCompleteOutput) pb.GoPayAppStepOutput {
	return pb.GoPayAppStepOutput{ActivationId: output.GetActivationId(), DeactivateComplete: output.GetDeactivateComplete(), Data: output.GetData(), StateJson: output.GetStateJson()}
}

func goPaySignupOTPNotReceivedError(wait pb.OTPWaitOutput) error {
	reason := wait.GetErrorMessage()
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("gopay signup otp not received: %s", reason)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
