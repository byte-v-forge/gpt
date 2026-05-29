package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayPaymentStepResult struct {
	JobID              string         `json:"job_id"`
	AccountID          string         `json:"account_id,omitempty"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Action             string         `json:"action"`
	Step               string         `json:"step"`
	Success            bool           `json:"success"`
	Checked            bool           `json:"checked,omitempty"`
	PlusTrialEligible  bool           `json:"plus_trial_eligible,omitempty"`
	PlusTrialChecked   bool           `json:"plus_trial_checked,omitempty"`
	PlusActive         bool           `json:"plus_active,omitempty"`
	UserID             string         `json:"user_id,omitempty"`
	Phone              string         `json:"phone,omitempty"`
	ActivationID       string         `json:"activation_id,omitempty"`
	StateJSON          string         `json:"state_json,omitempty"`
	FlowID             string         `json:"flow_id,omitempty"`
	CheckoutURL        string         `json:"checkout_url,omitempty"`
	CheckoutSessionID  string         `json:"checkout_session_id,omitempty"`
	UseAccountToken    bool           `json:"use_account_token,omitempty"`
	Ready              bool           `json:"ready,omitempty"`
	AccountTokenReady  bool           `json:"account_token_ready,omitempty"`
	SignupComplete     bool           `json:"signup_complete,omitempty"`
	PhoneAccepted      bool           `json:"phone_accepted,omitempty"`
	DeviceProxyMatched bool           `json:"device_proxy_matched,omitempty"`
	RetryableFailure   bool           `json:"retryable_failure,omitempty"`
	RotatableFailure   bool           `json:"rotatable_failure,omitempty"`
	FailureCount       int32          `json:"failure_count,omitempty"`
	MaxFailures        int32          `json:"max_failures,omitempty"`
	ProxyHash          string         `json:"proxy_hash,omitempty"`
	DeviceFingerprint  string         `json:"device_fingerprint,omitempty"`
	OTPRequired        bool           `json:"otp_required,omitempty"`
	OTPChannel         string         `json:"otp_channel,omitempty"`
	OTPFound           bool           `json:"otp_found,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	ChargeRef          string         `json:"charge_ref,omitempty"`
	SnapTokenPresent   bool           `json:"snap_token_present,omitempty"`
	BalanceReady       bool           `json:"balance_ready,omitempty"`
	AddBalanceSelected bool           `json:"add_balance_selected,omitempty"`
	AddBalanceMethod   string         `json:"add_balance_method,omitempty"`
	ManualConfirmed    bool           `json:"manual_confirmed,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

func (s *Server) ResolveN8NGoPayPaymentAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	data := goPayPaymentBaseData(params)
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: firstNonEmpty(accountID, params["account_id"]), SourceJobId: params["source_job_id"]})
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: account.GetAccountId(), N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: "resolve_account", Success: err == nil, UserID: goPayAppUserID(params["user_id"]), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
	}
	if err := s.jobStore.SetAccountID(ctx, jobID, account.GetAccountId()); err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
	}
	data["account_id"] = account.GetAccountId()
	return result, nil
}

func (s *Server) ProbeN8NGoPayPaymentPlusTrial(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID})
	data := structMap(probe.GetData())
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepProbePlusTrial, Success: err == nil, Checked: probe.GetChecked(), PlusTrialEligible: probe.GetPlusTrialEligible(), PlusTrialChecked: probe.GetChecked(), PlusActive: probe.GetPlusActive(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayPaymentBalanceReady(ctx context.Context, jobID string, accountID string, n8nExecutionID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	nextStateJSON, data, ready, message := s.checkGoPayAddBalanceReadyAction(ctx, jobID, firstNonEmpty(stateJSON, "{}"))
	if data == nil {
		data = map[string]any{}
	}
	if message != "" {
		data["balance_message"] = message
	}
	return &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppEnsureBalance, Success: true, StateJSON: firstNonEmpty(nextStateJSON, stateJSON, "{}"), BalanceReady: ready, Data: data}, nil
}

func (s *Server) CheckN8NGoPayPaymentAddBalanceSelection(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	selected, err := s.peekGoPayAddBalanceSelection(ctx, jobID)
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedRetryable, false, true, err, nil)
	}
	method := goPayAddBalanceMethod(selected)
	data := map[string]any{"status": "awaiting_selection", "methods": goPayAddBalanceMethodOptionsAPI()}
	if method != "" {
		data["status"] = "selected"
		data["method"] = method
	}
	return &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppEnsureBalance, Success: true, AddBalanceSelected: method != "", AddBalanceMethod: method, Data: data}, nil
}

func (s *Server) ApplyN8NGoPayPaymentAddBalance(ctx context.Context, jobID string, accountID string, n8nExecutionID string, stateJSON string, targetPhone string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	selected, err := s.selectedGoPayAddBalanceParam(ctx, jobID)
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedRetryable, false, true, err, nil)
	}
	if goPayAddBalanceMethod(selected) == "" {
		err := fmt.Errorf("add_balance method is required")
		return nil, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedRetryable, false, true, err, nil)
	}
	balance, err := s.activities.GoPayAppAddBalanceActivity(ctx, pb.GoPayAppAddBalanceInput{JobId: jobID, StateJson: firstNonEmpty(stateJSON, "{}"), AddBalance: selected, TargetPhone: strings.TrimSpace(targetPhone)})
	data := structMap(balance.GetData())
	ready, _, _ := goPayAddBalanceBalanceReadyAPI(data)
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppEnsureBalance, Success: err == nil, StateJSON: firstNonEmpty(balance.GetStateJson(), stateJSON, "{}"), Phone: strings.TrimSpace(targetPhone), BalanceReady: ready, AddBalanceSelected: true, AddBalanceMethod: balance.GetMethod(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalance, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayPaymentManualAddBalanceConfirmation(ctx context.Context, jobID string, accountID string, n8nExecutionID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	nextStateJSON, data, ready, message := s.checkGoPayAddBalanceReadyAction(ctx, jobID, firstNonEmpty(stateJSON, "{}"))
	if data == nil {
		data = map[string]any{}
	}
	confirmed := ready
	if ready {
		data["auto_confirmed"] = true
	} else {
		value, found, err := s.jobStore.GetParam(ctx, jobID, manualAddBalanceConfirmParam)
		if err != nil {
			return nil, s.markActionFailed(ctx, jobID, stepGoPayAppEnsureBalanceConfirm, jobstatus.FailedRetryable, false, true, err, data)
		}
		confirmed = found && strings.EqualFold(strings.TrimSpace(value), "true")
		if confirmed {
			_ = s.jobStore.DeleteParam(ctx, jobID, manualAddBalanceConfirmParam)
			data["confirmed"] = true
			data["method"] = "manual_transfer"
		}
	}
	if message != "" {
		data["balance_message"] = message
	}
	return &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppEnsureBalanceConfirm, Success: true, StateJSON: firstNonEmpty(nextStateJSON, stateJSON, "{}"), BalanceReady: ready, ManualConfirmed: confirmed, AddBalanceMethod: "manual_transfer", Data: data}, nil
}

func (s *Server) PrepareN8NGoPayPaymentCheckout(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, phone string, stateJSON string) (any, error) {
	return s.prepareN8NGoPayPayment(ctx, stepGoPayPaymentPrepareCheckout, jobID, accountID, n8nExecutionID, userID, phone, "", "", "", stateJSON)
}

func (s *Server) PrepareN8NGoPayPaymentLink(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, phone string, flowID string, checkoutURL string, checkoutSessionID string, stateJSON string) (any, error) {
	return s.prepareN8NGoPayPayment(ctx, stepGoPayPaymentPrepareLink, jobID, accountID, n8nExecutionID, userID, phone, flowID, checkoutURL, checkoutSessionID, stateJSON)
}

func (s *Server) StartN8NGoPayPaymentStep(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, phone string, activationID string, flowID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return nil, err
	}
	input := pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, UseAccountToken: true, SkipAccountBalanceCheck: true, Tokenization: "true", PreparedFlowId: strings.TrimSpace(flowID), GopayPhone: strings.TrimSpace(phone), OtpChannel: "sms", SmsActivationId: strings.TrimSpace(activationID), UserId: goPayAppUserID(userID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin, CountryCode: params["country_code"]}
	start, err := s.activities.GoPayPaymentStartActivity(ctx, input)
	data := structMap(start.GetData())
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayPayment, Success: err == nil, UserID: goPayAppUserID(userID), Phone: strings.TrimSpace(phone), ActivationID: strings.TrimSpace(activationID), FlowID: paymentFlowID(start.GetFlowId(), flowID), StateJSON: start.GetStateJson(), UseAccountToken: start.GetUseAccountToken(), OTPRequired: start.GetOtpRequired(), OTPIssuedAfterUnix: start.GetIssuedAfterUnix(), OTPTimeoutSeconds: start.GetOtpTimeoutSeconds(), Data: data}
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(start.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, data)
	}
	if start.GetOtpRequired() {
		if err := s.requestGoPayPaymentSMSCode(ctx, input, stepGoPayPayment); err != nil {
			s.cancelGoPayPaymentAction(ctx, start.GetFlowId())
			return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, data)
		}
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayPaymentOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, activationID string, issuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	input := pb.OTPWaitInput{JobId: jobID, StepName: stepGoPayPayment, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, Target: &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}}
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayPaymentOTPCheckResult(jobID, accountID, n8nExecutionID, activationID, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayPaymentOTPCheckResult(jobID, accountID, n8nExecutionID, activationID, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayPaymentOTPCheckResult(jobID, accountID, n8nExecutionID, activationID, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) CompleteN8NGoPayPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, stateJSON string, useAccountToken bool, otpRequired bool, otpIssuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if !otpRequired {
		otpSource = "not_required"
	} else if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required when payment otp is required")
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return nil, err
	}
	payment, err := s.activities.GoPayPaymentCompleteActivity(ctx, pb.GoPayPaymentCompleteInput{JobId: jobID, AccountId: accountID, FlowId: strings.TrimSpace(flowID), OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, OtpIssuedAfterUnix: otpIssuedAfterUnix, OtpSource: strings.TrimSpace(otpSource), UseAccountToken: useAccountToken, Data: structData(data), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayPayment, Success: err == nil, FlowID: payment.GetFlowId(), StateJSON: payment.GetStateJson(), ChargeRef: payment.GetChargeRef(), SnapTokenPresent: strings.TrimSpace(payment.GetSnapToken()) != "", PlusTrialEligible: payment.GetPlusTrialEligible(), PlusTrialChecked: payment.GetPlusTrialChecked(), PlusActive: payment.GetPlusActive(), Data: structMap(payment.GetData())}
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(payment.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, structMap(payment.GetData()))
	}
	return result, nil
}

func (s *Server) FinishN8NGoPayPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, chargeRef string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	result := goPayPaymentBaseData(nil)
	for key, value := range data {
		result[key] = value
	}
	result["account_id"] = accountID
	result["payment_completed"] = strings.TrimSpace(chargeRef) != ""
	result["charge_ref"] = strings.TrimSpace(chargeRef)
	result["n8n_execution_id"] = n8nExecutionID
	tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: accountID})
	mergeActionData(result, "probe_tier", structMap(tier.GetData()))
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepProbeTier, jobstatus.FailedRecoverable, true, false, err, result)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteGoPayRuntimeSecrets(ctx, jobID)
	return &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: "finish", Success: true, ChargeRef: strings.TrimSpace(chargeRef), Data: result}, nil
}

func (s *Server) FailN8NGoPayPayment(ctx context.Context, jobID string, n8nExecutionID string, flowID string, errorMessage string, data map[string]any) (any, error) {
	if strings.TrimSpace(flowID) != "" {
		s.cancelGoPayPaymentAction(ctx, flowID)
	}
	return s.FailN8NGoPay(ctx, actionGoPayPayment, jobID, n8nExecutionID, errorMessage, data)
}

func (s *Server) prepareN8NGoPayPayment(ctx context.Context, step string, jobID string, accountID string, n8nExecutionID string, userID string, phone string, flowID string, checkoutURL string, checkoutSessionID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	input := pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, UseAccountToken: false, Tokenization: "true", GopayPhone: strings.TrimSpace(phone), UserId: goPayAppUserID(userID), StateJson: firstNonEmpty(stateJSON, "{}"), CountryCode: params["country_code"]}
	var out pb.GoPayPaymentPrepareOutput
	if step == stepGoPayPaymentPrepareCheckout {
		out, err = s.activities.GoPayPaymentPrepareCheckoutActivity(ctx, input)
	} else {
		input.PreparedFlowId = strings.TrimSpace(flowID)
		input.CheckoutUrl = strings.TrimSpace(checkoutURL)
		input.CheckoutSessionId = strings.TrimSpace(checkoutSessionID)
		out, err = s.activities.GoPayPaymentPrepareLinkActivity(ctx, input)
	}
	data := structMap(out.GetData())
	result := goPayPaymentPrepareResult(jobID, accountID, n8nExecutionID, step, goPayAppUserID(userID), strings.TrimSpace(phone), out, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	if out.GetRetryableFreshCheckout() {
		message := strings.TrimSpace(stringMapValue(data, "error_message"))
		if message == "" {
			message = "chatgpt approve blocked"
		}
		err := fmt.Errorf("payment prepare link blocked: %s", message)
		return result, s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func goPayPaymentBaseData(params map[string]string) map[string]any {
	data := map[string]any{
		"action":              actionGoPayPayment,
		"otp_channel":         "sms",
		"uses_gopay_app_flow": true,
		"uses_account_token":  true,
	}
	if params != nil {
		data["source_job_id"] = params["source_job_id"]
		data["user_id"] = goPayAppUserID(params["user_id"])
	}
	return data
}

func goPayPaymentPrepareResult(jobID string, accountID string, n8nExecutionID string, step string, userID string, phone string, out pb.GoPayPaymentPrepareOutput, data map[string]any, success bool) *n8nGoPayPaymentStepResult {
	return &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: step, Success: success, UserID: userID, Phone: phone, FlowID: out.GetFlowId(), CheckoutURL: out.GetCheckoutUrl(), CheckoutSessionID: out.GetCheckoutSessionId(), UseAccountToken: out.GetUseAccountToken(), StateJSON: out.GetStateJson(), SnapTokenPresent: strings.TrimSpace(out.GetSnapToken()) != "", Data: data}
}

func goPayPaymentOTPCheckResult(jobID string, accountID string, n8nExecutionID string, activationID string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayPaymentStepResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayPayment, Success: true, ActivationID: strings.TrimSpace(activationID), OTPFound: found, OTPSource: strings.TrimSpace(source), OTPIssuedAfterUnix: issuedAfterUnix, Data: data}
}
