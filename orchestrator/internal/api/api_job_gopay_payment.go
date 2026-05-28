package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	goPayPaymentPinSecretKey          = "gopay_payment_pin:"
	goPayPaymentAddBalanceParam       = "gopay_add_balance_json"
	goPayPaymentDefaultWaitSeconds    = int32(1800)
	goPayAppSignupMaxPhoneAttemptsAPI = 3
)

func (s *Server) runGoPayPaymentAction(ctx context.Context, jobID string, params map[string]string) error {
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["pin_secret_key"])

	otpChannel := normalizeGoPayOTPChannel(params["otp_channel"])
	if otpChannel == "" {
		otpChannel = "sms"
	}
	if otpChannel == "wa" {
		return s.runGoPayPaymentWithWAAppAction(ctx, jobID, params, pin)
	}
	return s.runGoPayPaymentWithSMSAppAction(ctx, jobID, params, pin)
}

func (s *Server) runGoPayPaymentWithWAAppAction(ctx context.Context, jobID string, params map[string]string, pin string) error {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: params["account_id"], SourceJobId: params["source_job_id"]})
	combined := map[string]any{"action": actionGoPayPayment, "otp_channel": "wa", "user_id": goPayAppUserID(params["user_id"])}
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}
	accountID := account.GetAccountId()
	if err := s.jobStore.SetAccountID(ctx, jobID, accountID); err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["account_id"] = accountID

	if _, err := s.probeGoPayPaymentEligibility(ctx, jobID, accountID, combined); err != nil {
		return err
	}

	userID := goPayAppUserID(params["user_id"])
	waPhone, err := s.activities.GoPayResolveWAPhoneActivity(ctx, pb.GoPayResolveWAPhoneInput{JobId: jobID, UserId: userID, WaPhone: params["wa_phone"]})
	mergeActionData(combined, "wa_phone_resolution", structMap(waPhone.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayResolveWAPhone, jobstatus.FailedRetryable, false, true, err, combined)
	}
	userID = goPayAppUserID(waPhone.GetUserId())
	combined["user_id"] = userID
	combined["wa_phone_present"] = strings.TrimSpace(waPhone.GetWaPhone()) != ""

	stored, err := s.activities.GoPayAppLoadStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "wa_payment_start"})
	mergeActionData(combined, "load_state", structMap(stored.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_gopay_state", jobstatus.FailedRetryable, false, true, err, combined)
	}
	stateJSON := firstNonEmpty(stored.GetStateJson(), "{}")
	opts := goPayAppActionOptions{Phone: waPhone.GetWaPhone(), OTPChannel: "wa", Source: userID, StateJSON: stateJSON, Pin: pin, CountryCode: params["country_code"], SkipPhoneProbe: true}

	signup, err := s.runGoPayAppSignup(ctx, jobID, opts)
	stateJSON = firstNonEmpty(signup.GetStateJson(), stateJSON)
	mergeActionData(combined, "signup", structMap(signup.GetData()))
	combined["signup_phone_probe_skipped"] = true
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, combined)
	}
	_, _ = s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, StateJson: stateJSON, Reason: "wa_payment_signup"})

	pinSetup, err := s.runGoPayAppEnsurePINSetup(ctx, jobID, goPayAppActionOptions{OTPChannel: "wa", Source: userID, StateJSON: stateJSON, Pin: pin, CountryCode: params["country_code"]})
	stateJSON = firstNonEmpty(pinSetup.GetStateJson(), stateJSON)
	mergeActionData(combined, "ensure_pin_setup", structMap(pinSetup.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !pinSetup.GetAccountTokenReady() {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after ensure pin setup"), combined)
	}
	_, _ = s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, StateJson: stateJSON, Reason: "wa_payment_pin_setup"})

	addBalance, err := goPayPaymentParamAddBalance(params)
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedFinal, false, false, err, combined)
	}
	stateJSON, err = s.ensureGoPayPaymentBalance(ctx, jobID, stateJSON, addBalance, waPhone.GetWaPhone(), paymentTimeoutSeconds(params), combined)
	if err != nil {
		return err
	}

	payment, stateJSON, err := s.prepareAndRunGoPayPaymentAction(ctx, jobID, accountID, "wa", userID, waPhone.GetWaPhone(), stateJSON, pin, params["country_code"], true, "")
	mergeActionData(combined, "gopay_payment", structMap(payment.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["payment_completed"] = true
	combined["charge_ref"] = payment.GetChargeRef()
	combined["snap_token_present"] = payment.GetSnapToken() != ""
	_, _ = s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, StateJson: stateJSON, Reason: "wa_payment_completed"})

	if err := s.probeGoPayPaymentTier(ctx, jobID, accountID, combined); err != nil {
		return err
	}
	s.enqueueGoPayPaymentRebindAction(ctx, jobID, accountID, userID, pin, params["country_code"], combined)
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) runGoPayPaymentWithSMSAppAction(ctx context.Context, jobID string, params map[string]string, pin string) error {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: params["account_id"], SourceJobId: params["source_job_id"]})
	userID := goPayAppUserID(params["user_id"])
	combined := map[string]any{"action": actionGoPayPayment, "otp_channel": "sms", "user_id": userID}
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}
	accountID := account.GetAccountId()
	if err := s.jobStore.SetAccountID(ctx, jobID, accountID); err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["account_id"] = accountID

	if _, err := s.probeGoPayPaymentEligibility(ctx, jobID, accountID, combined); err != nil {
		return err
	}
	signup, stateJSON, err := s.runGoPayPaymentSMSSignup(ctx, jobID, userID, pin, params["country_code"], combined)
	if err != nil {
		return err
	}
	if !signup.GetSignupComplete() {
		return s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, fmt.Errorf("gopay signup did not complete"), combined)
	}

	pinSetup, err := s.runGoPayAppEnsurePINSetup(ctx, jobID, goPayAppActionOptions{OTPChannel: "sms", SMSActivationID: signup.GetActivationId(), Source: userID, StateJSON: stateJSON, Pin: pin, CountryCode: params["country_code"]})
	stateJSON = firstNonEmpty(pinSetup.GetStateJson(), stateJSON)
	mergeActionData(combined, "ensure_pin_setup", structMap(pinSetup.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !pinSetup.GetAccountTokenReady() {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after ensure pin setup"), combined)
	}

	addBalance, err := goPayPaymentParamAddBalance(params)
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedFinal, false, false, err, combined)
	}
	stateJSON, err = s.ensureGoPayPaymentBalance(ctx, jobID, stateJSON, addBalance, signup.GetPhone(), paymentTimeoutSeconds(params), combined)
	if err != nil {
		return err
	}

	payment, _, err := s.prepareAndRunGoPayPaymentAction(ctx, jobID, accountID, "sms", userID, signup.GetPhone(), stateJSON, pin, params["country_code"], true, signup.GetActivationId())
	mergeActionData(combined, "gopay_payment", structMap(payment.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if err := s.probeGoPayPaymentTier(ctx, jobID, accountID, combined); err != nil {
		return err
	}
	combined["payment_completed"] = payment.GetChargeRef() != ""
	combined["charge_ref"] = payment.GetChargeRef()
	combined["snap_token_present"] = payment.GetSnapToken() != ""
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) probeGoPayPaymentEligibility(ctx context.Context, jobID string, accountID string, data map[string]any) (pb.ProbePlusTrialActivityOutput, error) {
	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID})
	mergeActionData(data, "probe_plus_trial", structMap(probe.GetData()))
	if err != nil {
		return probe, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, data)
	}
	if !probe.GetChecked() {
		return probe, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), data)
	}
	if !probe.GetPlusTrialEligible() {
		return probe, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), data)
	}
	return probe, nil
}

