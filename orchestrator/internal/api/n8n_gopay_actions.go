package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const goPayEngineParam = "engine"

type n8nGoPayActionResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id,omitempty"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Action         string         `json:"action"`
	Step           string         `json:"step"`
	Success        bool           `json:"success"`
	Status         string         `json:"status,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	Snapshot       any            `json:"snapshot,omitempty"`
}

func (s *Server) StartN8NGoPayApp(ctx context.Context, req *pb.GoPayAppRequest) (*pb.GoPayAppResponse, string, error) {
	jobID := uuid.NewString()
	pinSecretKey, err := s.saveGoPayPINSecret(ctx, goPayAppPinSecretKey, jobID, req.GetPin())
	if err != nil {
		return &pb.GoPayAppResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	params := goPayAppJobParams(req, pinSecretKey)
	params[goPayEngineParam] = "n8n"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, "", actionGoPayApp, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayAppResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	return &pb.GoPayAppResponse{JobId: jobID, Started: true}, "", nil
}

func (s *Server) StartN8NGoPayPayment(ctx context.Context, req *pb.GoPayPaymentRequest) (*pb.GoPayPaymentResponse, string, error) {
	jobID := uuid.NewString()
	otpChannel := strings.TrimSpace(req.GetOtpChannel())
	if otpChannel == "" {
		otpChannel = "sms"
	}
	addBalance := cloneGoPayAddBalance(req.GetAddBalance())
	if addBalance != nil {
		addBalance = s.mergeDefaultGoPayAddBalance(ctx, addBalance)
	}
	pinSecretKey, err := s.saveGoPayPINSecret(ctx, goPayPaymentPinSecretKey, jobID, req.GetPin())
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	params, err := goPayPaymentJobParams(req, otpChannel, addBalance, s.goPayAddBalanceConfirmTimeout(ctx, req.GetAddBalanceConfirmTimeoutSeconds()), pinSecretKey)
	if err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	params[goPayEngineParam] = "n8n"
	accountID := strings.TrimSpace(req.GetAccountId())
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionGoPayPayment, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) StartN8NGoPayQRISPaymentActivate(ctx context.Context, req *pb.GoPayQRISPaymentActivateRequest) (*pb.GoPayPaymentResponse, string, error) {
	jobID := uuid.NewString()
	params := goPayQRISPaymentJobParams(req)
	params[goPayEngineParam] = "n8n"
	accountID := strings.TrimSpace(req.GetAccountId())
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionGoPayQRISPaymentActivate, params); err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) StartN8NGoPayWAPayment(ctx context.Context, req *pb.GoPayWAPaymentRequest) (*pb.GoPayPaymentResponse, string, error) {
	jobID := uuid.NewString()
	pinSecretKey, err := s.saveGoPayPINSecret(ctx, goPayWAPaymentPinSecretKey, jobID, req.GetPin())
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	accessTokenSecretKey := ""
	if accessToken := strings.TrimSpace(req.GetAccessToken()); accessToken != "" {
		accessTokenSecretKey = goPayWAPaymentAccessTokenSecretKey + jobID
		if err := s.saveRuntimeSecretValue(ctx, accessTokenSecretKey, accessToken); err != nil {
			s.deleteRuntimeSecretValue(ctx, pinSecretKey)
			return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
		}
	}
	params := goPayWAPaymentJobParams(req, pinSecretKey, accessTokenSecretKey)
	params[goPayEngineParam] = "n8n"
	accountID := strings.TrimSpace(req.GetAccountId())
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionGoPayWAPayment, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		s.deleteRuntimeSecretValue(ctx, accessTokenSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) StartN8NGoPayPaymentRebind(ctx context.Context, req *pb.GoPayPaymentRebindRequest) (*pb.GoPayPaymentResponse, string, error) {
	jobID := uuid.NewString()
	sourceJobID := strings.TrimSpace(req.GetSourceJobId())
	if sourceJobID == "" {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: "source_job_id is required"}, "", fmt.Errorf("source_job_id is required")
	}
	if s.activities == nil {
		err := fmt.Errorf("GPT action API is not configured")
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	source, err := s.activities.GoPayPaymentRebindSourceActivity(ctx, pb.GoPayPaymentRebindSourceInput{
		JobId:       jobID,
		SourceJobId: sourceJobID,
		AccountId:   strings.TrimSpace(req.GetAccountId()),
		UserId:      strings.TrimSpace(req.GetUserId()),
	})
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	pinSecretKey, err := s.saveGoPayPINSecret(ctx, goPayPaymentRebindPinSecretKey, jobID, req.GetPin())
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, source.GetAccountId(), err
	}
	params := goPayPaymentRebindJobParams(source, strings.TrimSpace(req.GetCountryCode()), pinSecretKey)
	params[goPayEngineParam] = "n8n"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, source.GetAccountId(), actionGoPayPaymentRebind, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, source.GetAccountId(), err
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, source.GetAccountId(), nil
}

type n8nGoPayQRISPaymentResult struct {
	JobID                      string         `json:"job_id"`
	AccountID                  string         `json:"account_id,omitempty"`
	N8NExecutionID             string         `json:"n8n_execution_id,omitempty"`
	Action                     string         `json:"action"`
	Step                       string         `json:"step"`
	Success                    bool           `json:"success"`
	Checked                    bool           `json:"checked,omitempty"`
	PlusTrialEligible          bool           `json:"plus_trial_eligible,omitempty"`
	PlusTrialChecked           bool           `json:"plus_trial_checked,omitempty"`
	PlusActive                 bool           `json:"plus_active,omitempty"`
	CheckoutURL                string         `json:"checkout_url,omitempty"`
	CheckoutSessionID          string         `json:"checkout_session_id,omitempty"`
	UseAccountToken            bool           `json:"use_account_token,omitempty"`
	OTPRequired                bool           `json:"otp_required,omitempty"`
	OTPIssuedAfterUnix         int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds          int32          `json:"otp_timeout_seconds,omitempty"`
	AwaitingManualConfirmation bool           `json:"awaiting_manual_confirmation,omitempty"`
	ManualPaymentConfirmed     bool           `json:"manual_payment_confirmed,omitempty"`
	ChargeRef                  string         `json:"charge_ref,omitempty"`
	SnapTokenPresent           bool           `json:"snap_token_present,omitempty"`
	FlowID                     string         `json:"flow_id,omitempty"`
	StateJSON                  string         `json:"state_json,omitempty"`
	Data                       map[string]any `json:"data,omitempty"`
}

func (s *Server) ResolveN8NGoPayQRISPaymentAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	data := qrisPaymentData(params)
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: firstNonEmpty(accountID, params["account_id"]), SourceJobId: params["source_job_id"]})
	result := &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: account.GetAccountId(), N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: "resolve_account", Success: err == nil, Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
	}
	if err := s.jobStore.SetAccountID(ctx, jobID, account.GetAccountId()); err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
	}
	data["account_id"] = account.GetAccountId()
	return result, nil
}

func (s *Server) ProbeN8NGoPayQRISPaymentPlusTrial(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID})
	data := structMap(probe.GetData())
	result := &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: stepProbePlusTrial, Success: err == nil, Checked: probe.GetChecked(), PlusTrialEligible: probe.GetPlusTrialEligible(), PlusTrialChecked: probe.GetChecked(), PlusActive: probe.GetPlusActive(), CheckoutURL: probe.GetCheckoutUrl(), CheckoutSessionID: probe.GetCheckoutSessionId(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) PrepareN8NGoPayQRISPaymentCheckout(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	checkout, err := s.activities.GoPayPaymentPrepareCheckoutActivity(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, Tokenization: "qris", StateJson: "{}"})
	data := structMap(checkout.GetData())
	result := qrisPrepareResult(jobID, accountID, n8nExecutionID, stepGoPayPaymentPrepareCheckout, checkout, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayPaymentPrepareCheckout, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) PrepareN8NGoPayQRISPaymentLink(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, checkoutURL string, checkoutSessionID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	link, err := s.activities.GoPayPaymentPrepareLinkActivity(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, Tokenization: "qris", PreparedFlowId: strings.TrimSpace(flowID), CheckoutUrl: strings.TrimSpace(checkoutURL), CheckoutSessionId: strings.TrimSpace(checkoutSessionID), StateJson: firstNonEmpty(stateJSON, "{}")})
	data := structMap(link.GetData())
	result := qrisPrepareResult(jobID, accountID, n8nExecutionID, stepGoPayPaymentPrepareLink, link, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayPaymentPrepareLink, jobstatus.FailedRetryable, false, true, err, data)
	}
	if link.GetRetryableFreshCheckout() {
		message := strings.TrimSpace(stringMapValue(data, "error_message"))
		if message == "" {
			message = "chatgpt approve blocked"
		}
		err := fmt.Errorf("payment prepare link blocked: %s", message)
		return result, s.markActionFailed(ctx, jobID, stepGoPayPaymentPrepareLink, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) StartN8NGoPayQRISPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	start, err := s.activities.GoPayPaymentStartActivity(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: accountID, Tokenization: "qris", PreparedFlowId: strings.TrimSpace(flowID), StateJson: firstNonEmpty(stateJSON, "{}")})
	data := structMap(start.GetData())
	result := &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: stepGoPayPayment, Success: err == nil, FlowID: paymentFlowID(start.GetFlowId(), flowID), StateJSON: start.GetStateJson(), UseAccountToken: start.GetUseAccountToken(), OTPRequired: start.GetOtpRequired(), OTPIssuedAfterUnix: start.GetIssuedAfterUnix(), OTPTimeoutSeconds: start.GetOtpTimeoutSeconds(), Data: data}
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(start.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) CompleteN8NGoPayQRISPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, stateJSON string, useAccountToken bool, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	payment, err := s.activities.GoPayPaymentCompleteActivity(ctx, pb.GoPayPaymentCompleteInput{JobId: jobID, AccountId: accountID, FlowId: strings.TrimSpace(flowID), OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, OtpSource: "not_required", UseAccountToken: useAccountToken, Data: structData(data), StateJson: firstNonEmpty(stateJSON, "{}"), WaitForManualConfirmation: true})
	result := qrisPaymentOutputResult(jobID, accountID, n8nExecutionID, payment, err == nil)
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(payment.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, structMap(payment.GetData()))
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayQRISManualPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	confirmed, found, err := s.jobStore.GetParam(ctx, jobID, manualGoPayPaymentConfirmParam)
	if err != nil {
		return nil, err
	}
	ok := found && strings.EqualFold(strings.TrimSpace(confirmed), "true")
	if ok {
		_ = s.jobStore.DeleteParam(ctx, jobID, manualGoPayPaymentConfirmParam)
	}
	data := map[string]any{"manual_payment_confirmation": map[string]any{"required": true, "confirmed": ok}}
	return &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: stepGoPayPayment, Success: true, FlowID: strings.TrimSpace(flowID), AwaitingManualConfirmation: !ok, ManualPaymentConfirmed: ok, Data: data}, nil
}

func (s *Server) ConfirmN8NGoPayQRISManualPayment(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, stateJSON string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	confirmed, err := s.activities.GoPayPaymentManualConfirmActivity(ctx, pb.GoPayPaymentManualConfirmInput{JobId: jobID, AccountId: accountID, FlowId: strings.TrimSpace(flowID), Data: structData(data), StateJson: firstNonEmpty(stateJSON, "{}")})
	result := qrisPaymentOutputResult(jobID, accountID, n8nExecutionID, confirmed, err == nil)
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(confirmed.GetFlowId(), flowID))
		return result, s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, structMap(confirmed.GetData()))
	}
	return result, nil
}

func (s *Server) FinishN8NGoPayQRISPaymentActivate(ctx context.Context, jobID string, accountID string, n8nExecutionID string, chargeRef string, plusTrialEligible bool, plusTrialChecked bool, plusActive bool, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	result := qrisPaymentData(nil)
	for key, value := range data {
		result[key] = value
	}
	result["account_id"] = accountID
	result["charge_ref"] = strings.TrimSpace(chargeRef)
	result["plus_trial_eligible"] = plusTrialEligible
	result["plus_trial_checked"] = plusTrialChecked
	result["plus_active"] = plusActive
	result["n8n_execution_id"] = n8nExecutionID
	tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: accountID})
	mergeActionData(result, "probe_tier", structMap(tier.GetData()))
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepProbeTier, jobstatus.FailedRecoverable, true, false, err, result)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	return &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: "finish", Success: true, ChargeRef: strings.TrimSpace(chargeRef), PlusTrialEligible: plusTrialEligible, PlusTrialChecked: plusTrialChecked, PlusActive: plusActive, Data: result}, nil
}

func qrisPaymentData(params map[string]string) map[string]any {
	data := map[string]any{
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
	if params != nil {
		data["source_job_id"] = params["source_job_id"]
	}
	return data
}

func qrisPrepareResult(jobID string, accountID string, n8nExecutionID string, step string, out pb.GoPayPaymentPrepareOutput, data map[string]any, success bool) *n8nGoPayQRISPaymentResult {
	return &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: step, Success: success, FlowID: out.GetFlowId(), CheckoutURL: out.GetCheckoutUrl(), CheckoutSessionID: out.GetCheckoutSessionId(), UseAccountToken: out.GetUseAccountToken(), StateJSON: out.GetStateJson(), SnapTokenPresent: strings.TrimSpace(out.GetSnapToken()) != "", Data: data}
}

func qrisPaymentOutputResult(jobID string, accountID string, n8nExecutionID string, out pb.GoPayActivityOutput, success bool) *n8nGoPayQRISPaymentResult {
	return &n8nGoPayQRISPaymentResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayQRISPaymentActivate, Step: stepGoPayPayment, Success: success, FlowID: out.GetFlowId(), StateJSON: out.GetStateJson(), AwaitingManualConfirmation: out.GetAwaitingManualConfirmation(), ChargeRef: out.GetChargeRef(), SnapTokenPresent: strings.TrimSpace(out.GetSnapToken()) != "", PlusTrialEligible: out.GetPlusTrialEligible(), PlusTrialChecked: out.GetPlusTrialChecked(), PlusActive: out.GetPlusActive(), Data: structMap(out.GetData())}
}

func normalizeN8NGoPayQRISIDs(jobID string, accountID string, n8nExecutionID string) (string, string, string) {
	return strings.TrimSpace(jobID), strings.TrimSpace(accountID), strings.TrimSpace(n8nExecutionID)
}

func (s *Server) FailN8NGoPayQRISPaymentActivate(ctx context.Context, jobID string, n8nExecutionID string, flowID string, errorMessage string, data map[string]any) (any, error) {
	if strings.TrimSpace(flowID) != "" {
		s.cancelGoPayPaymentAction(ctx, flowID)
	}
	return s.FailN8NGoPay(ctx, actionGoPayQRISPaymentActivate, jobID, n8nExecutionID, errorMessage, data)
}

func (s *Server) FailN8NGoPay(ctx context.Context, action string, jobID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	action = strings.ToUpper(strings.TrimSpace(action))
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage == "" {
		errorMessage = "gopay workflow failed"
	}
	data["action"] = action
	data["n8n_execution_id"] = strings.TrimSpace(n8nExecutionID)
	step := s.goPayFailureStep(ctx, jobID)
	if err := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, fmt.Errorf("%s", errorMessage), data); err != nil {
		return nil, err
	}
	s.deleteGoPayRuntimeSecrets(ctx, jobID)
	return &n8nGoPayActionResult{JobID: jobID, N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: action, Step: step, Success: false, ErrorMessage: errorMessage, Data: data}, nil
}

func (s *Server) saveGoPayPINSecret(ctx context.Context, prefix string, jobID string, pin string) (string, error) {
	pin = strings.TrimSpace(pin)
	if pin == "" {
		return "", nil
	}
	key := strings.TrimSpace(prefix) + strings.TrimSpace(jobID)
	if err := s.saveRuntimeSecretValue(ctx, key, pin); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) bindN8NGoPayExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) goPayFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return "run_action"
}

func (s *Server) deleteGoPayRuntimeSecrets(ctx context.Context, jobID string) {
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return
	}
	for _, key := range []string{"pin_secret_key", "access_token_secret_key"} {
		s.deleteRuntimeSecretValue(ctx, params[key])
	}
}
