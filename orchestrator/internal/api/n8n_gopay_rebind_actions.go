package api

import (
	"context"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nGoPayPaymentRebindResult struct {
	JobID               string         `json:"job_id"`
	AccountID           string         `json:"account_id,omitempty"`
	N8NExecutionID      string         `json:"n8n_execution_id,omitempty"`
	Action              string         `json:"action"`
	Step                string         `json:"step"`
	Success             bool           `json:"success"`
	SourceJobID         string         `json:"source_job_id,omitempty"`
	UserID              string         `json:"user_id,omitempty"`
	WAPhone             string         `json:"wa_phone,omitempty"`
	Phone               string         `json:"phone,omitempty"`
	ActivationID        string         `json:"activation_id,omitempty"`
	RequirePIN          bool           `json:"require_pin"`
	StateJSON           string         `json:"state_json,omitempty"`
	AccountTokenReady   bool           `json:"account_token_ready,omitempty"`
	ChangePhoneComplete bool           `json:"change_phone_complete,omitempty"`
	Data                map[string]any `json:"data,omitempty"`
}

func (s *Server) ResolveN8NGoPayPaymentRebindSource(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	source, err := s.activities.GoPayPaymentRebindSourceActivity(ctx, pb.GoPayPaymentRebindSourceInput{JobId: jobID, SourceJobId: params["source_job_id"], AccountId: firstNonEmpty(accountID, params["account_id"]), UserId: params["user_id"]})
	data := rebindBaseData(params)
	data["rebind_source"] = structMap(source.GetData())
	userID := goPayAppUserID(source.GetUserId())
	result := &n8nGoPayPaymentRebindResult{JobID: jobID, AccountID: source.GetAccountId(), N8NExecutionID: n8nExecutionID, Action: actionGoPayPaymentRebind, Step: "resolve_rebind_source", Success: err == nil, SourceJobID: source.GetSourceJobId(), UserID: userID, WAPhone: source.GetWaPhone(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_rebind_source", jobstatus.FailedRetryable, false, true, err, data)
	}
	if err := s.jobStore.SetAccountID(ctx, jobID, source.GetAccountId()); err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_rebind_source", jobstatus.FailedRetryable, false, true, err, data)
	}
	data["account_id"] = source.GetAccountId()
	data["user_id"] = userID
	data["wa_phone_present"] = strings.TrimSpace(source.GetWaPhone()) != ""
	return result, nil
}

func (s *Server) LoadN8NGoPayPaymentRebindState(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	userID = goPayAppUserID(userID)
	params, _ := s.jobStore.Params(ctx, jobID)
	stored, err := s.activities.GoPayAppLoadStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "payment_rebind_retry"})
	data := map[string]any{"load_gopay_state": structMap(stored.GetData())}
	stateJSON := firstNonEmpty(stored.GetStateJson(), "{}")
	result := &n8nGoPayPaymentRebindResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPaymentRebind, Step: "load_gopay_state", Success: err == nil, UserID: userID, WAPhone: strings.TrimSpace(params["wa_phone"]), RequirePIN: true, StateJSON: stateJSON, Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, "load_gopay_state", jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) FinishN8NGoPayPaymentRebind(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, phone string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	userID = goPayAppUserID(userID)
	result := map[string]any{
		"account_id":            accountID,
		"user_id":               userID,
		"activation_id":         strings.TrimSpace(activationID),
		"bound_phone_present":   strings.TrimSpace(phone) != "",
		"change_phone_complete": true,
		"n8n_execution_id":      n8nExecutionID,
	}
	for key, value := range data {
		result[key] = value
	}
	if err := s.finishGoPayChangePhoneSMS(ctx, jobID, activationID, "payment_rebind_retry_complete"); err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepGoPayAppSMSFinish, jobstatus.FailedRetryable, false, true, err, result)
	}
	_, _ = s.activities.GoPayAppDeleteStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "payment_rebind_complete"})
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteGoPayRuntimeSecrets(ctx, jobID)
	return &n8nGoPayPaymentRebindResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPaymentRebind, Step: "finish", Success: true, UserID: userID, ActivationID: strings.TrimSpace(activationID), Phone: strings.TrimSpace(phone), ChangePhoneComplete: true, Data: result}, nil
}

func (s *Server) FailN8NGoPayPaymentRebind(ctx context.Context, jobID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	return s.FailN8NGoPay(ctx, actionGoPayPaymentRebind, jobID, n8nExecutionID, errorMessage, data)
}

func (s *Server) goPayPaymentRebindParams(ctx context.Context, jobID string) (map[string]string, string, error) {
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

func (s *Server) saveGoPayPaymentRebindState(ctx context.Context, jobID string, userID string, stateJSON string, reason string) error {
	stateJSON = strings.TrimSpace(stateJSON)
	if stateJSON == "" || stateJSON == "{}" {
		return nil
	}
	_, err := s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: strings.TrimSpace(jobID), UserId: goPayAppUserID(userID), StateJson: stateJSON, Reason: strings.TrimSpace(reason)})
	return err
}

func rebindBaseData(params map[string]string) map[string]any {
	data := map[string]any{"action": actionGoPayPaymentRebind}
	if params != nil {
		data["source_job_id"] = params["source_job_id"]
		data["user_id"] = goPayAppUserID(params["user_id"])
	}
	return data
}
