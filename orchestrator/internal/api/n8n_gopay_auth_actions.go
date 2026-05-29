package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayAuthResult struct {
	JobID              string         `json:"job_id"`
	AccountID          string         `json:"account_id,omitempty"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Action             string         `json:"action"`
	Step               string         `json:"step"`
	Operation          string         `json:"operation,omitempty"`
	Success            bool           `json:"success"`
	UserID             string         `json:"user_id,omitempty"`
	Phone              string         `json:"phone,omitempty"`
	WAPhone            string         `json:"wa_phone,omitempty"`
	ActivationID       string         `json:"activation_id,omitempty"`
	OTPChannel         string         `json:"otp_channel,omitempty"`
	OTPRequired        bool           `json:"otp_required,omitempty"`
	OTPFound           bool           `json:"otp_found,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	Ready              bool           `json:"ready,omitempty"`
	AccountTokenReady  bool           `json:"account_token_ready,omitempty"`
	UseAccountToken    bool           `json:"use_account_token,omitempty"`
	StateJSON          string         `json:"state_json,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

type goPayAuthStartParams struct {
	Action         string
	AccountID      string
	N8NExecutionID string
	Operation      string
	UserID         string
	Phone          string
	WAPhone        string
	OTPChannel     string
	ActivationID   string
	StateJSON      string
	Pin            string
	CountryCode    string
	SaveState      func(context.Context, string) error
}

func (s *Server) StartN8NGoPayAppAuth(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, phone string, otpChannel string, activationID string, stateJSON string) (any, error) {
	params, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	return s.startN8NGoPayAuth(ctx, jobID, goPayAuthStartParams{
		Action:         actionGoPayApp,
		N8NExecutionID: n8nExecutionID,
		Operation:      operation,
		UserID:         userID,
		Phone:          firstNonEmpty(phone, params["phone"]),
		OTPChannel:     firstNonEmpty(otpChannel, params["otp_channel"]),
		ActivationID:   activationID,
		StateJSON:      stateJSON,
		Pin:            pin,
		CountryCode:    params["country_code"],
		SaveState: func(ctx context.Context, nextStateJSON string) error {
			return s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, nextStateJSON)
		},
	})
}

func (s *Server) CheckN8NGoPayAppAuthOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, issuedAfterUnix int64) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	return s.checkN8NGoPayAuthOTP(ctx, actionGoPayApp, jobID, "", n8nExecutionID, normalizeGoPayAppOperation(operation), goPayAppUserID(userID), otpChannel, activationID, issuedAfterUnix)
}

func (s *Server) CompleteN8NGoPayAppAuth(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, phone string, otpChannel string, activationID string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	params, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	return s.completeN8NGoPayAuth(ctx, jobID, goPayAuthStartParams{
		Action:         actionGoPayApp,
		N8NExecutionID: n8nExecutionID,
		Operation:      operation,
		UserID:         userID,
		Phone:          firstNonEmpty(phone, params["phone"]),
		OTPChannel:     firstNonEmpty(otpChannel, params["otp_channel"]),
		ActivationID:   activationID,
		StateJSON:      stateJSON,
		Pin:            pin,
		CountryCode:    params["country_code"],
		SaveState: func(ctx context.Context, nextStateJSON string) error {
			return s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, nextStateJSON)
		},
	}, issuedAfterUnix, otpSource, data)
}

func (s *Server) StartN8NGoPayPaymentRebindAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, waPhone string, stateJSON string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, pin, err := s.goPayPaymentRebindParams(ctx, jobID)
	if err != nil {
		return nil, err
	}
	userID = goPayAppUserID(firstNonEmpty(userID, params["user_id"]))
	return s.startN8NGoPayAuth(ctx, jobID, goPayAuthStartParams{
		Action:         actionGoPayPaymentRebind,
		AccountID:      accountID,
		N8NExecutionID: n8nExecutionID,
		Operation:      goPayAppOperationLogin,
		UserID:         userID,
		WAPhone:        firstNonEmpty(waPhone, params["wa_phone"]),
		OTPChannel:     "wa",
		StateJSON:      stateJSON,
		Pin:            pin,
		CountryCode:    params["country_code"],
		SaveState: func(ctx context.Context, nextStateJSON string) error {
			return s.saveGoPayPaymentRebindState(ctx, jobID, userID, nextStateJSON, "payment_rebind_login_ready")
		},
	})
}

func (s *Server) CheckN8NGoPayPaymentRebindAuthOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, otpChannel string, issuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	return s.checkN8NGoPayAuthOTP(ctx, actionGoPayPaymentRebind, jobID, accountID, n8nExecutionID, goPayAppOperationLogin, goPayAppUserID(userID), otpChannel, "", issuedAfterUnix)
}

func (s *Server) CompleteN8NGoPayPaymentRebindAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, waPhone string, otpChannel string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, pin, err := s.goPayPaymentRebindParams(ctx, jobID)
	if err != nil {
		return nil, err
	}
	userID = goPayAppUserID(firstNonEmpty(userID, params["user_id"]))
	return s.completeN8NGoPayAuth(ctx, jobID, goPayAuthStartParams{
		Action:         actionGoPayPaymentRebind,
		AccountID:      accountID,
		N8NExecutionID: n8nExecutionID,
		Operation:      goPayAppOperationLogin,
		UserID:         userID,
		WAPhone:        firstNonEmpty(waPhone, params["wa_phone"]),
		OTPChannel:     firstNonEmpty(otpChannel, "wa"),
		StateJSON:      stateJSON,
		Pin:            pin,
		CountryCode:    params["country_code"],
		SaveState: func(ctx context.Context, nextStateJSON string) error {
			return s.saveGoPayPaymentRebindState(ctx, jobID, userID, nextStateJSON, "payment_rebind_login_ready")
		},
	}, issuedAfterUnix, otpSource, data)
}

func (s *Server) startN8NGoPayAuth(ctx context.Context, jobID string, params goPayAuthStartParams) (any, error) {
	jobID = strings.TrimSpace(jobID)
	phone := firstNonEmpty(params.Phone, params.WAPhone)
	start, err := s.activities.GoPayAppOTPStartActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "auth", StepName: stepGoPayAppLogin, Phone: phone, OtpChannel: params.OTPChannel, SmsActivationId: params.ActivationID, StateJson: firstNonEmpty(params.StateJSON, "{}"), Pin: params.Pin, CountryCode: params.CountryCode})
	result := goPayAuthResult(jobID, params, start, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppLogin, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if result.AccountTokenReady && params.SaveState != nil {
		if err := params.SaveState(ctx, result.StateJSON); err != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, err, result.Data)
		}
	}
	return result, nil
}

func (s *Server) completeN8NGoPayAuth(ctx context.Context, jobID string, params goPayAuthStartParams, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required")
	}
	completed, err := s.activities.GoPayAppOTPCompleteActivity(ctx, pb.GoPayAppOTPCompleteInput{JobId: jobID, Operation: "auth", OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix, OtpSource: strings.TrimSpace(otpSource), Data: structData(data), OtpChannel: normalizeGoPayOTPChannel(params.OTPChannel), SmsActivationId: params.ActivationID, StateJson: firstNonEmpty(params.StateJSON, "{}"), Pin: params.Pin})
	result := goPayAuthResult(jobID, params, completed, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppLogin, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if result.AccountTokenReady && params.SaveState != nil {
		if err := params.SaveState(ctx, result.StateJSON); err != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, err, result.Data)
		}
	}
	return result, nil
}

func (s *Server) checkN8NGoPayAuthOTP(ctx context.Context, action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, otpChannel string, activationID string, issuedAfterUnix int64) (any, error) {
	input := goPayAuthOTPWaitInput(jobID, stepGoPayAppLogin, otpChannel, activationID, userID, issuedAfterUnix)
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayAuthOTPCheckResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, otpChannel, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayAuthOTPCheckResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, otpChannel, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayAuthOTPCheckResult(action, jobID, accountID, n8nExecutionID, operation, userID, activationID, otpChannel, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func goPayAuthOTPWaitInput(jobID string, stepName string, otpChannel string, activationID string, userID string, issuedAfterUnix int64) pb.OTPWaitInput {
	input := pb.OTPWaitInput{JobId: strings.TrimSpace(jobID), StepName: strings.TrimSpace(stepName), TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam}
	if normalizeGoPayOTPChannel(otpChannel) == "sms" {
		input.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}
		return input
	}
	input.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: goPayAppUserID(userID)}}
	return input
}

func goPayAuthResult(jobID string, params goPayAuthStartParams, output pb.GoPayAppOTPOutput, success bool) *n8nGoPayAuthResult {
	ready := output.GetReady() || output.GetAccountTokenReady()
	channel := normalizeGoPayOTPChannel(output.GetOtpChannel())
	if channel == "" {
		channel = normalizeGoPayOTPChannel(params.OTPChannel)
	}
	return &n8nGoPayAuthResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(params.AccountID), N8NExecutionID: strings.TrimSpace(params.N8NExecutionID), Action: params.Action, Step: stepGoPayAppLogin, Operation: normalizeGoPayAppOperation(params.Operation), Success: success, UserID: goPayAppUserID(params.UserID), Phone: firstNonEmpty(output.GetPhone(), params.Phone), WAPhone: strings.TrimSpace(params.WAPhone), ActivationID: strings.TrimSpace(params.ActivationID), OTPChannel: channel, OTPRequired: output.GetOtpRequired(), OTPIssuedAfterUnix: output.GetIssuedAfterUnix(), OTPTimeoutSeconds: output.GetTimeoutSeconds(), Ready: ready, AccountTokenReady: ready, UseAccountToken: ready, StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), Data: structMap(output.GetData())}
}

func goPayAuthOTPCheckResult(action string, jobID string, accountID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayAuthResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayAuthResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: action, Step: stepGoPayAppLogin, Operation: normalizeGoPayAppOperation(operation), Success: true, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: normalizeGoPayOTPChannel(otpChannel), OTPIssuedAfterUnix: issuedAfterUnix, OTPFound: found, OTPSource: strings.TrimSpace(source), Data: data}
}
