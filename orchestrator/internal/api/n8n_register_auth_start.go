package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/pb"
)

type n8nRegisterJobConfig struct {
	n8nActionJobConfig
	GenerateFingerprint bool
	Params              func(string, *pb.RegisterAccountRequest) map[string]string
}

func (s *Server) StartN8NRegisterAccount(ctx context.Context, actionID string, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error) {
	cfg, err := n8nRegisterStartConfigForAction(actionID)
	if err != nil {
		return n8nAccountStartResponse("", "", err, n8nRegisterStartResponse)
	}
	jobID, accountID, err := s.startN8NRegisterJob(ctx, req, cfg)
	return n8nAccountStartResponse(jobID, accountID, err, n8nRegisterStartResponse)
}

func n8nRegisterStartConfigForAction(actionID string) (n8nRegisterJobConfig, error) {
	return n8nStartConfigForAction(actionID, "register start", func(profile n8nActionRuntimeProfile) *n8nRegisterJobConfig {
		return profile.RegisterStart
	})
}

func (s *Server) startN8NRegisterJob(ctx context.Context, req *pb.RegisterAccountRequest, cfg n8nRegisterJobConfig) (string, string, error) {
	jobID := newN8NActionJobID()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	if s.activities == nil {
		return jobID, "", fmt.Errorf("GPT action API is not configured")
	}
	account, err := s.activities.EnsureAccountActivity(ctx, pb.EnsureAccountInput{Account: &pb.AccountSpec{
		AccountId:     accountID,
		Email:         req.GetEmail(),
		Password:      req.GetPassword(),
		EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()),
		CountryCode:   req.GetCountryCode(),
		Region:        req.GetRegion(),
	}})
	if err != nil {
		return jobID, "", err
	}
	accountID = account.GetAccountId()
	if cfg.GenerateFingerprint {
		if err := s.generateAccountFingerprint(ctx, accountID, accountfingerprint.GenerateParams{CountryCode: req.GetCountryCode(), Region: req.GetRegion()}); err != nil {
			return jobID, accountID, err
		}
	}
	params := map[string]string{"account_id": accountID}
	if cfg.Params != nil {
		params = cfg.Params(accountID, req)
	}
	return s.startN8NAccountActionJob(ctx, jobID, accountID, cfg.n8nActionJobConfig, params)
}
