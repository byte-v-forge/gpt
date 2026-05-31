package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) StartN8NLoginSessionAccount(ctx context.Context, actionID string, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error) {
	cfg, err := n8nLoginSessionStartConfigForAction(actionID)
	if err != nil {
		return n8nAccountStartResponse("", "", err, n8nLoginStartResponse)
	}
	jobID, accountID, err := s.startN8NLoginJob(ctx, req, cfg)
	return n8nAccountStartResponse(jobID, accountID, err, n8nLoginStartResponse)
}

func n8nLoginSessionStartConfigForAction(actionID string) (n8nActionJobConfig, error) {
	return n8nStartConfigForAction(actionID, "login start", func(profile n8nActionRuntimeProfile) *n8nActionJobConfig {
		return profile.LoginStart
	})
}

func (s *Server) startN8NLoginJob(ctx context.Context, req *pb.LoginAccountRequest, cfg n8nActionJobConfig) (string, string, error) {
	jobID := newN8NActionJobID()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return jobID, "", fmt.Errorf("account_id is required")
	}
	return s.startN8NAccountActionJob(ctx, jobID, accountID, cfg, map[string]string{n8nDefaultAccountIDParam: accountID})
}
