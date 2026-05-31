package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nAuthFinishConfig struct {
	Action             string
	Driver             string
	Mode               string
	ResultSecretPrefix string
	CompleteStep       string
	MissingTokenError  string
	RequireSession     bool
	RequireAccess      bool
	IncludePlusTrial   bool
}

func (cfg n8nAuthFinishConfig) withAction(profile contracts.ActionProfile) n8nAuthFinishConfig {
	cfg.Action = profile.ActionID
	return cfg
}

func (s *Server) finishN8NAuth(ctx context.Context, cfg n8nAuthFinishConfig, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	accountID = strings.TrimSpace(accountID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = strings.TrimSpace(cfg.ResultSecretPrefix) + jobID
	}
	authResult, err := s.loadN8NAuthResult(ctx, cfg.ResultSecretPrefix, jobID, resultRef)
	if err != nil {
		return nil, s.markN8NActionFailure(ctx, jobID, n8nActionFailureRecord{Step: cfg.CompleteStep, Status: jobstatus.FailedRetryable, Retryable: true}, err)
	}
	if err := validateN8NAuthTokens(authResult, cfg); err != nil {
		return nil, s.markN8NActionFailure(ctx, jobID, n8nActionFailureRecord{Step: cfg.CompleteStep, Status: jobstatus.FailedRetryable, Retryable: true}, err)
	}
	result := n8nAuthFinishResult(accountID, n8nExecutionID, authResult, cfg)
	persist := pb.PersistRegisteredInput{
		AccountId:    accountID,
		SessionToken: authResult.GetSessionToken(),
		AccessToken:  authResult.GetAccessToken(),
	}
	if cfg.IncludePlusTrial {
		persist.PlusTrialEligible = authResult.GetPlusTrialEligible()
		persist.PlusTrialChecked = authResult.GetPlusTrialChecked()
	}
	if err := s.activities.PersistRegisteredActivity(ctx, persist); err != nil {
		return nil, s.markN8NActionFailure(ctx, jobID, n8nActionFailureRecord{Step: "persist_registered", Status: jobstatus.FailedRecoverable, Recoverable: true, ResultMessage: result}, err)
	}
	if err := s.storeN8NActionSuccessMessage(ctx, jobID, result); err != nil {
		return nil, err
	}
	s.deleteRuntimeSecretValue(ctx, resultRef)
	return n8nActionCompleteOutcomeMessage(jobID, accountID, n8nExecutionID, cfg.Action, true, true, "", result), nil
}

func validateN8NAuthTokens(result *pb.RegisterActivityOutput, cfg n8nAuthFinishConfig) error {
	missing := result == nil
	if !missing && cfg.RequireSession {
		missing = strings.TrimSpace(result.GetSessionToken()) == ""
	}
	if !missing && cfg.RequireAccess {
		missing = strings.TrimSpace(result.GetAccessToken()) == ""
	}
	if !missing {
		return nil
	}
	if message := strings.TrimSpace(cfg.MissingTokenError); message != "" {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("auth result did not contain required tokens")
}

func n8nAuthFinishResult(accountID string, n8nExecutionID string, authResult *pb.RegisterActivityOutput, cfg n8nAuthFinishConfig) *pb.N8NAuthFinishData {
	scope := n8nActionScopeFrom("", accountID, n8nExecutionID)
	result := &pb.N8NAuthFinishData{
		AccountId:           scope.AccountID,
		N8NExecutionId:      scope.N8NExecutionID,
		Driver:              strings.TrimSpace(cfg.Driver),
		Mode:                strings.TrimSpace(cfg.Mode),
		SessionTokenPresent: strings.TrimSpace(authResult.GetSessionToken()) != "",
		AccessTokenPresent:  strings.TrimSpace(authResult.GetAccessToken()) != "",
	}
	if cfg.IncludePlusTrial {
		checked := authResult.GetPlusTrialChecked()
		eligible := authResult.GetPlusTrialEligible()
		result.PlusTrialChecked = &checked
		result.PlusTrialEligible = &eligible
	}
	return result
}