func (s *Server) runGoPayPaymentSMSSignup(ctx context.Context, jobID string, userID string, pin string, countryCode string, data map[string]any) (pb.GoPayAppStepOutput, string, error) {
	var last pb.GoPayAppStepOutput
	var stateJSON string
	attempts := []any{}
	for attempt := 1; attempt <= goPayAppSignupMaxPhoneAttemptsAPI; attempt++ {
		attemptData := map[string]any{"attempt": attempt, "max_attempts": goPayAppSignupMaxPhoneAttemptsAPI}
		phone, err := s.activities.GoPayAppAcquireSignupPhoneActivity(ctx, pb.GoPayAppAcquireSignupPhoneInput{JobId: jobID, FailureCount: int32(attempt - 1)})
		attemptData["signup_phone"] = structMap(phone.GetData())
		last = pb.GoPayAppStepOutput{ActivationId: phone.GetActivationId(), Phone: phone.GetPhone(), Data: phone.GetData(), StateJson: phone.GetStateJson()}
		if err != nil {
			attemptData["error_message"] = err.Error()
			attempts = append(attempts, attemptData)
			data["signup_attempts"] = attempts
			return last, stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppSignupPhone, jobstatus.FailedRetryable, false, true, err, data)
		}

		deviceProxy, err := s.activities.GoPayAppGenerateDeviceProxyActivity(ctx, pb.GoPayAppGenerateDeviceProxyInput{JobId: jobID})
		attemptData["device_proxy"] = structMap(deviceProxy.GetData())
		stateJSON = deviceProxy.GetStateJson()
		if err != nil {
			attemptData["error_message"] = err.Error()
			attempts = append(attempts, attemptData)
			data["signup_attempts"] = attempts
			_, _ = s.activities.GoPayAppDiscardSignupPhoneActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: phone.GetActivationId(), FailureCount: int32(attempt), Reason: err.Error()})
			return last, stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppGenerateDeviceProxy, jobstatus.FailedRetryable, false, true, err, data)
		}
		if strings.TrimSpace(stateJSON) == "" {
			err := fmt.Errorf("generated device proxy state_json missing")
			attemptData["error_message"] = err.Error()
			attempts = append(attempts, attemptData)
			data["signup_attempts"] = attempts
			_, _ = s.activities.GoPayAppDiscardSignupPhoneActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: phone.GetActivationId(), FailureCount: int32(attempt), Reason: err.Error()})
			return last, stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppGenerateDeviceProxy, jobstatus.FailedRetryable, false, true, err, data)
		}

		check, err := s.activities.GoPayAppCheckSignupPhoneActivity(ctx, pb.GoPayAppCheckSignupPhoneInput{JobId: jobID, ActivationId: phone.GetActivationId(), Phone: phone.GetPhone(), CountryCode: countryCode, StateJson: stateJSON})
		attemptData["check_phone"] = structMap(check.GetData())
		stateJSON = firstNonEmpty(check.GetStateJson(), stateJSON)
		if err != nil || !goPayAppDeviceProxyMatchedAPI(deviceProxy, check) {
			if err == nil {
				err = fmt.Errorf("generated device proxy mismatch after check_phone")
			}
			attemptData["error_message"] = err.Error()
			attempts = append(attempts, attemptData)
			data["signup_attempts"] = attempts
			_, _ = s.activities.GoPayAppDiscardSignupPhoneActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: phone.GetActivationId(), FailureCount: int32(attempt), Reason: err.Error()})
			if attempt < goPayAppSignupMaxPhoneAttemptsAPI && isGoPaySignupPhoneRotatableErrorAPI(err) {
				continue
			}
			return last, stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppCheckPhone, jobstatus.FailedRetryable, false, true, err, data)
		}

		signup, err := s.runGoPayAppSignup(ctx, jobID, goPayAppActionOptions{Phone: phone.GetPhone(), OTPChannel: "sms", SMSActivationID: phone.GetActivationId(), Source: userID, StateJSON: stateJSON, Pin: pin, CountryCode: countryCode})
		stateJSON = firstNonEmpty(signup.GetStateJson(), stateJSON)
		last = signup
		attemptData["signup"] = structMap(signup.GetData())
		attempts = append(attempts, attemptData)
		data["signup_attempts"] = attempts
		mergeActionData(data, "signup", structMap(signup.GetData()))
		if err == nil {
			return signup, stateJSON, nil
		}
		if attempt < goPayAppSignupMaxPhoneAttemptsAPI && (isGoPaySignupOTPNotReceivedAPI(err) || isGoPaySignupPhoneRotatableErrorAPI(err)) {
			_, _ = s.activities.GoPayAppDiscardSignupPhoneActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: phone.GetActivationId(), FailureCount: int32(attempt), Reason: err.Error()})
			continue
		}
		return signup, stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, data)
	}
	return last, stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, fmt.Errorf("gopay signup attempts exhausted"), data)
}

