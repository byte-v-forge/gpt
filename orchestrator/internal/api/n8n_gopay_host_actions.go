package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"orchestrator/internal/accountauth"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	actionGoPayPayment             = "GOPAY_PAYMENT"
	actionGoPayQRISPaymentActivate = "GOPAY_QRIS_PAYMENT_ACTIVATE"
	actionGoPayWAPayment           = "GOPAY_WA_PAYMENT"
	actionGoPayPaymentRebind       = "GOPAY_PAYMENT_REBIND"

	goPayPaymentScope             = "gopay-payment"
	goPayQRISPaymentActivateScope = "gopay-qris-payment-activate"
	goPayWAPaymentScope           = "gopay-wa-payment"
	goPayPaymentRebindScope       = "gopay-payment-rebind"

	goPayEngineParam                  = "engine"
	manualGoPayPaymentConfirmParam    = "manual_gopay_payment_confirmed"
	stepGoPayPaymentPrepareCheckout   = "gopay_payment_prepare_checkout"
	stepGoPayPaymentPrepareLink       = "gopay_payment_prepare_link"
	stepGoPayPayment                  = "gopay_payment"
	stepGoPayResolveWAPhone           = "gopay_resolve_wa_phone"
	goPayDefaultPaymentFailureMessage = "gopay workflow failed"
)

type rawGoPayActionRequest struct {
	JobID             string         `json:"job_id"`
	AccountID         string         `json:"account_id"`
	N8NExecutionID    string         `json:"n8n_execution_id"`
	FlowID            string         `json:"flow_id"`
	CheckoutURL       string         `json:"checkout_url"`
	CheckoutSessionID string         `json:"checkout_session_id"`
	StateJSON         string         `json:"state_json"`
	GopayAccountID    string         `json:"gopay_account_id"`
	Phone             string         `json:"phone"`
	WAPhone           string         `json:"wa_phone"`
	CountryCode       string         `json:"country_code"`
	Tokenization      string         `json:"tokenization"`
	ChargeRef         string         `json:"charge_ref"`
	PlusTrialEligible bool           `json:"plus_trial_eligible"`
	PlusTrialChecked  bool           `json:"plus_trial_checked"`
	PlusActive        bool           `json:"plus_active"`
	ProxyURL          string         `json:"proxy_url"`
	ErrorMessage      string         `json:"error_message"`
	Data              map[string]any `json:"data"`
}

type goPayWorkflowStartRequest struct {
	AccountID      string `json:"account_id"`
	SourceJobID    string `json:"source_job_id"`
	GopayAccountID string `json:"gopay_account_id"`
	Tokenization   string `json:"tokenization"`
}

