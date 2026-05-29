package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayWAPaymentResult struct {
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
	WAPhone            string         `json:"wa_phone,omitempty"`
	CheckoutURL        string         `json:"checkout_url,omitempty"`
	CheckoutSessionID  string         `json:"checkout_session_id,omitempty"`
	UseAccountToken    bool           `json:"use_account_token,omitempty"`
	OTPRequired        bool           `json:"otp_required,omitempty"`
	OTPFound           bool           `json:"otp_found,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	ChargeRef          string         `json:"charge_ref,omitempty"`
	SnapTokenPresent   bool           `json:"snap_token_present,omitempty"`
	FlowID             string         `json:"flow_id,omitempty"`
	StateJSON          string         `json:"state_json,omitempty"`
	AccessTokenPresent bool           `json:"access_token_present,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

func (s *Server) ResolveN8NGoPayWAPaymentAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	accountID = firstNonEmpty(accountID, params["account_id"])
	sourceJobID := strings.TrimSpace(params["source_job_id"])
	accessTokenPresent := strings.TrimSpace(params["access_token_secret_key"]) != ""
	data := waPaymentData(params)
	data["access_token_present"] = accessTokenPresent
	if accountID == "" && sourceJobID == "" && !accessTokenPresent {
		err := fmt.Errorf("account_id, source_job_id, or access_token is required")
		return &n8nGoPayWAPaymentResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: "resolve_account", Success: false, Data: data}, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedFinal, false, false, err, data)
	}
	if accountID != "" || sourceJobID != "" {
		account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: accountID, SourceJobId: sourceJobID})
		result := &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: account.GetAccountId(), N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: "resolve_account", Success: err == nil, AccessTokenPresent: accessTokenPresent, Data: data}
		if err != nil {
			return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
		}
		accountID = account.GetAccountId()
		if err := s.jobStore.SetAccountID(ctx, jobID, accountID); err != nil {
			return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
		}
	}
	data["account_id"] = accountID
	return &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: "resolve_account", Success: true, AccessTokenPresent: accessTokenPresent, Data: data}, nil
}

func (s *Server) ProbeN8NGoPayWAPaymentPlusTrial(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	accessToken, err := s.runtimeSecretValue(ctx, params["access_token_secret_key"])
	if err != nil {
		return nil, err
	}
	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID, AccessToken: accessToken})
	data := structMap(probe.GetData())
	result := &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: stepProbePlusTrial, Success: err == nil, Checked: probe.GetChecked(), PlusTrialEligible: probe.GetPlusTrialEligible(), PlusTrialChecked: probe.GetChecked(), PlusActive: probe.GetPlusActive(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) ResolveN8NGoPayWAPaymentPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	userID := goPayAppUserID(params["user_id"])
	waPhone, err := s.activities.GoPayResolveWAPhoneActivity(ctx, pb.GoPayResolveWAPhoneInput{JobId: jobID, UserId: userID, WaPhone: params["wa_phone"]})
	data := structMap(waPhone.GetData())
	result := &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: stepGoPayResolveWAPhone, Success: err == nil, UserID: goPayAppUserID(waPhone.GetUserId()), WAPhone: waPhone.GetWaPhone(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayResolveWAPhone, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) PrepareN8NGoPayWAPaymentCheckout(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, waPhone string) (any, error) {
	return s.prepareN8NGoPayWAPayment(ctx, stepGoPayPaymentPrepareCheckout, jobID, accountID, n8nExecutionID, userID, waPhone, "", "", "", "{}")
}

func (s *Server) PrepareN8NGoPayWAPaymentLink(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, waPhone string, flowID string, checkoutURL string, checkoutSessionID string, stateJSON string) (any, error) {
	return s.prepareN8NGoPayWAPayment(ctx, stepGoPayPaymentPrepareLink, jobID, accountID, n8nExecutionID, userID, waPhone, flowID, checkoutURL, checkoutSessionID, stateJSON)
}