func (s *Server) ensureGoPayPaymentBalance(ctx context.Context, jobID string, stateJSON string, addBalance *pb.GoPayAddBalance, targetPhone string, timeoutSeconds int32, data map[string]any) (string, error) {
	if addBalance == nil {
		selected, nextStateJSON, balanceData, ready, err := s.waitForGoPayAddBalanceSelectionAction(ctx, jobID, stateJSON, timeoutSeconds)
		stateJSON = firstNonEmpty(nextStateJSON, stateJSON)
		if err != nil {
			if len(balanceData) > 0 {
				data["add_balance_check"] = balanceData
			}
			data["add_balance"] = map[string]any{"status": "awaiting_selection", "methods": goPayAddBalanceMethodOptionsAPI()}
			return stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedRetryable, false, true, err, data)
		}
		if ready {
			data["add_balance"] = balanceData
			return stateJSON, nil
		}
		addBalance = selected
	}
	balance, err := s.activities.GoPayAppAddBalanceActivity(ctx, pb.GoPayAppAddBalanceInput{JobId: jobID, StateJson: stateJSON, AddBalance: addBalance, TargetPhone: targetPhone})
	stateJSON = firstNonEmpty(balance.GetStateJson(), stateJSON)
	mergeActionData(data, "add_balance", structMap(balance.GetData()))
	if err != nil {
		return stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedRetryable, false, true, err, data)
	}
	if balance.GetMethod() == "manual_transfer" {
		nextStateJSON, confirmation, err := s.waitForManualAddBalanceAction(ctx, jobID, stateJSON, timeoutSeconds)
		stateJSON = firstNonEmpty(nextStateJSON, stateJSON)
		data["add_balance_confirmation"] = confirmation
		if err != nil {
			return stateJSON, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalanceConfirm, jobstatus.FailedRetryable, false, true, err, data)
		}
	}
	return stateJSON, nil
}

