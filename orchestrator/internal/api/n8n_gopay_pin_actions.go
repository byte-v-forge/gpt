package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayPINSetupResult struct {
	JobID              string         `json:"job_id"`
	AccountID          string         `json:"account_id,omitempty"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Action             string         `json:"action"`
	Step               string         `json:"step"`
	Operation          string         `json:"operation,omitempty"`
	Success            bool           `json:"success"`
	UserID             string         `json:"user_id,omitempty"`
	ActivationID       string         `json:"activation_id,omitempty"`
	OTPChannel         string         `json:"otp_channel,omitempty"`
	OTPRequired        bool           `json:"otp_required,omitempty"`
	OTPFound           bool           `json:"otp_found,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	Ready              bool           `json:"ready,omitempty"`
	AccountTokenReady  bool           `json:"account_token_ready,omitempty"`
	SignupPINComplete  bool           `json:"signup_pin_complete,omitempty"`
	StateJSON          string         `json:"state_json,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

func (s *Server) StartN8NGoPayPaymentPINSetup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, stateJSON string) (any, error) {
	return s.startN8NGoPayPINSetup(ctx, actionGoPayPayment, jobID, accountID, n8nExecutionID, "", userID, "sms", activationID, stateJSON)
}

func (s *Server) RequestN8NGoPayPaymentPINOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string) (any, error) {
	return s.requestN8NGoPayPINOTP(ctx, actionGoPayPayment, jobID, accountID, n8nExecutionID, "", userID, otpChannel, activationID)
}

func (s *Server) CheckN8NGoPayPaymentPINOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, issuedAfterUnix int64) (any, error) {
	return s.checkN8NGoPayPINOTP(ctx, actionGoPayPayment, jobID, accountID, n8nExecutionID, "", userID, otpChannel, activationID, issuedAfterUnix)
}

func (s *Server) RetryN8NGoPayPaymentPINSetup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, stateJSON string, data map[string]any) (any, error) {
	return s.retryN8NGoPayPINSetup(ctx, actionGoPayPayment, jobID, accountID, n8nExecutionID, "", userID, otpChannel, activationID, stateJSON, data)
}

func (s *Server) CompleteN8NGoPayPaymentPINSetup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	return s.completeN8NGoPayPINSetup(ctx, actionGoPayPayment, jobID, accountID, n8nExecutionID, "", userID, otpChannel, activationID, stateJSON, issuedAfterUnix, otpSource, data)
}

func (s *Server) StartN8NGoPayAppPINSetup(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, stateJSON string) (any, error) {
	return s.startN8NGoPayPINSetup(ctx, actionGoPayApp, jobID, "", n8nExecutionID, operation, userID, otpChannel, activationID, stateJSON)
}

func (s *Server) RequestN8NGoPayAppPINOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string) (any, error) {
	return s.requestN8NGoPayPINOTP(ctx, actionGoPayApp, jobID, "", n8nExecutionID, operation, userID, otpChannel, activationID)
}

func (s *Server) CheckN8NGoPayAppPINOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, issuedAfterUnix int64) (any, error) {
	return s.checkN8NGoPayPINOTP(ctx, actionGoPayApp, jobID, "", n8nExecutionID, operation, userID, otpChannel, activationID, issuedAfterUnix)
}

func (s *Server) RetryN8NGoPayAppPINSetup(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, stateJSON string, data map[string]any) (any, error) {
	return s.retryN8NGoPayPINSetup(ctx, actionGoPayApp, jobID, "", n8nExecutionID, operation, userID, otpChannel, activationID, stateJSON, data)
}

func (s *Server) CompleteN8NGoPayAppPINSetup(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	return s.completeN8NGoPayPINSetup(ctx, actionGoPayApp, jobID, "", n8nExecutionID, operation, userID, otpChannel, activationID, stateJSON, issuedAfterUnix, otpSource, data)
}

func (s *Server) StartN8NGoPayPaymentRebindPINSetup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, stateJSON string) (any, error) {
	result, err := s.startN8NGoPayPINSetup(ctx, actionGoPayPaymentRebind, jobID, accountID, n8nExecutionID, goPayAppOperationLogin, userID, "wa", "", stateJSON)
	if err == nil {
		_ = s.saveGoPayPINStateIfReady(ctx, jobID, userID, result, "payment_rebind_pin_ready")
	}
	return result, err
}

func (s *Server) CheckN8NGoPayPaymentRebindPINOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, otpChannel string, issuedAfterUnix int64) (any, error) {
	return s.checkN8NGoPayPINOTP(ctx, actionGoPayPaymentRebind, jobID, accountID, n8nExecutionID, goPayAppOperationLogin, userID, otpChannel, "", issuedAfterUnix)
}

func (s *Server) RetryN8NGoPayPaymentRebindPINSetup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, otpChannel string, stateJSON string, data map[string]any) (any, error) {
	return s.retryN8NGoPayPINSetup(ctx, actionGoPayPaymentRebind, jobID, accountID, n8nExecutionID, goPayAppOperationLogin, userID, otpChannel, "", stateJSON, data)
}

func (s *Server) CompleteN8NGoPayPaymentRebindPINSetup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, otpChannel string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	result, err := s.completeN8NGoPayPINSetup(ctx, actionGoPayPaymentRebind, jobID, accountID, n8nExecutionID, goPayAppOperationLogin, userID, otpChannel, "", stateJSON, issuedAfterUnix, otpSource, data)
	if err == nil {
		if saveErr := s.saveGoPayPINStateIfReady(ctx, jobID, userID, result, "payment_rebind_pin_ready"); saveErr != nil {
			return result, s.markActionFailed(ctx, strings.TrimSpace(jobID), "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, nil)
		}
	}
	return result, err
}

func (s *Server) startN8NGoPayPINSetup(ctx context.Context, action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, stateJSON string) (any, error) {
	params, pin, err := s.goPayPINSetupParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	otpChannel = firstNonEmpty(otpChannel, params["otp_channel"])
	activationID = strings.TrimSpace(activationID)
	start, err := s.activities.GoPayAppCreatePinStartActivity(ctx, pb.GoPayAppCreatePinStartInput{JobId: jobID, OtpChannel: otpChannel, SmsActivationId: activationID, StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := goPayPINSetupResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, stepGoPayAppEnsurePINSetup, start, nil, err == nil)
	if result.OTPChannel == "" {
		result.OTPChannel = normalizeGoPayOTPChannel(otpChannel)
	}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if !start.GetReady() && !start.GetAccountTokenReady() && !start.GetSignupPinComplete() && !start.GetOtpRequired() {
		err := fmt.Errorf("gopay create pin did not request OTP and did not become ready")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) requestN8NGoPayPINOTP(ctx context.Context, action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	otpChannel = normalizeGoPayOTPChannel(otpChannel)
	activationID = strings.TrimSpace(activationID)
	data := map[string]any{"otp_channel": otpChannel, "activation_id": activationID}
	if otpChannel == "sms" && activationID != "" {
		out, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: activationID, Reason: stepGoPayAppEnsurePINSetup})
		data = structMap(out.GetData())
		if err != nil {
			result := goPayPINSetupActionResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, stepGoPayAppEnsurePINSetup, otpChannel, data, false)
			return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, data)
		}
	}
	return goPayPINSetupActionResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, stepGoPayAppEnsurePINSetup, otpChannel, data, true), nil
}

func (s *Server) checkN8NGoPayPINOTP(ctx context.Context, action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, issuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	input := goPayPINOTPWaitInput(jobID, normalizeGoPayOTPChannel(otpChannel), activationID, userID, issuedAfterUnix)
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayPINOTPCheckResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, otpChannel, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayPINOTPCheckResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, otpChannel, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayPINOTPCheckResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, otpChannel, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) retryN8NGoPayPINSetup(ctx context.Context, action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, stateJSON string, data map[string]any) (any, error) {
	_, pin, err := s.goPayPINSetupParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	otpChannel = normalizeGoPayOTPChannel(otpChannel)
	activationID = strings.TrimSpace(activationID)
	retry, err := s.activities.GoPayAppCreatePinRetryActivity(ctx, pb.GoPayAppCreatePinStartInput{JobId: jobID, OtpChannel: otpChannel, Data: structData(data), SmsActivationId: activationID, StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := goPayPINSetupResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, stepGoPayAppEnsurePINSetup, retry, nil, err == nil)
	if result.OTPChannel == "" {
		result.OTPChannel = normalizeGoPayOTPChannel(otpChannel)
	}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if !retry.GetReady() && !retry.GetAccountTokenReady() && !retry.GetSignupPinComplete() && !retry.GetOtpRequired() {
		err := fmt.Errorf("gopay create pin retry did not request OTP")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) completeN8NGoPayPINSetup(ctx context.Context, action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	_, pin, err := s.goPayPINSetupParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	otpChannel = normalizeGoPayOTPChannel(otpChannel)
	activationID = strings.TrimSpace(activationID)
	if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required")
	}
	completed, err := s.activities.GoPayAppCreatePinCompleteActivity(ctx, pb.GoPayAppCreatePinCompleteInput{JobId: jobID, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix, OtpSource: strings.TrimSpace(otpSource), Data: structData(data), OtpChannel: otpChannel, SmsActivationId: activationID, StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := goPayPINSetupResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, stepGoPayAppEnsurePINSetup, completed, nil, err == nil)
	if result.OTPChannel == "" {
		result.OTPChannel = normalizeGoPayOTPChannel(otpChannel)
	}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppEnsurePINSetup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) goPayPINSetupParams(ctx context.Context, jobID string, n8nExecutionID string) (map[string]string, string, error) {
	if err := s.bindN8NGoPayExecution(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID)); err != nil {
		return nil, "", err
	}
	params, err := s.jobStore.Params(ctx, strings.TrimSpace(jobID))
	if err != nil {
		return nil, "", err
	}
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return nil, "", err
	}
	return params, pin, nil
}

func goPayPINSetupResult(action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, activationID string, step string, output pb.GoPayAppOTPOutput, data map[string]any, success bool) *n8nGoPayPINSetupResult {
	if data == nil {
		data = structMap(output.GetData())
	}
	channel := normalizeGoPayOTPChannel(output.GetOtpChannel())
	return &n8nGoPayPINSetupResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: action, Step: step, Operation: normalizeGoPayAppOperation(operation), Success: success, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: channel, OTPRequired: output.GetOtpRequired(), OTPIssuedAfterUnix: output.GetIssuedAfterUnix(), OTPTimeoutSeconds: output.GetTimeoutSeconds(), Ready: output.GetReady(), AccountTokenReady: output.GetAccountTokenReady(), SignupPINComplete: output.GetSignupPinComplete(), StateJSON: output.GetStateJson(), Data: data}
}

func goPayPINSetupActionResult(action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, activationID string, step string, otpChannel string, data map[string]any, success bool) *n8nGoPayPINSetupResult {
	return &n8nGoPayPINSetupResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: action, Step: step, Operation: normalizeGoPayAppOperation(operation), Success: success, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: normalizeGoPayOTPChannel(otpChannel), Data: data}
}

func goPayPINOTPWaitInput(jobID string, otpChannel string, activationID string, userID string, issuedAfterUnix int64) pb.OTPWaitInput {
	input := pb.OTPWaitInput{JobId: strings.TrimSpace(jobID), StepName: stepGoPayAppEnsurePINSetup, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam}
	if normalizeGoPayOTPChannel(otpChannel) == "sms" {
		input.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}
		return input
	}
	input.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: goPayAppUserID(userID)}}
	return input
}

func goPayPINOTPCheckResult(action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayPINSetupResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayPINSetupResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: action, Step: stepGoPayAppEnsurePINSetup, Operation: normalizeGoPayAppOperation(operation), Success: true, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: normalizeGoPayOTPChannel(otpChannel), OTPIssuedAfterUnix: issuedAfterUnix, OTPFound: found, OTPSource: strings.TrimSpace(source), Data: data}
}

func (s *Server) saveGoPayPINStateIfReady(ctx context.Context, jobID string, userID string, result any, reason string) error {
	pin, ok := result.(*n8nGoPayPINSetupResult)
	if !ok || pin == nil {
		return nil
	}
	if !pin.Ready && !pin.AccountTokenReady && !pin.SignupPINComplete {
		return nil
	}
	return s.saveGoPayPaymentRebindState(ctx, jobID, userID, pin.StateJSON, reason)
}