func (s *Server) StartN8NGoPayWAPaymentStep(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, waPhone string, flowID string, stateJSON string) (any, error) {
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
	accessToken, err := s.runtimeSecretValue(ctx, params["access_token_secret_key"])
	if err != nil {
		return nil, err
	}
	start, err := s.activities.GoPayPaymentStartActivity(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, AccessToken: accessToken, Tokenization: "true", PreparedFlowId: strings.TrimSpace(flowID), GopayPhone: strings.TrimSpace(waPhone), OtpChannel: "wa", UserId: goPayAppUserID(userID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin, CountryCode: params["country_code"]})
	data := structMap(start.GetData())
	result := &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: stepGoPayPayment, Success: err == nil, UserID: goPayAppUserID(userID), WAPhone: strings.TrimSpace(waPhone), FlowID: paymentFlowID(start.GetFlowId(), flowID), StateJSON: start.GetStateJson(), UseAccountToken: start.GetUseAccountToken(), OTPRequired: start.GetOtpRequired(), OTPIssuedAfterUnix: start.GetIssuedAfterUnix(), OTPTimeoutSeconds: start.GetOtpTimeoutSeconds(), Data: data}
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(start.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayWAPaymentOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, issuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	input := pb.OTPWaitInput{JobId: jobID, StepName: stepGoPayPayment, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, Target: &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: goPayAppUserID(userID)}}}
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return waOTPCheckResult(jobID, accountID, n8nExecutionID, userID, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return waOTPCheckResult(jobID, accountID, n8nExecutionID, userID, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return waOTPCheckResult(jobID, accountID, n8nExecutionID, userID, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) CompleteN8NGoPayWAPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, stateJSON string, useAccountToken bool, otpRequired bool, otpIssuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
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
	result := &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: stepGoPayPayment, Success: err == nil, FlowID: payment.GetFlowId(), StateJSON: payment.GetStateJson(), ChargeRef: payment.GetChargeRef(), SnapTokenPresent: strings.TrimSpace(payment.GetSnapToken()) != "", PlusTrialEligible: payment.GetPlusTrialEligible(), PlusTrialChecked: payment.GetPlusTrialChecked(), PlusActive: payment.GetPlusActive(), Data: structMap(payment.GetData())}
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(payment.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, structMap(payment.GetData()))
	}
	return result, nil
}

func (s *Server) FinishN8NGoPayWAPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, chargeRef string, plusTrialEligible bool, plusTrialChecked bool, plusActive bool, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	result := waPaymentData(nil)
	for key, value := range data {
		result[key] = value
	}
	result["account_id"] = accountID
	result["payment_completed"] = strings.TrimSpace(chargeRef) != "" && plusActive
	result["payment_async_pending"] = false
	result["charge_ref"] = strings.TrimSpace(chargeRef)
	result["plus_trial_eligible"] = plusTrialEligible
	result["plus_trial_checked"] = plusTrialChecked
	result["plus_active"] = plusActive
	result["n8n_execution_id"] = n8nExecutionID
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteGoPayRuntimeSecrets(ctx, jobID)
	return &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: "finish", Success: true, ChargeRef: strings.TrimSpace(chargeRef), PlusTrialEligible: plusTrialEligible, PlusTrialChecked: plusTrialChecked, PlusActive: plusActive, Data: result}, nil
}

func (s *Server) FailN8NGoPayWAPayment(ctx context.Context, jobID string, n8nExecutionID string, flowID string, errorMessage string, data map[string]any) (any, error) {
	if strings.TrimSpace(flowID) != "" {
		s.cancelGoPayPaymentAction(ctx, flowID)
	}
	return s.FailN8NGoPay(ctx, actionGoPayWAPayment, jobID, n8nExecutionID, errorMessage, data)
}

func (s *Server) prepareN8NGoPayWAPayment(ctx context.Context, step string, jobID string, accountID string, n8nExecutionID string, userID string, waPhone string, flowID string, checkoutURL string, checkoutSessionID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	accessToken, err := s.runtimeSecretValue(ctx, params["access_token_secret_key"])
	if err != nil {
		return nil, err
	}
	input := pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, AccessToken: accessToken, Tokenization: "true", GopayPhone: strings.TrimSpace(waPhone), UserId: goPayAppUserID(userID), StateJson: firstNonEmpty(stateJSON, "{}"), CountryCode: params["country_code"]}
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
	result := waPrepareResult(jobID, accountID, n8nExecutionID, step, goPayAppUserID(userID), strings.TrimSpace(waPhone), out, data, err == nil)
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

func waPaymentData(params map[string]string) map[string]any {
	data := map[string]any{
		"action":              actionGoPayWAPayment,
		"otp_channel":         "wa",
		"payment_only":        true,
		"uses_account_token":  false,
		"uses_gopay_app_flow": false,
	}
	if params != nil {
		data["source_job_id"] = params["source_job_id"]
		data["user_id"] = goPayAppUserID(params["user_id"])
	}
	return data
}

func waPrepareResult(jobID string, accountID string, n8nExecutionID string, step string, userID string, waPhone string, out pb.GoPayPaymentPrepareOutput, data map[string]any, success bool) *n8nGoPayWAPaymentResult {
	return &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: step, Success: success, UserID: userID, WAPhone: waPhone, FlowID: out.GetFlowId(), CheckoutURL: out.GetCheckoutUrl(), CheckoutSessionID: out.GetCheckoutSessionId(), UseAccountToken: out.GetUseAccountToken(), StateJSON: out.GetStateJson(), SnapTokenPresent: strings.TrimSpace(out.GetSnapToken()) != "", Data: data}
}

func waOTPCheckResult(jobID string, accountID string, n8nExecutionID string, userID string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayWAPaymentResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayWAPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayWAPayment, Step: stepGoPayPayment, Success: true, UserID: goPayAppUserID(userID), OTPFound: found, OTPSource: strings.TrimSpace(source), OTPIssuedAfterUnix: issuedAfterUnix, Data: data}
}