func (s *Server) waitForGoPayAddBalanceSelectionAction(ctx context.Context, jobID string, stateJSON string, timeoutSeconds int32) (*pb.GoPayAddBalance, string, map[string]any, bool, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = goPayPaymentDefaultWaitSeconds
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: jobID, StepName: stepGoPayAppEnsureBalance, Recoverable: false, Retryable: true, Detail: structData(map[string]any{"status": "awaiting_selection", "methods": goPayAddBalanceMethodOptionsAPI()})}); err != nil {
		return nil, stateJSON, nil, false, err
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	var lastData map[string]any
	var lastErr string
	for {
		nextStateJSON, checkData, ready, checkErr := s.checkGoPayAddBalanceReadyAction(ctx, jobID, stateJSON)
		stateJSON = firstNonEmpty(nextStateJSON, stateJSON)
		if len(checkData) > 0 {
			lastData = checkData
		}
		if checkErr != "" {
			lastErr = checkErr
		}
		if ready {
			checkData["status"] = "balance_ready"
			checkData["method"] = "balance_poll"
			checkData["ensure_balance_complete"] = true
			checkData["auto_confirmed"] = true
			_ = s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{JobId: jobID, StepName: stepGoPayAppEnsureBalance, Recoverable: false, Retryable: true, Result: structData(checkData)})
			return nil, stateJSON, checkData, true, nil
		}
		selected, err := s.selectedGoPayAddBalanceParam(ctx, jobID)
		if err != nil {
			return nil, stateJSON, lastData, false, err
		}
		if goPayAddBalanceMethod(selected) != "" {
			return selected, stateJSON, lastData, false, nil
		}
		if time.Now().After(deadline) {
			if lastErr != "" {
				return nil, stateJSON, lastData, false, fmt.Errorf("add_balance method not selected and balance not ready after %ds: %s", timeoutSeconds, lastErr)
			}
			return nil, stateJSON, lastData, false, fmt.Errorf("add_balance method not selected and balance not ready after %ds", timeoutSeconds)
		}
		select {
		case <-ctx.Done():
			return nil, stateJSON, lastData, false, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (s *Server) waitForManualAddBalanceAction(ctx context.Context, jobID string, stateJSON string, timeoutSeconds int32) (string, map[string]any, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = goPayPaymentDefaultWaitSeconds
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	var lastData map[string]any
	var lastErr string
	for {
		nextStateJSON, checkData, ready, checkErr := s.checkGoPayAddBalanceReadyAction(ctx, jobID, stateJSON)
		stateJSON = firstNonEmpty(nextStateJSON, stateJSON)
		if len(checkData) > 0 {
			lastData = checkData
		}
		if checkErr != "" {
			lastErr = checkErr
		}
		if ready {
			checkData["confirmed"] = true
			checkData["auto_confirmed"] = true
			checkData["method"] = "manual_transfer"
			return stateJSON, checkData, nil
		}
		confirmed, found, err := s.jobStore.GetParam(ctx, jobID, manualAddBalanceConfirmParam)
		if err != nil {
			return stateJSON, lastData, err
		}
		if found && strings.EqualFold(strings.TrimSpace(confirmed), "true") {
			_ = s.jobStore.DeleteParam(ctx, jobID, manualAddBalanceConfirmParam)
			if lastData == nil {
				lastData = map[string]any{}
			}
			lastData["confirmed"] = true
			lastData["method"] = "manual_transfer"
			return stateJSON, lastData, nil
		}
		if time.Now().After(deadline) {
			if lastErr != "" {
				return stateJSON, lastData, fmt.Errorf("manual add_balance not confirmed and balance not ready after %ds: %s", timeoutSeconds, lastErr)
			}
			return stateJSON, lastData, fmt.Errorf("manual add_balance not confirmed and balance not ready after %ds", timeoutSeconds)
		}
		select {
		case <-ctx.Done():
			return stateJSON, lastData, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (s *Server) checkGoPayAddBalanceReadyAction(ctx context.Context, jobID string, stateJSON string) (string, map[string]any, bool, string) {
	status, err := s.activities.GoPayAppBalanceCheckActivity(ctx, pb.GoPayAppStepInput{JobId: jobID, StateJson: stateJSON})
	data := structMap(status.GetData())
	if err != nil {
		return firstNonEmpty(status.GetStateJson(), stateJSON), map[string]any{"error_message": err.Error()}, false, err.Error()
	}
	ready, amount, currency := goPayAddBalanceBalanceReadyAPI(data)
	if ready {
		data["balance_ready"] = true
		data["balance_amount"] = amount
		if currency != "" {
			data["balance_currency"] = currency
		}
		return firstNonEmpty(status.GetStateJson(), stateJSON), data, true, ""
	}
	if message := stringMapValue(data, "error_message"); message != "" {
		return firstNonEmpty(status.GetStateJson(), stateJSON), data, false, message
	}
	if amount != 0 || currency != "" {
		return firstNonEmpty(status.GetStateJson(), stateJSON), data, false, fmt.Sprintf("balance %d %s < 1 IDR", amount, currency)
	}
	return firstNonEmpty(status.GetStateJson(), stateJSON), data, false, ""
}

func (s *Server) prepareAndRunGoPayPaymentAction(ctx context.Context, jobID string, accountID string, otpChannel string, userID string, phone string, stateJSON string, pin string, countryCode string, useAccountToken bool, smsActivationID string) (pb.GoPayActivityOutput, string, error) {
	prepare, err := s.prepareGoPayPaymentAction(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, UseAccountToken: false, Tokenization: "true", GopayPhone: phone, UserId: userID, StateJson: stateJSON, Pin: pin, CountryCode: countryCode})
	stateJSON = firstNonEmpty(prepare.GetStateJson(), stateJSON)
	if err != nil {
		return pb.GoPayActivityOutput{Data: prepare.GetData(), StateJson: stateJSON}, stateJSON, err
	}
	payment, err := s.runGoPayPaymentActionStep(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, UseAccountToken: useAccountToken, Tokenization: "true", PreparedFlowId: prepare.GetFlowId(), GopayPhone: phone, OtpChannel: otpChannel, SmsActivationId: smsActivationID, UserId: userID, StateJson: stateJSON, Pin: pin, CountryCode: countryCode})
	return payment, firstNonEmpty(payment.GetStateJson(), stateJSON), err
}

func (s *Server) probeGoPayPaymentTier(ctx context.Context, jobID string, accountID string, data map[string]any) error {
	tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: accountID})
	mergeActionData(data, "probe_tier", structMap(tier.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepProbeTier, jobstatus.FailedRecoverable, true, false, err, data)
	}
	return nil
}

func (s *Server) enqueueGoPayPaymentRebindAction(ctx context.Context, sourceJobID string, accountID string, userID string, pin string, countryCode string, data map[string]any) {
	rebindJobID := sourceJobID + "-rebind"
	pinSecretKey := ""
	if strings.TrimSpace(pin) != "" {
		pinSecretKey = goPayPaymentRebindPinSecretKey + rebindJobID
		if err := s.saveRuntimeSecretValue(ctx, pinSecretKey, pin); err != nil {
			data["rebind_error_message"] = err.Error()
			return
		}
	}
	params := map[string]string{"source_job_id": sourceJobID, "account_id": accountID, "user_id": userID}
	if countryCode = strings.TrimSpace(countryCode); countryCode != "" {
		params["country_code"] = countryCode
	}
	if pinSecretKey != "" {
		params["pin_secret_key"] = pinSecretKey
	}
	if _, err := s.jobStore.CreateWithID(ctx, rebindJobID, accountID, actionGoPayPaymentRebind, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		data["rebind_error_message"] = err.Error()
		return
	}
	data["rebind_required"] = true
	data["rebind_job_id"] = rebindJobID
}

func (s *Server) selectedGoPayAddBalanceParam(ctx context.Context, jobID string) (*pb.GoPayAddBalance, error) {
	raw, found, err := s.jobStore.GetParam(ctx, jobID, goPayAddBalanceSelectionParam)
	if err != nil || !found {
		return nil, err
	}
	addBalance, err := decodeGoPayAddBalance(raw)
	if err != nil {
		return nil, err
	}
	_ = s.jobStore.DeleteParam(ctx, jobID, goPayAddBalanceSelectionParam)
	return addBalance, nil
}

func encodeGoPayAddBalance(value *pb.GoPayAddBalance) (string, error) {
	if value == nil {
		return "", nil
	}
	out, err := protojson.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func decodeGoPayAddBalance(raw string) (*pb.GoPayAddBalance, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := &pb.GoPayAddBalance{}
	if err := protojson.Unmarshal([]byte(raw), out); err != nil {
		return nil, err
	}
	return out, nil
}

func goPayPaymentParamAddBalance(params map[string]string) (*pb.GoPayAddBalance, error) {
	return decodeGoPayAddBalance(params[goPayPaymentAddBalanceParam])
}

func paymentTimeoutSeconds(params map[string]string) int32 {
	value, _ := strconv.ParseInt(strings.TrimSpace(params["add_balance_confirm_timeout_seconds"]), 10, 32)
	if value <= 0 {
		return goPayPaymentDefaultWaitSeconds
	}
	return int32(value)
}

func goPayAddBalanceMethodOptionsAPI() []any {
	return []any{"manual_transfer", "envelope", "rekberinaja"}
}

func goPayAddBalanceBalanceReadyAPI(data map[string]any) (bool, int64, string) {
	source := data
	if nested, ok := data["status"].(map[string]any); ok && len(nested) > 0 {
		source = nested
	}
	amount := int64MapValueAPI(source, "balance_amount")
	currency := stringMapValue(source, "balance_currency")
	return boolMapValueAPI(source, "has_min_balance") || amount >= 1 || boolMapValueAPI(source, "balance_ready"), amount, currency
}

func boolMapValueAPI(data map[string]any, key string) bool {
	switch value := data[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

func int64MapValueAPI(data map[string]any, key string) int64 {
	switch value := data[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	default:
		return 0
	}
}

func goPayAppDeviceProxyMatchedAPI(expected pb.GoPayAppGenerateDeviceProxyOutput, actual pb.GoPayAppCheckSignupPhoneOutput) bool {
	expectedProxy := strings.TrimSpace(expected.GetProxyHash())
	actualProxy := strings.TrimSpace(actual.GetProxyHash())
	expectedDevice := strings.TrimSpace(expected.GetDeviceFingerprint())
	actualDevice := strings.TrimSpace(actual.GetDeviceFingerprint())
	return expectedProxy != "" && actualProxy != "" && expectedDevice != "" && actualDevice != "" && expectedProxy == actualProxy && expectedDevice == actualDevice
}

func isGoPaySignupPhoneRotatableErrorAPI(err error) bool {
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

func isGoPaySignupOTPNotReceivedAPI(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "gopay signup otp not received") ||
		strings.Contains(message, "otp not received") ||
		strings.Contains(message, "otp not found") ||
		strings.Contains(message, "waitcode")
}
