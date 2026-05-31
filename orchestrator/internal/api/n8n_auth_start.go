package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
)

func n8nStartConfigForAction[T any](actionID string, kind string, pick func(n8nActionRuntimeProfile) *T) (T, error) {
	profile, err := n8nRuntimeProfileForAction(kind, actionID)
	if err != nil {
		var zero T
		return zero, err
	}
	cfg := pick(profile)
	if cfg == nil {
		var zero T
		return zero, unsupportedN8NAuthActionError(kind, profile.ActionID)
	}
	return *cfg, nil
}

func (s *Server) startN8NAccountActionJob(ctx context.Context, jobID string, accountID string, cfg n8nActionJobConfig, params map[string]string) (string, string, error) {
	email, err := s.n8nAuthAccountEmail(ctx, accountID)
	if err != nil {
		return jobID, accountID, err
	}
	if err := s.createN8NActionJob(ctx, cfg, jobID, accountID, email, params); err != nil {
		return jobID, accountID, err
	}
	return jobID, accountID, nil
}

func (s *Server) n8nAuthAccountEmail(ctx context.Context, accountID string) (string, error) {
	account, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: strings.TrimSpace(accountID)})
	if err != nil {
		return "", err
	}
	if account.GetAccount() == nil {
		return "", fmt.Errorf("account not found")
	}
	return gptaccount.Email(account.GetAccount()), nil
}
