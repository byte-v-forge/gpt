package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayAppResult struct {
	JobID               string         `json:"job_id"`
	N8NExecutionID      string         `json:"n8n_execution_id,omitempty"`
	Action              string         `json:"action"`
	Step                string         `json:"step"`
	Operation           string         `json:"operation,omitempty"`
	Success             bool           `json:"success"`
	UserID              string         `json:"user_id,omitempty"`
	Phone               string         `json:"phone,omitempty"`
	OTPChannel          string         `json:"otp_channel,omitempty"`
	OTPRequired         bool           `json:"otp_required,omitempty"`
	OTPSent             bool           `json:"otp_sent,omitempty"`
	OTPFound            bool           `json:"otp_found,omitempty"`
	OTPSource           string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix  int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds   int32          `json:"otp_timeout_seconds,omitempty"`
	CountryCode         string         `json:"country_code,omitempty"`
	ActivationID        string         `json:"activation_id,omitempty"`
	RetryableFailure    bool           `json:"retryable_failure,omitempty"`
	FailureCount        int32          `json:"failure_count,omitempty"`
	MaxFailures         int32          `json:"max_failures,omitempty"`
	OTPRetryAttempt     int32          `json:"otp_retry_attempt,omitempty"`
	OTPRetryAttempts    int32          `json:"otp_retry_attempts,omitempty"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	RequirePIN          bool           `json:"require_pin"`
	StateJSON           string         `json:"state_json,omitempty"`
	Ready               bool           `json:"ready,omitempty"`
	AccountTokenReady   bool           `json:"account_token_ready,omitempty"`
	ChangePhoneComplete bool           `json:"change_phone_complete,omitempty"`
	DeactivateComplete  bool           `json:"deactivate_complete,omitempty"`
	SignupComplete      bool           `json:"signup_complete,omitempty"`
	SignupPINComplete   bool           `json:"signup_pin_complete,omitempty"`
	Data                map[string]any `json:"data,omitempty"`
}

func (s *Server) LoadN8NGoPayAppParams(ctx context.Context, jobID string, n8nExecutionID string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	operation := normalizeGoPayAppOperation(params["operation"])
	userID := goPayAppUserID(params["user_id"])
	data := goPayAppBaseData(params, operation, userID)
	return &n8nGoPayAppResult{
		JobID:          jobID,
		N8NExecutionID: n8nExecutionID,
		Action:         actionGoPayApp,
		Step:           "load_params",
		Operation:      operation,
		Success:        true,
		UserID:         userID,
		Phone:          strings.TrimSpace(params["phone"]),
		OTPChannel:     normalizeGoPayOTPChannel(params["otp_channel"]),
		CountryCode:    strings.TrimSpace(params["country_code"]),
		RequirePIN:     operation == goPayAppOperationProvision,
		StateJSON:      "{}",
		Data:           data,
	}, nil
}

func (s *Server) LoadN8NGoPayAppState(ctx context.Context, jobID string, n8nExecutionID string, userID string, operation string) (any, error) {
	jobID, _, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, "", n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	params, _ := s.jobStore.Params(ctx, jobID)
	stored, err := s.activities.GoPayAppLoadStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "gopay_app_" + operation})
	data := map[string]any{"load_gopay_state": structMap(stored.GetData())}
	stateJSON := firstNonEmpty(stored.GetStateJson(), "{}")
	result := &n8nGoPayAppResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionGoPayApp, Step: "load_gopay_state", Operation: operation, Success: err == nil, UserID: userID, Phone: strings.TrimSpace(params["phone"]), OTPChannel: normalizeGoPayOTPChannel(params["otp_channel"]), CountryCode: strings.TrimSpace(params["country_code"]), RequirePIN: operation == goPayAppOperationProvision, StateJSON: stateJSON, Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, "load_gopay_state", jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) CheckBalanceN8NGoPayApp(ctx context.Context, jobID string, n8nExecutionID string, userID string, operation string, stateJSON string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	status, err := s.activities.GoPayAppStatusActivity(ctx, pb.GoPayAppStepInput{JobId: jobID, StateJson: firstNonEmpty(stateJSON, "{}")})
	data := map[string]any{"status": structMap(status.GetData())}
	result := s.goPayAppStepResult(jobID, n8nExecutionID, operation, userID, stepGoPayAppStatus, status, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppStatus, jobstatus.FailedRetryable, false, true, err, data)
	}
	if !status.GetAccountTokenReady() {
		err := fmt.Errorf("gopay app token is not ready")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppStatus, jobstatus.FailedRetryable, false, true, err, data)
	}
	statusData := structMap(status.GetData())
	if snapshot, ok := statusData["status"].(map[string]any); ok {
		for _, key := range []string{"balance_amount", "balance_currency", "has_min_balance"} {
			if value, ok := snapshot[key]; ok {
				data[key] = value
			}
		}
	}
	if saveErr := s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, result.StateJSON); saveErr != nil {
		return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, data)
	}
	return result, nil
}

func (s *Server) CheckPINN8NGoPayApp(ctx context.Context, jobID string, n8nExecutionID string, userID string, operation string, stateJSON string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	status, err := s.activities.GoPayAppStatusActivity(ctx, pb.GoPayAppStepInput{JobId: jobID, StateJson: firstNonEmpty(stateJSON, "{}")})
	data := map[string]any{"status": structMap(status.GetData()), "pin_setup": status.GetSignupPinComplete()}
	result := s.goPayAppStepResult(jobID, n8nExecutionID, operation, userID, stepGoPayAppStatus, status, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppStatus, jobstatus.FailedRetryable, false, true, err, data)
	}
	if !status.GetAccountTokenReady() {
		err := fmt.Errorf("gopay app token is not ready")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppStatus, jobstatus.FailedRetryable, false, true, err, data)
	}
	if saveErr := s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, result.StateJSON); saveErr != nil {
		return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, data)
	}
	return result, nil
}

func (s *Server) FinishN8NGoPayApp(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, stateJSON string, data map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	if data == nil {
		data = map[string]any{}
	}
	data["action"] = actionGoPayApp
	data["operation"] = operation
	data["user_id"] = userID
	data["n8n_execution_id"] = n8nExecutionID
	if stateJSON = strings.TrimSpace(stateJSON); stateJSON != "" {
		data["state_json_present"] = true
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(data)}); err != nil {
		return nil, err
	}
	s.deleteGoPayRuntimeSecrets(ctx, jobID)
	return &n8nGoPayAppResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionGoPayApp, Step: "finish", Operation: operation, Success: true, UserID: userID, StateJSON: stateJSON, Data: data}, nil
}

func (s *Server) goPayAppStepParams(ctx context.Context, jobID string, n8nExecutionID string) (map[string]string, string, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, "", fmt.Errorf("job_id is required")
	}
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, "", err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, "", err
	}
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return nil, "", err
	}
	return params, pin, nil
}

func (s *Server) goPayAppStepResult(jobID string, n8nExecutionID string, operation string, userID string, step string, out pb.GoPayAppStepOutput, data map[string]any, success bool) *n8nGoPayAppResult {
	return &n8nGoPayAppResult{
		JobID:               strings.TrimSpace(jobID),
		N8NExecutionID:      strings.TrimSpace(n8nExecutionID),
		Action:              actionGoPayApp,
		Step:                step,
		Operation:           normalizeGoPayAppOperation(operation),
		Success:             success,
		UserID:              goPayAppUserID(userID),
		Phone:               out.GetPhone(),
		ActivationID:        out.GetActivationId(),
		StateJSON:           firstNonEmpty(out.GetStateJson(), "{}"),
		Ready:               out.GetReady(),
		AccountTokenReady:   out.GetAccountTokenReady(),
		ChangePhoneComplete: out.GetChangePhoneComplete(),
		DeactivateComplete:  out.GetDeactivateComplete(),
		SignupComplete:      out.GetSignupComplete(),
		SignupPINComplete:   out.GetSignupPinComplete(),
		Data:                data,
	}
}

func (s *Server) saveGoPayAppStateForUserOperation(ctx context.Context, jobID string, userID string, operation string, stateJSON string) error {
	operation = normalizeGoPayAppOperation(operation)
	if operation == goPayAppOperationProvision {
		return nil
	}
	stateJSON = strings.TrimSpace(stateJSON)
	if stateJSON == "" || stateJSON == "{}" {
		return nil
	}
	_, err := s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: goPayAppUserID(userID), StateJson: stateJSON, Reason: "gopay_app_" + operation})
	return err
}

func goPayAppBaseData(params map[string]string, operation string, userID string) map[string]any {
	data := map[string]any{"action": actionGoPayApp, "operation": normalizeGoPayAppOperation(operation), "user_id": goPayAppUserID(userID)}
	if params != nil {
		data["phone_present"] = strings.TrimSpace(params["phone"]) != ""
		data["otp_channel"] = normalizeGoPayOTPChannel(params["otp_channel"])
		data["country_code_present"] = strings.TrimSpace(params["country_code"]) != ""
	}
	return data
}
