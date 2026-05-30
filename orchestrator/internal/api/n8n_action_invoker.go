package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"
)

type rawN8NActionRequest struct {
	JobID             string         `json:"job_id"`
	AccountID         string         `json:"account_id"`
	N8NExecutionID    string         `json:"n8n_execution_id"`
	FlowID            string         `json:"flow_id"`
	CheckoutURL       string         `json:"checkout_url"`
	CheckoutSessionID string         `json:"checkout_session_id"`
	StateJSON         string         `json:"state_json"`
	UseAccountToken   bool           `json:"use_account_token"`
	OTPRequired       bool           `json:"otp_required"`
	OTPSource         string         `json:"otp_source"`
	OTP               string         `json:"otp"`
	Source            string         `json:"source"`
	Purpose           string         `json:"purpose"`
	ResumeURL         string         `json:"resume_url"`
	OTPIssuedAfter    int64          `json:"otp_issued_after_unix"`
	OTPReceivedAt     int64          `json:"otp_received_at_unix"`
	OTPTimeoutSeconds int32          `json:"otp_timeout_seconds"`
	OTPRetryAttempt   int32          `json:"otp_retry_attempt"`
	UserID            string         `json:"user_id"`
	Operation         string         `json:"operation"`
	Phone             string         `json:"phone"`
	OTPChannel        string         `json:"otp_channel"`
	ActivationID      string         `json:"activation_id"`
	WAPhone           string         `json:"wa_phone"`
	ChargeRef         string         `json:"charge_ref"`
	PlusTrialEligible bool           `json:"plus_trial_eligible"`
	PlusTrialChecked  bool           `json:"plus_trial_checked"`
	PlusActive        bool           `json:"plus_active"`
	FailureCount      int32          `json:"failure_count"`
	ProxyHash         string         `json:"proxy_hash"`
	DeviceFingerprint string         `json:"device_fingerprint"`
	Reason            string         `json:"reason"`
	ErrorMessage      string         `json:"error_message"`
	Data              map[string]any `json:"data"`
}

func (s *Server) InvokeN8NAction(ctx context.Context, call gptplugin.N8NActionCall) (any, error) {
	actionID := strings.ToUpper(strings.TrimSpace(call.ActionID))
	subPath := strings.Trim(strings.TrimSpace(call.SubPath), "/")
	var req rawN8NActionRequest
	if len(call.RawJSON) > 0 {
		if err := json.Unmarshal(call.RawJSON, &req); err != nil {
			return nil, fmt.Errorf("decode n8n action request: %w", err)
		}
	}
	switch actionID {
	case actionGoPayApp:
		return s.invokeN8NGoPayApp(ctx, subPath, req)
	case actionGoPayPayment:
		return s.invokeN8NGoPayPayment(ctx, subPath, req)
	case actionGoPayPaymentRebind:
		return s.invokeN8NGoPayPaymentRebind(ctx, subPath, req)
	case actionGoPayQRISPaymentActivate:
		return s.invokeN8NGoPayQRISPayment(ctx, subPath, req)
	case actionGoPayWAPayment:
		return s.invokeN8NGoPayWAPayment(ctx, subPath, req)
	default:
		return nil, fmt.Errorf("unsupported raw n8n action: %s", actionID)
	}
}

