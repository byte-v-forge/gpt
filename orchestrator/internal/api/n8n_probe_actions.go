package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"orchestrator/internal/accountauth"
	"orchestrator/internal/chatgptauth"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type N8NProbeTokenResult struct {
	JobID             string         `json:"job_id"`
	AccountID         string         `json:"account_id"`
	N8NExecutionID    string         `json:"n8n_execution_id,omitempty"`
	Step              string         `json:"step"`
	TokenValid        bool           `json:"token_valid"`
	RequiresLogin     bool           `json:"requires_login"`
	AccessTokenCached bool           `json:"access_token_cached"`
	ExpiresAtUnix     int64          `json:"expires_at_unix,omitempty"`
	TTLSeconds        int64          `json:"ttl_seconds,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	Account           map[string]any `json:"account"`
}

type N8NProbeStepResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Step           string         `json:"step"`
	Success        bool           `json:"success"`
	Data           map[string]any `json:"data"`
}

type N8NProbeCompleteInput struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id"`
	PlusTrial      map[string]any `json:"plus_trial"`
	Tier           map[string]any `json:"tier"`
}

type N8NProbeCompleteResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Action         string         `json:"action"`
	Started        bool           `json:"started"`
	Success        bool           `json:"success"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Result         map[string]any `json:"result"`
}

type N8NProbeFailureResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Action         string         `json:"action"`
	Started        bool           `json:"started"`
	Success        bool           `json:"success"`
	ErrorMessage   string         `json:"error_message"`
	Result         map[string]any `json:"result"`
}

func (s *Server) StartN8NProbeAccount(ctx context.Context, accountID string) (*pb.ProbeAccountResponse, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: accountID})
	if err != nil {
		return nil, err
	}
	accountID = strings.TrimSpace(account.GetAccountId())
	if accountID == "" {
		return nil, fmt.Errorf("account not found")
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithIDWithoutDispatch(ctx, jobID, accountID, actionProbeAccount, map[string]string{"account_id": accountID, "engine": "n8n"}); err != nil {
		return nil, err
	}
	return &pb.ProbeAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) CheckN8NProbeToken(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	accountID = strings.TrimSpace(accountID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if jobID == "" || accountID == "" {
		return nil, fmt.Errorf("job_id and account_id are required")
	}
	if err := s.bindN8NProbeExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	accountResp, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		return nil, err
	}
	account := accountResp.GetAccount()
	if account == nil || strings.TrimSpace(account.GetAccountId()) == "" {
		return nil, fmt.Errorf("account not found")
	}
	result := &N8NProbeTokenResult{
		JobID:          jobID,
		AccountID:      account.GetAccountId(),
		N8NExecutionID: n8nExecutionID,
		Step:           "check_token",
		Account: probeAccountFacts(pb.AccountRef{
			AccountId:         account.GetAccountId(),
			PlusTrialKnown:    account.PlusTrialEligible != nil,
			PlusTrialEligible: account.GetPlusTrialEligible(),
			PlusActive:        account.GetPlusActive(),
			Tier:              account.GetTier(),
		}),
	}
	if s == nil || s.runtimeSecrets == nil {
		return nil, fmt.Errorf("runtime secret store is not configured")
	}
	cached, found, err := accountauth.LoadChatGPTAccessToken(ctx, s.runtimeSecrets, accountID)
	if err != nil {
		return nil, err
	}
	if found && strings.TrimSpace(cached) != "" {
		result.TokenValid = true
		result.AccessTokenCached = true
		return result, nil
	}
	sessionToken, _, err := accountauth.LoadChatGPTSessionToken(ctx, s.runtimeSecrets, accountID)
	if err != nil {
		return nil, err
	}
	if sessionToken == "" {
		result.RequiresLogin = true
		result.Reason = "session_token_missing"
		return result, nil
	}
	if s.paymentClient == nil {
		return nil, fmt.Errorf("payment service is not configured")
	}
	credential, err := s.paymentCredential(ctx, accountID, sessionToken, "")
	if err != nil {
		result.RequiresLogin = true
		result.Reason = "account_fingerprint_missing"
		return result, nil
	}
	fetched, err := s.paymentClient.FetchAccessToken(ctx, &pb.FetchAccessTokenPaymentRequest{Credential: credential})
	if err != nil || fetched == nil || !fetched.GetSuccess() || strings.TrimSpace(fetched.GetAccessToken()) == "" {
		result.RequiresLogin = true
		result.Reason = "access_token_fetch_failed"
		return result, nil
	}
	accessToken := fetched.GetAccessToken()
	ttl, expiresAt, ok := chatgptauth.AccessTokenTTL(accessToken, time.Now())
	if !ok {
		result.RequiresLogin = true
		result.ExpiresAtUnix = expiresAt
		result.Reason = "access_token_expired"
		return result, nil
	}
	if err := accountauth.SaveChatGPTAccessToken(ctx, s.runtimeSecrets, accountID, accessToken); err != nil {
		return nil, err
	}
	result.TokenValid = true
	result.AccessTokenCached = true
	result.ExpiresAtUnix = expiresAt
	result.TTLSeconds = int64(ttl.Seconds())
	return result, nil
}

func (s *Server) RunN8NProbePlusTrial(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	accountID = strings.TrimSpace(accountID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if jobID == "" || accountID == "" {
		return nil, fmt.Errorf("job_id and account_id are required")
	}
	if err := s.bindN8NProbeExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID})
	result := &N8NProbeStepResult{
		JobID:          jobID,
		AccountID:      accountID,
		N8NExecutionID: n8nExecutionID,
		Step:           stepProbePlusTrial,
		Success:        out.GetSuccess(),
		Data:           structMap(out.GetData()),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Server) RunN8NProbeTier(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	accountID = strings.TrimSpace(accountID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if jobID == "" || accountID == "" {
		return nil, fmt.Errorf("job_id and account_id are required")
	}
	if err := s.bindN8NProbeExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: accountID})
	result := &N8NProbeStepResult{
		JobID:          jobID,
		AccountID:      accountID,
		N8NExecutionID: n8nExecutionID,
		Step:           stepProbeTier,
		Success:        out.GetSuccess(),
		Data:           structMap(out.GetData()),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Server) FailN8NProbeAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	jobID = strings.TrimSpace(jobID)
	accountID = strings.TrimSpace(accountID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	errorMessage = strings.TrimSpace(errorMessage)
	if jobID == "" || accountID == "" {
		return nil, fmt.Errorf("job_id and account_id are required")
	}
	if err := s.bindN8NProbeExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if errorMessage == "" {
		errorMessage = "probe account failed"
	}
	result := map[string]any{
		"account_id":       accountID,
		"n8n_execution_id": n8nExecutionID,
		"failure":          data,
	}
	if err := s.activities.MarkJobFailedActivity(ctx, pb.JobFailureInput{
		JobId:        jobID,
		StepName:     "check_token",
		Status:       jobstatus.FailedRecoverable,
		Recoverable:  true,
		Retryable:    false,
		ErrorMessage: errorMessage,
		Result:       structData(result),
	}); err != nil {
		return nil, err
	}
	return &N8NProbeFailureResult{
		JobID:          jobID,
		AccountID:      accountID,
		N8NExecutionID: n8nExecutionID,
		Action:         actionProbeAccount,
		Started:        true,
		Success:        false,
		ErrorMessage:   errorMessage,
		Result:         result,
	}, nil
}

func (s *Server) CompleteN8NProbeAccount(ctx context.Context, jobID string, accountID string, n8nExecutionID string, plusTrial map[string]any, tier map[string]any) (any, error) {
	input := N8NProbeCompleteInput{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, PlusTrial: plusTrial, Tier: tier}
	jobID = strings.TrimSpace(input.JobID)
	accountID = strings.TrimSpace(input.AccountID)
	n8nExecutionID = strings.TrimSpace(input.N8NExecutionID)
	if jobID == "" || accountID == "" {
		return nil, fmt.Errorf("job_id and account_id are required")
	}
	if err := s.bindN8NProbeExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	result := map[string]any{
		"account_id":       accountID,
		"n8n_execution_id": n8nExecutionID,
		"probe_plus_trial": nestedData(input.PlusTrial),
		"probe_tier":       nestedData(input.Tier),
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	success := truthy(input.PlusTrial["success"]) && truthy(input.Tier["success"])
	message := ""
	if !success {
		message = "probe completed with unsuccessful result"
	}
	return &N8NProbeCompleteResult{
		JobID:          jobID,
		AccountID:      accountID,
		N8NExecutionID: n8nExecutionID,
		Action:         actionProbeAccount,
		Started:        true,
		Success:        success,
		ErrorMessage:   message,
		Result:         result,
	}, nil
}

func probeAccountFacts(account pb.AccountRef) map[string]any {
	return map[string]any{
		"account_id":               account.GetAccountId(),
		"plus_trial_known":         account.GetPlusTrialKnown(),
		"plus_trial_already_known": account.GetPlusTrialKnown(),
		"plus_trial_eligible":      account.GetPlusTrialEligible(),
		"tier":                     account.GetTier(),
		"plus_active":              account.GetPlusActive(),
	}
}

func (s *Server) bindN8NProbeExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func nestedData(value map[string]any) map[string]any {
	if data, ok := value["data"].(map[string]any); ok {
		return data
	}
	return value
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
