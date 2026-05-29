package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayChangePhoneResult struct {
	JobID               string         `json:"job_id"`
	AccountID           string         `json:"account_id,omitempty"`
	N8NExecutionID      string         `json:"n8n_execution_id,omitempty"`
	Action              string         `json:"action"`
	Step                string         `json:"step"`
	Operation           string         `json:"operation,omitempty"`
	Success             bool           `json:"success"`
	UserID              string         `json:"user_id,omitempty"`
	Phone               string         `json:"phone,omitempty"`
	ActivationID        string         `json:"activation_id,omitempty"`
	StateJSON           string         `json:"state_json,omitempty"`
	FailureCount        int32          `json:"failure_count,omitempty"`
	MaxFailures         int32          `json:"max_failures,omitempty"`
	RetryableFailure    bool           `json:"retryable_failure,omitempty"`
	OTPSent             bool           `json:"otp_sent,omitempty"`
	OTPFound            bool           `json:"otp_found,omitempty"`
	OTPSource           string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix  int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds   int32          `json:"otp_timeout_seconds,omitempty"`
	OTPRetryAttempt     int32          `json:"otp_retry_attempt,omitempty"`
	OTPRetryAttempts    int32          `json:"otp_retry_attempts,omitempty"`
	ChangePhoneComplete bool           `json:"change_phone_complete,omitempty"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	Data                map[string]any `json:"data,omitempty"`
}

type goPayChangePhoneScope struct {
	Action         string
	AccountID      string
	N8NExecutionID string
	Operation      string
	UserID         string
	CountryCode    string
	Pin            string
	SaveState      func(context.Context, string) error
}

func (s *Server) AcquireN8NGoPayAppChangePhoneNumber(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, failureCount int32) (any, error) {
	params, _, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayApp, N8NExecutionID: n8nExecutionID, Operation: normalizeGoPayAppOperation(operation), UserID: goPayAppUserID(userID), CountryCode: params["country_code"]}
	return s.acquireN8NGoPayChangePhoneNumber(ctx, jobID, scope, failureCount)
}

func (s *Server) StartN8NGoPayAppChangePhone(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, phone string, stateJSON string, failureCount int32) (any, error) {
	params, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	scope := goPayChangePhoneScope{Action: actionGoPayApp, N8NExecutionID: n8nExecutionID, Operation: operation, UserID: userID, CountryCode: params["country_code"], Pin: pin, SaveState: func(ctx context.Context, nextStateJSON string) error {
		return s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, nextStateJSON)
	}}
	return s.startN8NGoPayChangePhone(ctx, jobID, scope, activationID, phone, stateJSON, failureCount)
}

func (s *Server) CheckN8NGoPayAppChangePhoneOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, issuedAfterUnix int64, stateJSON string, failureCount int32, phone string) (any, error) {
	if err := s.bindN8NGoPayExecution(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID)); err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayApp, N8NExecutionID: n8nExecutionID, Operation: normalizeGoPayAppOperation(operation), UserID: goPayAppUserID(userID)}
	return s.checkN8NGoPayChangePhoneOTP(ctx, jobID, scope, activationID, issuedAfterUnix, stateJSON, failureCount, phone)
}

func (s *Server) RetryN8NGoPayAppChangePhoneOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, stateJSON string, otpAttempt int32, failureCount int32, otpTimeoutSeconds int32, phone string) (any, error) {
	if err := s.bindN8NGoPayExecution(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID)); err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayApp, N8NExecutionID: n8nExecutionID, Operation: normalizeGoPayAppOperation(operation), UserID: goPayAppUserID(userID)}
	return s.retryN8NGoPayChangePhoneOTP(ctx, jobID, scope, activationID, stateJSON, otpAttempt, failureCount, otpTimeoutSeconds, phone)
}

func (s *Server) CancelN8NGoPayAppChangePhone(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, failureCount int32, reason string) (any, error) {
	if err := s.bindN8NGoPayExecution(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID)); err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayApp, N8NExecutionID: n8nExecutionID, Operation: normalizeGoPayAppOperation(operation), UserID: goPayAppUserID(userID)}
	return s.cancelN8NGoPayChangePhone(ctx, jobID, scope, activationID, failureCount, reason)
}

func (s *Server) CompleteN8NGoPayAppChangePhone(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, stateJSON string, failureCount int32, issuedAfterUnix int64, otpSource string) (any, error) {
	if err := s.bindN8NGoPayExecution(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID)); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	scope := goPayChangePhoneScope{Action: actionGoPayApp, N8NExecutionID: n8nExecutionID, Operation: operation, UserID: userID, SaveState: func(ctx context.Context, nextStateJSON string) error {
		return s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, nextStateJSON)
	}}
	return s.completeN8NGoPayChangePhone(ctx, jobID, scope, activationID, stateJSON, failureCount, issuedAfterUnix, otpSource)
}

func (s *Server) AcquireN8NGoPayPaymentRebindChangePhoneNumber(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, failureCount int32) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, _, err := s.goPayPaymentRebindParams(ctx, jobID)
	if err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayPaymentRebind, AccountID: accountID, N8NExecutionID: n8nExecutionID, Operation: goPayAppOperationChangePhone, UserID: goPayAppUserID(firstNonEmpty(userID, params["user_id"])), CountryCode: params["country_code"]}
	return s.acquireN8NGoPayChangePhoneNumber(ctx, jobID, scope, failureCount)
}

func (s *Server) StartN8NGoPayPaymentRebindChangePhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, phone string, stateJSON string, failureCount int32) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, pin, err := s.goPayPaymentRebindParams(ctx, jobID)
	if err != nil {
		return nil, err
	}
	userID = goPayAppUserID(firstNonEmpty(userID, params["user_id"]))
	scope := goPayChangePhoneScope{Action: actionGoPayPaymentRebind, AccountID: accountID, N8NExecutionID: n8nExecutionID, Operation: goPayAppOperationChangePhone, UserID: userID, CountryCode: params["country_code"], Pin: pin, SaveState: func(ctx context.Context, nextStateJSON string) error {
		return s.saveGoPayPaymentRebindState(ctx, jobID, userID, nextStateJSON, "payment_rebind_attempt")
	}}
	return s.startN8NGoPayChangePhone(ctx, jobID, scope, activationID, phone, stateJSON, failureCount)
}

func (s *Server) CheckN8NGoPayPaymentRebindChangePhoneOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, issuedAfterUnix int64, stateJSON string, failureCount int32, phone string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayPaymentRebind, AccountID: accountID, N8NExecutionID: n8nExecutionID, Operation: goPayAppOperationChangePhone, UserID: goPayAppUserID(userID)}
	return s.checkN8NGoPayChangePhoneOTP(ctx, jobID, scope, activationID, issuedAfterUnix, stateJSON, failureCount, phone)
}

func (s *Server) RetryN8NGoPayPaymentRebindChangePhoneOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, stateJSON string, otpAttempt int32, failureCount int32, otpTimeoutSeconds int32, phone string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayPaymentRebind, AccountID: accountID, N8NExecutionID: n8nExecutionID, Operation: goPayAppOperationChangePhone, UserID: goPayAppUserID(userID)}
	return s.retryN8NGoPayChangePhoneOTP(ctx, jobID, scope, activationID, stateJSON, otpAttempt, failureCount, otpTimeoutSeconds, phone)
}

func (s *Server) CancelN8NGoPayPaymentRebindChangePhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, failureCount int32, reason string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	scope := goPayChangePhoneScope{Action: actionGoPayPaymentRebind, AccountID: accountID, N8NExecutionID: n8nExecutionID, Operation: goPayAppOperationChangePhone, UserID: goPayAppUserID(userID)}
	return s.cancelN8NGoPayChangePhone(ctx, jobID, scope, activationID, failureCount, reason)
}

func (s *Server) CompleteN8NGoPayPaymentRebindChangePhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, stateJSON string, failureCount int32, issuedAfterUnix int64, otpSource string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	userID = goPayAppUserID(userID)
	scope := goPayChangePhoneScope{Action: actionGoPayPaymentRebind, AccountID: accountID, N8NExecutionID: n8nExecutionID, Operation: goPayAppOperationChangePhone, UserID: userID, SaveState: func(ctx context.Context, nextStateJSON string) error {
		return s.saveGoPayPaymentRebindState(ctx, jobID, userID, nextStateJSON, "payment_rebind_attempt")
	}}
	return s.completeN8NGoPayChangePhone(ctx, jobID, scope, activationID, stateJSON, failureCount, issuedAfterUnix, otpSource)
}

func (s *Server) acquireN8NGoPayChangePhoneNumber(ctx context.Context, jobID string, scope goPayChangePhoneScope, failureCount int32) (any, error) {
	jobID = strings.TrimSpace(jobID)
	number, err := s.activities.GoPayAppChangePhoneGetNumberActivity(ctx, pb.GoPayAppChangePhoneGetNumberInput{JobId: jobID, FailureCount: failureCount, AccountId: scope.AccountID, UserId: scope.UserID, CountryCode: scope.CountryCode})
	result := goPayChangePhoneNumberResult(jobID, scope, number, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppChangePhoneGetNumber, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) startN8NGoPayChangePhone(ctx context.Context, jobID string, scope goPayChangePhoneScope, activationID string, phone string, stateJSON string, failureCount int32) (any, error) {
	jobID = strings.TrimSpace(jobID)
	issuedAfterUnix := time.Now().Unix()
	start, err := s.activities.GoPayAppChangePhoneStartActivity(ctx, pb.GoPayAppChangePhoneStartInput{JobId: jobID, FailureCount: failureCount, StateJson: firstNonEmpty(stateJSON, "{}"), ActivationId: strings.TrimSpace(activationID), Phone: strings.TrimSpace(phone), Pin: scope.Pin, CountryCode: scope.CountryCode})
	result := goPayChangePhoneStartResult(jobID, scope, issuedAfterUnix, start, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppChangePhoneStart, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if start.GetErrorMessage() != "" {
		result.Success = true
		return result, nil
	}
	if scope.SaveState != nil {
		if saveErr := scope.SaveState(ctx, result.StateJSON); saveErr != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, result.Data)
		}
	}
	return result, nil
}

func (s *Server) checkN8NGoPayChangePhoneOTP(ctx context.Context, jobID string, scope goPayChangePhoneScope, activationID string, issuedAfterUnix int64, stateJSON string, failureCount int32, phone string) (any, error) {
	input := goPayChangePhoneOTPWaitInput(jobID, activationID, issuedAfterUnix)
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayChangePhoneOTPCheckResult(jobID, scope, activationID, issuedAfterUnix, stateJSON, failureCount, phone, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayChangePhoneOTPCheckResult(jobID, scope, activationID, issuedAfterUnix, stateJSON, failureCount, phone, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayChangePhoneOTPCheckResult(jobID, scope, activationID, issuedAfterUnix, stateJSON, failureCount, phone, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) retryN8NGoPayChangePhoneOTP(ctx context.Context, jobID string, scope goPayChangePhoneScope, activationID string, stateJSON string, otpAttempt int32, failureCount int32, otpTimeoutSeconds int32, phone string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	issuedAfterUnix := time.Now().Unix()
	retry, err := s.activities.GoPayAppChangePhoneRetryActivity(ctx, pb.GoPayAppChangePhoneRetryInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), OtpAttempt: otpAttempt, StateJson: firstNonEmpty(stateJSON, "{}")})
	result := goPayChangePhoneRetryResult(jobID, scope, issuedAfterUnix, otpAttempt, failureCount, otpTimeoutSeconds, phone, retry, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppChangePhoneRetry, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) cancelN8NGoPayChangePhone(ctx context.Context, jobID string, scope goPayChangePhoneScope, activationID string, failureCount int32, reason string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	if strings.TrimSpace(reason) == "" {
		reason = "change phone retry"
	}
	out, err := s.activities.GoPayAppSMSCancelBeforeRotationActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), FailureCount: failureCount, Reason: reason})
	result := goPayChangePhoneCancelResult(jobID, scope, reason, out, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppChangePhoneCancel, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) completeN8NGoPayChangePhone(ctx context.Context, jobID string, scope goPayChangePhoneScope, activationID string, stateJSON string, failureCount int32, issuedAfterUnix int64, otpSource string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required")
	}
	complete, err := s.activities.GoPayAppChangePhoneCompleteActivity(ctx, pb.GoPayAppChangePhoneCompleteInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), FailureCount: failureCount, StateJson: firstNonEmpty(stateJSON, "{}"), OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix})
	result := goPayChangePhoneCompleteResult(jobID, scope, complete, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppChangePhoneComplete, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if complete.GetRetryableFailure() || !complete.GetChangePhoneComplete() {
		return result, nil
	}
	if scope.SaveState != nil {
		if saveErr := scope.SaveState(ctx, result.StateJSON); saveErr != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, result.Data)
		}
	}
	return result, nil
}

func goPayChangePhoneNumberResult(jobID string, scope goPayChangePhoneScope, output pb.GoPayAppChangePhoneGetNumberOutput, success bool) *n8nGoPayChangePhoneResult {
	return &n8nGoPayChangePhoneResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(scope.AccountID), N8NExecutionID: strings.TrimSpace(scope.N8NExecutionID), Action: scope.Action, Step: stepGoPayAppChangePhoneGetNumber, Operation: normalizeGoPayAppOperation(scope.Operation), Success: success, UserID: goPayAppUserID(scope.UserID), ActivationID: output.GetActivationId(), Phone: output.GetPhone(), FailureCount: output.GetFailureCount(), MaxFailures: output.GetMaxFailures(), Data: structMap(output.GetData())}
}

func goPayChangePhoneStartResult(jobID string, scope goPayChangePhoneScope, issuedAfterUnix int64, output pb.GoPayAppChangePhoneStartOutput, success bool) *n8nGoPayChangePhoneResult {
	data := structMap(output.GetData())
	data["otp_issued_after_unix"] = issuedAfterUnix
	return &n8nGoPayChangePhoneResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(scope.AccountID), N8NExecutionID: strings.TrimSpace(scope.N8NExecutionID), Action: scope.Action, Step: stepGoPayAppChangePhoneStart, Operation: normalizeGoPayAppOperation(scope.Operation), Success: success, UserID: goPayAppUserID(scope.UserID), ActivationID: output.GetActivationId(), Phone: output.GetPhone(), StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), FailureCount: output.GetFailureCount(), MaxFailures: output.GetMaxFailures(), RetryableFailure: output.GetRetryableFailure(), OTPSent: output.GetErrorMessage() == "", OTPIssuedAfterUnix: issuedAfterUnix, OTPTimeoutSeconds: output.GetOtpTimeoutSeconds(), OTPRetryAttempts: output.GetOtpRetryAttempts(), ErrorMessage: output.GetErrorMessage(), Data: data}
}

func goPayChangePhoneRetryResult(jobID string, scope goPayChangePhoneScope, issuedAfterUnix int64, otpAttempt int32, failureCount int32, otpTimeoutSeconds int32, phone string, output pb.GoPayAppChangePhoneRetryOutput, success bool) *n8nGoPayChangePhoneResult {
	data := structMap(output.GetData())
	data["otp_issued_after_unix"] = issuedAfterUnix
	return &n8nGoPayChangePhoneResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(scope.AccountID), N8NExecutionID: strings.TrimSpace(scope.N8NExecutionID), Action: scope.Action, Step: stepGoPayAppChangePhoneRetry, Operation: normalizeGoPayAppOperation(scope.Operation), Success: success, UserID: goPayAppUserID(scope.UserID), ActivationID: output.GetActivationId(), Phone: strings.TrimSpace(phone), StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), FailureCount: failureCount, OTPSent: output.GetOtpSent(), OTPIssuedAfterUnix: issuedAfterUnix, OTPTimeoutSeconds: otpTimeoutSeconds, OTPRetryAttempt: otpAttempt, ErrorMessage: output.GetErrorMessage(), Data: data}
}

func goPayChangePhoneCancelResult(jobID string, scope goPayChangePhoneScope, reason string, output pb.GoPayAppSMSActivationOutput, success bool) *n8nGoPayChangePhoneResult {
	data := structMap(output.GetData())
	data["reason"] = reason
	return &n8nGoPayChangePhoneResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(scope.AccountID), N8NExecutionID: strings.TrimSpace(scope.N8NExecutionID), Action: scope.Action, Step: stepGoPayAppChangePhoneCancel, Operation: normalizeGoPayAppOperation(scope.Operation), Success: success, UserID: goPayAppUserID(scope.UserID), ActivationID: output.GetActivationId(), FailureCount: output.GetFailureCount(), MaxFailures: output.GetMaxFailures(), ErrorMessage: output.GetErrorMessage(), Data: data}
}

func goPayChangePhoneCompleteResult(jobID string, scope goPayChangePhoneScope, output pb.GoPayAppChangePhoneCompleteOutput, success bool) *n8nGoPayChangePhoneResult {
	return &n8nGoPayChangePhoneResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(scope.AccountID), N8NExecutionID: strings.TrimSpace(scope.N8NExecutionID), Action: scope.Action, Step: stepGoPayAppChangePhoneComplete, Operation: normalizeGoPayAppOperation(scope.Operation), Success: success, UserID: goPayAppUserID(scope.UserID), ActivationID: output.GetActivationId(), Phone: output.GetPhone(), StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), FailureCount: output.GetFailureCount(), MaxFailures: output.GetMaxFailures(), RetryableFailure: output.GetRetryableFailure(), ChangePhoneComplete: output.GetChangePhoneComplete(), ErrorMessage: output.GetErrorMessage(), Data: structMap(output.GetData())}
}

func goPayChangePhoneOTPWaitInput(jobID string, activationID string, issuedAfterUnix int64) pb.OTPWaitInput {
	return pb.OTPWaitInput{JobId: strings.TrimSpace(jobID), StepName: stepGoPayAppChangePhoneSMSWait, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, Target: &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}}
}

func goPayChangePhoneOTPCheckResult(jobID string, scope goPayChangePhoneScope, activationID string, issuedAfterUnix int64, stateJSON string, failureCount int32, phone string, source string, found bool, data map[string]any) *n8nGoPayChangePhoneResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayChangePhoneResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(scope.AccountID), N8NExecutionID: strings.TrimSpace(scope.N8NExecutionID), Action: scope.Action, Step: stepGoPayAppChangePhoneSMSWait, Operation: normalizeGoPayAppOperation(scope.Operation), Success: true, UserID: goPayAppUserID(scope.UserID), ActivationID: strings.TrimSpace(activationID), Phone: strings.TrimSpace(phone), StateJSON: firstNonEmpty(stateJSON, "{}"), FailureCount: failureCount, OTPIssuedAfterUnix: issuedAfterUnix, OTPFound: found, OTPSource: strings.TrimSpace(source), Data: data}
}