func (s *Server) invokeN8NGoPayApp(ctx context.Context, action string, req rawN8NActionRequest) (any, error) {
	switch action {
	case "load-params":
		return s.LoadN8NGoPayAppParams(ctx, req.JobID, req.N8NExecutionID)
	case "load-state":
		return s.LoadN8NGoPayAppState(ctx, req.JobID, req.N8NExecutionID, req.UserID, req.Operation)
	case "start-gopay-auth":
		return s.StartN8NGoPayAppAuth(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.Phone, req.OTPChannel, req.ActivationID, req.StateJSON)
	case "check-gopay-auth-otp":
		return s.CheckN8NGoPayAppAuthOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.OTPChannel, req.ActivationID, req.OTPIssuedAfter)
	case "await-sms-otp":
		return s.AwaitN8NSMSOTP(ctx, goPayAppSMSOTPWaitRequest(req))
	case "await-payment-otp":
		return s.AwaitN8NPaymentOTP(ctx, goPayAppPaymentOTPWaitRequest(req))
	case "resume-payment-otp":
		return s.ReceiveN8NPaymentOTP(ctx, paymentOTPReceiveRequest(req))
	case "complete-gopay-auth":
		return s.CompleteN8NGoPayAppAuth(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.Phone, req.OTPChannel, req.ActivationID, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "start-signup":
		return s.StartN8NGoPayAppSignup(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.Phone, req.OTPChannel, req.ActivationID, req.StateJSON)
	case "check-signup-otp":
		return s.CheckN8NGoPayAppSignupOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.OTPChannel, req.OTPIssuedAfter)
	case "retry-signup":
		return s.RetryN8NGoPayAppSignup(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.OTPChannel, req.StateJSON, req.Data)
	case "request-signup-otp":
		return s.RequestN8NGoPayAppSignupOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.OTPChannel)
	case "complete-signup":
		return s.CompleteN8NGoPayAppSignup(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.OTPChannel, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "start-deactivate":
		return s.StartN8NGoPayAppDeactivate(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.StateJSON)
	case "check-deactivate-otp":
		return s.CheckN8NGoPayAppDeactivateOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.OTPIssuedAfter)
	case "complete-deactivate":
		return s.CompleteN8NGoPayAppDeactivate(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.StateJSON, req.OTPIssuedAfter, req.OTPSource)
	case "finish-sms":
		return s.FinishN8NGoPayAppSMS(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.Reason)
	case "start-pin", "start-gopay-pin":
		return s.StartN8NGoPayAppPINSetup(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.OTPChannel, req.ActivationID, req.StateJSON)
	case "request-pin-otp":
		return s.RequestN8NGoPayAppPINOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.OTPChannel, req.ActivationID)
	case "check-pin-otp", "check-gopay-pin-otp":
		return s.CheckN8NGoPayAppPINOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.OTPChannel, req.ActivationID, req.OTPIssuedAfter)
	case "retry-pin", "retry-gopay-pin":
		return s.RetryN8NGoPayAppPINSetup(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.OTPChannel, req.ActivationID, req.StateJSON, req.Data)
	case "complete-pin", "complete-gopay-pin":
		return s.CompleteN8NGoPayAppPINSetup(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.OTPChannel, req.ActivationID, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "check-balance":
		return s.CheckBalanceN8NGoPayApp(ctx, req.JobID, req.N8NExecutionID, req.UserID, req.Operation, req.StateJSON)
	case "check-pin":
		return s.CheckPINN8NGoPayApp(ctx, req.JobID, req.N8NExecutionID, req.UserID, req.Operation, req.StateJSON)
	case "acquire-change-phone-number":
		return s.AcquireN8NGoPayAppChangePhoneNumber(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.FailureCount)
	case "start-change-phone":
		return s.StartN8NGoPayAppChangePhone(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.Phone, req.StateJSON, req.FailureCount)
	case "check-change-phone-otp":
		return s.CheckN8NGoPayAppChangePhoneOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.OTPIssuedAfter, req.StateJSON, req.FailureCount, req.Phone)
	case "retry-change-phone-otp":
		return s.RetryN8NGoPayAppChangePhoneOTP(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.StateJSON, req.OTPRetryAttempt, req.FailureCount, req.OTPTimeoutSeconds, req.Phone)
	case "cancel-change-phone":
		return s.CancelN8NGoPayAppChangePhone(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.FailureCount, firstNonEmpty(req.Reason, req.ErrorMessage))
	case "complete-change-phone":
		return s.CompleteN8NGoPayAppChangePhone(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.ActivationID, req.StateJSON, req.FailureCount, req.OTPIssuedAfter, req.OTPSource)
	case "finish":
		return s.FinishN8NGoPayApp(ctx, req.JobID, req.N8NExecutionID, req.Operation, req.UserID, req.StateJSON, req.Data)
	case "fail":
		return s.FailN8NGoPay(ctx, actionGoPayApp, req.JobID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported gopay app action: %s", action)
	}
}