type goPayWorkflowStartResponse struct {
	JobID          string `json:"job_id"`
	Started        bool   `json:"started"`
	GopayAccountID string `json:"gopay_account_id,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

func (r goPayWorkflowStartResponse) GetStarted() bool { return r.Started }

func (r goPayWorkflowStartResponse) GetErrorMessage() string {
	return strings.TrimSpace(r.ErrorMessage)
}

type n8nGoPayPaymentStepResult struct {
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
	GopayAccountID             string         `json:"gopay_account_id,omitempty"`
	Phone                      string         `json:"phone,omitempty"`
	WAPhone                    string         `json:"wa_phone,omitempty"`
	FlowID                     string         `json:"flow_id,omitempty"`
	CheckoutURL                string         `json:"checkout_url,omitempty"`
	CheckoutSessionID          string         `json:"checkout_session_id,omitempty"`
	UseAccountToken            bool           `json:"use_account_token,omitempty"`
	StateJSON                  string         `json:"state_json,omitempty"`
	SnapToken                  string         `json:"snap_token,omitempty"`
	SnapTokenPresent           bool           `json:"snap_token_present,omitempty"`
	AwaitingManualConfirmation bool           `json:"awaiting_manual_confirmation,omitempty"`
	ManualPaymentConfirmed     bool           `json:"manual_payment_confirmed,omitempty"`
	ChargeRef                  string         `json:"charge_ref,omitempty"`
	ErrorMessage               string         `json:"error_message,omitempty"`
	Data                       map[string]any `json:"data,omitempty"`
}

func (s *Server) startN8NGoPayWorkflow(ctx context.Context, actionID string, raw []byte) (gptplugin.N8NWorkflowStartResult, error) {
	var req goPayWorkflowStartRequest
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return gptplugin.N8NWorkflowStartResult{}, fmt.Errorf("decode gopay workflow request: %w", err)
		}
	}
	jobID := uuid.NewString()
	gopayAccountID := strings.TrimSpace(req.GopayAccountID)
	if gopayAccountID == "" {
		resp := goPayWorkflowStartResponse{JobID: jobID, ErrorMessage: "gopay_account_id is required"}
		return newGoPayWorkflowStartResult(resp, "", ""), fmt.Errorf("%s", resp.ErrorMessage)
	}
	params := goPayWorkflowParams(actionID, req)
	params[goPayEngineParam] = "n8n"
	accountID := strings.TrimSpace(req.AccountID)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionID, params); err != nil {
		resp := goPayWorkflowStartResponse{JobID: jobID, GopayAccountID: gopayAccountID, ErrorMessage: err.Error()}
		return newGoPayWorkflowStartResult(resp, jobID, accountID), err
	}
	resp := goPayWorkflowStartResponse{JobID: jobID, Started: true, GopayAccountID: gopayAccountID}
	return newGoPayWorkflowStartResult(resp, jobID, accountID), nil
}

func goPayWorkflowParams(actionID string, req goPayWorkflowStartRequest) map[string]string {
	params := map[string]string{}
	if value := strings.TrimSpace(req.AccountID); value != "" {
		params["account_id"] = value
	}
	if value := strings.TrimSpace(req.SourceJobID); value != "" {
		params["source_job_id"] = value
	}
	if value := strings.TrimSpace(req.GopayAccountID); value != "" {
		params["gopay_account_id"] = value
	}
	switch actionID {
	case actionGoPayQRISPaymentActivate:
		params["activation_mode"] = "qris_payment"
		params["payment_type"] = "qris"
		params["tokenization"] = "qris"
		params["otp_channel"] = "not_required"
		params["uses_wa"] = "false"
		params["uses_gopay_app_flow"] = "false"
		params["manual_confirmation"] = "true"
		params["manual_payment_button"] = "true"
	default:
		params["tokenization"] = firstNonEmpty(req.Tokenization, "true")
	}
	return params
}

func newGoPayWorkflowStartResult(resp goPayWorkflowStartResponse, jobID string, accountID string) gptplugin.N8NWorkflowStartResult {
	payload := map[string]string{"job_id": strings.TrimSpace(jobID)}
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		payload["account_id"] = accountID
	}
	return gptplugin.N8NWorkflowStartResult{
		Response:       resp,
		JobID:          strings.TrimSpace(jobID),
		AccountID:      accountID,
		TriggerPayload: payload,
	}
}

func (s *Server) invokeN8NGoPayHostAction(ctx context.Context, actionID string, subPath string, raw []byte) (any, error) {
	var req rawGoPayActionRequest
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode gopay n8n action request: %w", err)
		}
	}
	action := strings.Trim(strings.TrimSpace(subPath), "/")
	switch actionID {
	case actionGoPayPayment:
		return s.invokeN8NGoPayPayment(ctx, actionID, action, req, goPayPaymentScope)
	case actionGoPayQRISPaymentActivate:
		return s.invokeN8NGoPayPayment(ctx, actionID, action, req, goPayQRISPaymentActivateScope)
	case actionGoPayWAPayment:
		return s.invokeN8NGoPayPayment(ctx, actionID, action, req, goPayWAPaymentScope)
	case actionGoPayPaymentRebind:
		return s.invokeN8NGoPayRebind(ctx, actionID, action, req)
	default:
		return nil, fmt.Errorf("unsupported gopay n8n action: %s", actionID)
	}
}

func (s *Server) invokeN8NGoPayPayment(ctx context.Context, actionID string, action string, req rawGoPayActionRequest, scope string) (any, error) {
	switch action {
	case "proxy-settings":
		return s.n8nDynamicProxySettings(ctx, req.JobID, req.AccountID, req.N8NExecutionID, n8nDynamicProxyProfile{Purpose: actionID})
	case "record-proxy":
		return s.recordN8NDynamicProxy(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ProxyURL, goPayDynamicProxyData(req.Data), n8nDynamicProxyProfile{Purpose: actionID})
	case "fail-proxy":
		return s.failN8NDynamicProxy(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ErrorMessage, goPayDynamicProxyData(req.Data), n8nDynamicProxyProfile{Purpose: actionID})
	case "resolve-account":
		return s.resolveN8NGoPayPaymentAccount(ctx, actionID, req)
	case "probe-plus-trial":
		return s.probeN8NGoPayPaymentPlusTrial(ctx, actionID, req)
	case "resolve-wa-phone":
		return s.resolveN8NGoPayWAPhone(ctx, actionID, req)
	case "prepare-checkout":
		return s.prepareN8NGoPayPayment(ctx, actionID, req, scope, stepGoPayPaymentPrepareCheckout)
	case "prepare-link":
		return s.prepareN8NGoPayPayment(ctx, actionID, req, scope, stepGoPayPaymentPrepareLink)
	case "check-manual-payment":
		return s.checkN8NGoPayManualPayment(ctx, actionID, req)
	case "finish":
		return s.finishN8NGoPayPayment(ctx, actionID, req)
	case "fail":
		return s.failN8NGoPayHostAction(ctx, actionID, req.JobID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported %s action: %s", scope, action)
	}
}

func (s *Server) invokeN8NGoPayRebind(ctx context.Context, actionID string, action string, req rawGoPayActionRequest) (any, error) {
	switch action {
	case "resolve-source":
		return s.resolveN8NGoPayPaymentAccount(ctx, actionID, req)
	case "finish":
		return s.finishN8NGoPayPayment(ctx, actionID, req)
	case "fail":
		return s.failN8NGoPayHostAction(ctx, actionID, req.JobID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported %s action: %s", goPayPaymentRebindScope, action)
	}
}

func (s *Server) resolveN8NGoPayPaymentAccount(ctx context.Context, actionID string, req rawGoPayActionRequest) (any, error) {
	req = normalizeRawGoPayIDs(req)
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, req.JobID)
	if err != nil {
		return nil, err
	}
	data := goPayPaymentBaseData(actionID, params)
	gopayAccountID := firstNonEmpty(req.GopayAccountID, params["gopay_account_id"])
	if gopayAccountID == "" && actionID != actionGoPayPaymentRebind {
		err := fmt.Errorf("gopay_account_id is required")
		result := goPayStepResult(actionID, "resolve_account", req, false, data)
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, "resolve_account", jobstatus.FailedFinal, false, false, err, data)
	}
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: firstNonEmpty(req.AccountID, params["account_id"]), SourceJobId: params["source_job_id"]})
	req.AccountID = strings.TrimSpace(account.GetAccountId())
	result := goPayStepResult(actionID, "resolve_account", req, err == nil, data)
	result.GopayAccountID = gopayAccountID
	if err != nil {
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
	}
	if err := s.jobStore.SetAccountID(ctx, req.JobID, req.AccountID); err != nil {
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, data)
	}
	data["account_id"] = req.AccountID
	result.Data = data
	return result, nil
}

func (s *Server) probeN8NGoPayPaymentPlusTrial(ctx context.Context, actionID string, req rawGoPayActionRequest) (any, error) {
	req = normalizeRawGoPayIDs(req)
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: req.JobID, AccountId: req.AccountID, ProxyUrl: s.protocolProxyURL(ctx, req.JobID)})
	data := protoMessageMap(probe.GetData())
	result := goPayStepResult(actionID, "probe_plus_trial", req, err == nil, data)
	result.Checked = probe.GetChecked()
	result.PlusTrialEligible = probe.GetPlusTrialEligible()
	result.PlusTrialChecked = probe.GetChecked()
	result.PlusActive = probe.GetPlusActive()
	if err != nil {
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, "probe_plus_trial", jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) resolveN8NGoPayWAPhone(ctx context.Context, actionID string, req rawGoPayActionRequest) (any, error) {
	req = normalizeRawGoPayIDs(req)
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, req.JobID)
	if err != nil {
		return nil, err
	}
	req.GopayAccountID = firstNonEmpty(req.GopayAccountID, params["gopay_account_id"])
	req.WAPhone = firstNonEmpty(req.WAPhone, params["wa_phone"], params["phone"])
	data := goPayPaymentBaseData(actionID, params)
	data["gopay_account_id"] = req.GopayAccountID
	data["wa_phone"] = req.WAPhone
	result := goPayStepResult(actionID, stepGoPayResolveWAPhone, req, req.GopayAccountID != "", data)
	if req.GopayAccountID == "" {
		err := fmt.Errorf("gopay_account_id is required")
		result.ErrorMessage = err.Error()
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, stepGoPayResolveWAPhone, jobstatus.FailedFinal, false, false, err, data)
	}
	return result, nil
}

func (s *Server) prepareN8NGoPayPayment(ctx context.Context, actionID string, req rawGoPayActionRequest, scope string, step string) (any, error) {
	req = normalizeRawGoPayIDs(req)
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, req.JobID)
	if err != nil {
		return nil, err
	}
	credential, err := s.goPayPaymentCredential(ctx, req.JobID, req.AccountID)
	if err != nil {
		data := goPayPaymentBaseData(actionID, params)
		result := goPayStepResult(actionID, step, req, false, data)
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	var resp *pb.PrepareGoPayResponse
	if step == stepGoPayPaymentPrepareCheckout {
		resp, err = s.paymentClient.PrepareGoPayCheckout(ctx, &pb.PrepareGoPayCheckoutRequest{
			Credential:        credential,
			Tokenization:      goPayPaymentTokenization(scope, req, params),
			CheckoutUrl:       strings.TrimSpace(req.CheckoutURL),
			CheckoutSessionId: strings.TrimSpace(req.CheckoutSessionID),
			GopayPhone:        firstNonEmpty(req.WAPhone, req.Phone),
			GopayCountryCode:  strings.TrimSpace(req.CountryCode),
		})
	} else {
		resp, err = s.paymentClient.PrepareGoPayLink(ctx, &pb.PrepareGoPayLinkRequest{FlowId: strings.TrimSpace(req.FlowID)})
	}
	data := goPayPrepareData(resp)
	result := goPayPrepareResult(actionID, step, req, scope, resp, data, err == nil)
	if err != nil {
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	if resp == nil {
		err := fmt.Errorf("payment service returned empty gopay prepare response")
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	if !resp.GetSuccess() {
		err := fmt.Errorf("%s", firstNonEmpty(resp.GetErrorMessage(), "gopay payment prepare failed"))
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	if resp.GetRetryableFreshCheckout() {
		err := fmt.Errorf("payment prepare link blocked: %s", firstNonEmpty(resp.GetErrorMessage(), "chatgpt approve blocked"))
		return result, s.markGoPayHostActionFailed(ctx, req.JobID, step, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) goPayPaymentCredential(ctx context.Context, jobID string, accountID string) (*pb.ChatGPTCredential, error) {
	if s.paymentClient == nil {
		return nil, fmt.Errorf("payment service is not configured")
	}
	if s.runtimeSecrets == nil {
		return nil, fmt.Errorf("runtime secret store is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	sessionToken, _, err := accountauth.LoadChatGPTSessionToken(ctx, s.runtimeSecrets, accountID)
	if err != nil {
		return nil, err
	}
	accessToken, _, err := accountauth.LoadChatGPTAccessToken(ctx, s.runtimeSecrets, accountID)
	if err != nil {
		return nil, err
	}
	proxyURL := s.protocolProxyURL(ctx, jobID)
	if accessToken == "" && sessionToken != "" {
		fetched, fetchErr := s.fetchAndCacheChatGPTAccessToken(ctx, accountID, sessionToken, proxyURL)
		if fetchErr != nil {
			return nil, fetchErr
		}
		accessToken = fetched.AccessToken
	}
	if sessionToken == "" && accessToken == "" {
		return nil, fmt.Errorf("session_token or access_token is required")
	}
	return s.paymentCredentialWithProxy(ctx, accountID, sessionToken, accessToken, proxyURL)
}

func (s *Server) checkN8NGoPayManualPayment(ctx context.Context, actionID string, req rawGoPayActionRequest) (any, error) {
	req = normalizeRawGoPayIDs(req)
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	confirmed, found, err := s.jobStore.GetParam(ctx, req.JobID, manualGoPayPaymentConfirmParam)
	if err != nil {
		return nil, err
	}
	ok := found && strings.EqualFold(strings.TrimSpace(confirmed), "true")
	if ok {
		_ = s.jobStore.DeleteParam(ctx, req.JobID, manualGoPayPaymentConfirmParam)
	}
	data := map[string]any{"manual_payment_confirmation": map[string]any{"required": true, "confirmed": ok}}
	result := goPayStepResult(actionID, stepGoPayPayment, req, true, data)
	result.FlowID = strings.TrimSpace(req.FlowID)
	result.AwaitingManualConfirmation = !ok
	result.ManualPaymentConfirmed = ok
	return result, nil
}

func (s *Server) finishN8NGoPayPayment(ctx context.Context, actionID string, req rawGoPayActionRequest) (any, error) {
	req = normalizeRawGoPayIDs(req)
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	resultData := goPayPaymentBaseData(actionID, nil)
	for key, value := range req.Data {
		resultData[key] = value
	}
	resultData["account_id"] = req.AccountID
	resultData["charge_ref"] = strings.TrimSpace(req.ChargeRef)
	resultData["payment_completed"] = strings.TrimSpace(req.ChargeRef) != ""
	resultData["plus_trial_eligible"] = req.PlusTrialEligible
	resultData["plus_trial_checked"] = req.PlusTrialChecked
	resultData["plus_active"] = req.PlusActive
	resultData["n8n_execution_id"] = req.N8NExecutionID
	tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: req.JobID, AccountId: req.AccountID, ProxyUrl: s.protocolProxyURL(ctx, req.JobID)})
	resultData["probe_tier"] = protoMessageMap(tier.GetData())
	if err != nil {
		return nil, s.markGoPayHostActionFailed(ctx, req.JobID, "probe_tier", jobstatus.FailedRecoverable, true, false, err, resultData)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: req.JobID, Result: mapJobData(resultData)}); err != nil {
		return nil, err
	}
	result := goPayStepResult(actionID, "finish", req, true, resultData)
	result.ChargeRef = strings.TrimSpace(req.ChargeRef)
	return result, nil
}

func (s *Server) failN8NGoPayHostAction(ctx context.Context, actionID string, jobID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage == "" {
		errorMessage = goPayDefaultPaymentFailureMessage
	}
	data["action"] = actionID
	data["n8n_execution_id"] = n8nExecutionID
	step := s.goPayHostFailureStep(ctx, jobID)
	if err := s.markGoPayHostActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, fmt.Errorf("%s", errorMessage), data); err != nil {
		return nil, err
	}
	return &n8nGoPayPaymentStepResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionID, Step: step, Success: false, ErrorMessage: errorMessage, Data: data}, nil
}

func (s *Server) markGoPayHostActionFailed(ctx context.Context, jobID string, step string, status string, recoverable bool, retryable bool, err error, data map[string]any) error {
	if err == nil {
		return nil
	}
	return s.storeN8NActionFailure(ctx, jobID, n8nActionFailureRecord{
		Step:          step,
		Status:        status,
		Recoverable:   recoverable,
		Retryable:     retryable,
		ErrorMessage:  err.Error(),
		ResultMessage: mapProto(data),
	})
}

func (s *Server) goPayHostFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return "run_action"
}

func normalizeRawGoPayIDs(req rawGoPayActionRequest) rawGoPayActionRequest {
	req.JobID = strings.TrimSpace(req.JobID)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.N8NExecutionID = strings.TrimSpace(req.N8NExecutionID)
	req.GopayAccountID = strings.TrimSpace(req.GopayAccountID)
	return req
}

func goPayPaymentBaseData(actionID string, params map[string]string) map[string]any {
	data := map[string]any{"action": actionID}
	switch actionID {
	case actionGoPayQRISPaymentActivate:
		data["activation_mode"] = "qris_payment"
		data["payment_type"] = "qris"
		data["tokenization"] = "qris"
		data["otp_channel"] = "not_required"
		data["uses_wa"] = false
		data["uses_gopay_app_flow"] = false
		data["uses_gopay_app_token"] = false
		data["manual_confirmation"] = true
		data["manual_payment_button"] = true
	case actionGoPayWAPayment:
		data["uses_wa"] = true
		data["uses_gopay_app_flow"] = true
		data["uses_account_token"] = false
	default:
		data["uses_gopay_app_flow"] = true
		data["uses_account_token"] = true
	}
	if params != nil {
		data["source_job_id"] = params["source_job_id"]
		data["gopay_account_id"] = strings.TrimSpace(params["gopay_account_id"])
		data["tokenization"] = firstNonEmpty(params["tokenization"], fmt.Sprint(data["tokenization"]))
	}
	return data
}

func goPayPaymentTokenization(scope string, req rawGoPayActionRequest, params map[string]string) string {
	switch scope {
	case goPayQRISPaymentActivateScope:
		return "qris"
	default:
		return firstNonEmpty(req.Tokenization, params["tokenization"], "true")
	}
}

func goPayPrepareData(resp *pb.PrepareGoPayResponse) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	return map[string]any{
		"success":                  resp.GetSuccess(),
		"error_message":            resp.GetErrorMessage(),
		"flow_id":                  resp.GetFlowId(),
		"snap_token_present":       strings.TrimSpace(resp.GetSnapToken()) != "",
		"checkout_url":             resp.GetCheckoutUrl(),
		"checkout_session_id":      resp.GetCheckoutSessionId(),
		"retryable_fresh_checkout": resp.GetRetryableFreshCheckout(),
		"checkout_attempt":         resp.GetCheckoutAttempt(),
		"stage":                    resp.GetStage(),
	}
}

func goPayPrepareResult(actionID string, step string, req rawGoPayActionRequest, scope string, resp *pb.PrepareGoPayResponse, data map[string]any, success bool) *n8nGoPayPaymentStepResult {
	result := goPayStepResult(actionID, step, req, success, data)
	result.GopayAccountID = strings.TrimSpace(req.GopayAccountID)
	result.UseAccountToken = scope == goPayPaymentScope
	result.StateJSON = firstNonEmpty(req.StateJSON, "{}")
	if resp == nil {
		return result
	}
	result.FlowID = resp.GetFlowId()
	result.CheckoutURL = resp.GetCheckoutUrl()
	result.CheckoutSessionID = resp.GetCheckoutSessionId()
	result.SnapToken = strings.TrimSpace(resp.GetSnapToken())
	result.SnapTokenPresent = result.SnapToken != ""
	result.ErrorMessage = strings.TrimSpace(resp.GetErrorMessage())
	return result
}

func goPayStepResult(actionID string, step string, req rawGoPayActionRequest, success bool, data map[string]any) *n8nGoPayPaymentStepResult {
	return &n8nGoPayPaymentStepResult{
		JobID:          strings.TrimSpace(req.JobID),
		AccountID:      strings.TrimSpace(req.AccountID),
		N8NExecutionID: strings.TrimSpace(req.N8NExecutionID),
		Action:         actionID,
		Step:           strings.TrimSpace(step),
		Success:        success,
		GopayAccountID: strings.TrimSpace(req.GopayAccountID),
		Phone:          strings.TrimSpace(req.Phone),
		WAPhone:        strings.TrimSpace(req.WAPhone),
		Data:           data,
	}
}

func protoMessageMap(message proto.Message) map[string]any {
	if message == nil {
		return map[string]any{}
	}
	if st, ok := message.(*structpb.Struct); ok {
		return st.AsMap()
	}
	raw, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func mapProto(data map[string]any) *structpb.Struct {
	if data == nil {
		data = map[string]any{}
	}
	st, err := structpb.NewStruct(data)
	if err != nil {
		return &structpb.Struct{}
	}
	return st
}

func mapJobData(data map[string]any) *pb.JobData {
	return jobDataMessage(mapProto(data))
}

func goPayDynamicProxyData(data map[string]any) *pb.N8NDynamicProxyPreflightData {
	out := &pb.N8NDynamicProxyPreflightData{}
	if data == nil {
		return out
	}
	if value := goPayDataString(data, "purpose"); value != "" {
		out.Purpose = value
	}
	if value := goPayDataString(data, "reason"); value != "" {
		out.Reason = value
	}
	if value := goPayDataString(data, "error_message"); value != "" {
		out.ErrorMessage = value
	}
	return out
}

func goPayDataString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