func (s *Server) invokeN8NGoPayPaymentRebind(ctx context.Context, action string, req rawN8NActionRequest) (any, error) {
	switch action {
	case "resolve-source":
		return s.ResolveN8NGoPayPaymentRebindSource(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "load-state":
		return s.LoadN8NGoPayPaymentRebindState(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID)
	case "start-gopay-auth":
		return s.StartN8NGoPayPaymentRebindAuth(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.WAPhone, req.StateJSON)
	case "check-gopay-auth-otp":
		return s.CheckN8NGoPayPaymentRebindAuthOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.OTPChannel, req.OTPIssuedAfter)
	case "await-sms-otp":
		return s.AwaitN8NSMSOTP(ctx, goPayPaymentRebindSMSOTPWaitRequest(req))
	case "await-payment-otp":
		return s.AwaitN8NPaymentOTP(ctx, goPayPaymentRebindPaymentOTPWaitRequest(req))
	case "resume-payment-otp":
		return s.ReceiveN8NPaymentOTP(ctx, paymentOTPReceiveRequest(req))
	case "complete-gopay-auth":
		return s.CompleteN8NGoPayPaymentRebindAuth(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.WAPhone, req.OTPChannel, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "start-gopay-pin":
		return s.StartN8NGoPayPaymentRebindPINSetup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.StateJSON)
	case "check-gopay-pin-otp":
		return s.CheckN8NGoPayPaymentRebindPINOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.OTPChannel, req.OTPIssuedAfter)
	case "retry-gopay-pin":
		return s.RetryN8NGoPayPaymentRebindPINSetup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.OTPChannel, req.StateJSON, req.Data)
	case "complete-gopay-pin":
		return s.CompleteN8NGoPayPaymentRebindPINSetup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.OTPChannel, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "acquire-change-phone-number":
		return s.AcquireN8NGoPayPaymentRebindChangePhoneNumber(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.FailureCount)
	case "start-change-phone":
		return s.StartN8NGoPayPaymentRebindChangePhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.Phone, req.StateJSON, req.FailureCount)
	case "check-change-phone-otp":
		return s.CheckN8NGoPayPaymentRebindChangePhoneOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPIssuedAfter, req.StateJSON, req.FailureCount, req.Phone)
	case "retry-change-phone-otp":
		return s.RetryN8NGoPayPaymentRebindChangePhoneOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.StateJSON, req.OTPRetryAttempt, req.FailureCount, req.OTPTimeoutSeconds, req.Phone)
	case "cancel-change-phone":
		return s.CancelN8NGoPayPaymentRebindChangePhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.FailureCount, firstNonEmpty(req.Reason, req.ErrorMessage))
	case "complete-change-phone":
		return s.CompleteN8NGoPayPaymentRebindChangePhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.StateJSON, req.FailureCount, req.OTPIssuedAfter, req.OTPSource)
	case "finish":
		return s.FinishN8NGoPayPaymentRebind(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.Phone, req.Data)
	case "fail":
		return s.FailN8NGoPayPaymentRebind(ctx, req.JobID, req.N8NExecutionID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported gopay payment rebind action: %s", action)
	}
}

func (s *Server) invokeN8NGoPayPayment(ctx context.Context, action string, req rawN8NActionRequest) (any, error) {
	switch action {
	case "resolve-account":
		return s.ResolveN8NGoPayPaymentAccount(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "probe-plus-trial":
		return s.ProbeN8NGoPayPaymentPlusTrial(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "acquire-signup-phone":
		return s.AcquireN8NGoPayPaymentSignupPhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FailureCount)
	case "generate-signup-device-proxy":
		return s.GenerateN8NGoPayPaymentSignupDeviceProxy(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "check-signup-phone":
		return s.CheckN8NGoPayPaymentSignupPhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ActivationID, req.Phone, req.StateJSON, req.ProxyHash, req.DeviceFingerprint)
	case "start-signup":
		return s.StartN8NGoPayPaymentSignup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.Phone, req.StateJSON)
	case "check-signup-otp":
		return s.CheckN8NGoPayPaymentSignupOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel, req.OTPIssuedAfter)
	case "await-sms-otp":
		return s.AwaitN8NSMSOTP(ctx, goPayPaymentSMSOTPWaitRequest(req))
	case "await-payment-otp":
		return s.AwaitN8NPaymentOTP(ctx, goPayPaymentPaymentOTPWaitRequest(req))
	case "resume-payment-otp":
		return s.ReceiveN8NPaymentOTP(ctx, paymentOTPReceiveRequest(req))
	case "retry-signup":
		return s.RetryN8NGoPayPaymentSignup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel, req.StateJSON, req.Data)
	case "request-signup-otp":
		return s.RequestN8NGoPayPaymentSignupOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel)
	case "complete-signup":
		return s.CompleteN8NGoPayPaymentSignup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "discard-signup-phone":
		return s.DiscardN8NGoPayPaymentSignupPhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ActivationID, req.FailureCount, firstNonEmpty(req.Reason, req.ErrorMessage))
	case "start-pin":
		return s.StartN8NGoPayPaymentPINSetup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.StateJSON)
	case "request-pin-otp":
		return s.RequestN8NGoPayPaymentPINOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel)
	case "check-pin-otp":
		return s.CheckN8NGoPayPaymentPINOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel, req.OTPIssuedAfter)
	case "retry-pin":
		return s.RetryN8NGoPayPaymentPINSetup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel, req.StateJSON, req.Data)
	case "complete-pin":
		return s.CompleteN8NGoPayPaymentPINSetup(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.ActivationID, req.OTPChannel, req.StateJSON, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "check-balance-ready":
		return s.CheckN8NGoPayPaymentBalanceReady(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.StateJSON)
	case "check-add-balance-selection":
		return s.CheckN8NGoPayPaymentAddBalanceSelection(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "apply-add-balance":
		return s.ApplyN8NGoPayPaymentAddBalance(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.StateJSON, req.Phone)
	case "check-manual-add-balance":
		return s.CheckN8NGoPayPaymentManualAddBalanceConfirmation(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.StateJSON)
	case "prepare-checkout":
		return s.PrepareN8NGoPayPaymentCheckout(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.Phone, req.StateJSON)
	case "prepare-link":
		return s.PrepareN8NGoPayPaymentLink(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.Phone, req.FlowID, req.CheckoutURL, req.CheckoutSessionID, req.StateJSON)
	case "start-payment":
		return s.StartN8NGoPayPaymentStep(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.Phone, req.ActivationID, req.FlowID, req.StateJSON)
	case "check-payment-otp":
		return s.CheckN8NGoPayPaymentOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ActivationID, req.OTPIssuedAfter)
	case "complete-payment":
		return s.CompleteN8NGoPayPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.StateJSON, req.UseAccountToken, req.OTPRequired, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "finish":
		return s.FinishN8NGoPayPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ChargeRef, req.Data)
	case "fail":
		return s.FailN8NGoPayPayment(ctx, req.JobID, req.N8NExecutionID, req.FlowID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported gopay payment action: %s", action)
	}
}

func (s *Server) invokeN8NGoPayQRISPayment(ctx context.Context, action string, req rawN8NActionRequest) (any, error) {
	switch action {
	case "resolve-account":
		return s.ResolveN8NGoPayQRISPaymentAccount(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "probe-plus-trial":
		return s.ProbeN8NGoPayQRISPaymentPlusTrial(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "prepare-checkout":
		return s.PrepareN8NGoPayQRISPaymentCheckout(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "prepare-link":
		return s.PrepareN8NGoPayQRISPaymentLink(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.CheckoutURL, req.CheckoutSessionID, req.StateJSON)
	case "start-payment":
		return s.StartN8NGoPayQRISPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.StateJSON)
	case "complete-payment":
		return s.CompleteN8NGoPayQRISPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.StateJSON, req.UseAccountToken, req.Data)
	case "check-manual-payment":
		return s.CheckN8NGoPayQRISManualPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID)
	case "confirm-manual-payment":
		return s.ConfirmN8NGoPayQRISManualPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.StateJSON, req.Data)
	case "finish":
		return s.FinishN8NGoPayQRISPaymentActivate(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ChargeRef, req.PlusTrialEligible, req.PlusTrialChecked, req.PlusActive, req.Data)
	case "fail":
		return s.FailN8NGoPayQRISPaymentActivate(ctx, req.JobID, req.N8NExecutionID, req.FlowID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported gopay qris action: %s", action)
	}
}

func (s *Server) invokeN8NGoPayWAPayment(ctx context.Context, action string, req rawN8NActionRequest) (any, error) {
	switch action {
	case "resolve-account":
		return s.ResolveN8NGoPayWAPaymentAccount(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "probe-plus-trial":
		return s.ProbeN8NGoPayWAPaymentPlusTrial(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "resolve-wa-phone":
		return s.ResolveN8NGoPayWAPaymentPhone(ctx, req.JobID, req.AccountID, req.N8NExecutionID)
	case "prepare-checkout":
		return s.PrepareN8NGoPayWAPaymentCheckout(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.WAPhone)
	case "prepare-link":
		return s.PrepareN8NGoPayWAPaymentLink(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.WAPhone, req.FlowID, req.CheckoutURL, req.CheckoutSessionID, req.StateJSON)
	case "start-payment":
		return s.StartN8NGoPayWAPaymentStep(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.WAPhone, req.FlowID, req.StateJSON)
	case "check-payment-otp":
		return s.CheckN8NGoPayWAPaymentOTP(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.UserID, req.OTPIssuedAfter)
	case "await-payment-otp":
		return s.AwaitN8NPaymentOTP(ctx, goPayWAPaymentOTPWaitRequest(req))
	case "resume-payment-otp":
		return s.ReceiveN8NPaymentOTP(ctx, paymentOTPReceiveRequest(req))
	case "complete-payment":
		return s.CompleteN8NGoPayWAPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.FlowID, req.StateJSON, req.UseAccountToken, req.OTPRequired, req.OTPIssuedAfter, req.OTPSource, req.Data)
	case "finish":
		return s.FinishN8NGoPayWAPayment(ctx, req.JobID, req.AccountID, req.N8NExecutionID, req.ChargeRef, req.PlusTrialEligible, req.PlusTrialChecked, req.PlusActive, req.Data)
	case "fail":
		return s.FailN8NGoPayWAPayment(ctx, req.JobID, req.N8NExecutionID, req.FlowID, req.ErrorMessage, req.Data)
	default:
		return nil, fmt.Errorf("unsupported gopay wa action: %s", action)
	}
}
