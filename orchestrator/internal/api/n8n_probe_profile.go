package api

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const stepProbeCheckToken = "check_token"

type n8nProbeProfile struct {
	Start     n8nActionJobConfig
	TokenStep string
	Proxy     n8nDynamicProxyProfile
	Failure   n8nActionFailureStoreConfig
}

type n8nProbeScope struct {
	n8nActionScope
	ProxyURL string
}

type n8nProbeStepRunner func(context.Context, n8nProbeScope) (bool, proto.Message, error)

func n8nProbeAccountProfile() n8nProbeProfile {
	profile := contracts.ResolveActionProfile(contracts.ActionProbeAccount)
	return n8nProbeProfile{
		Start:     (n8nActionJobConfig{}).withAction(profile),
		TokenStep: stepProbeCheckToken,
		Proxy:     n8nProbeProxyProfile(),
		Failure: (n8nActionFailureStoreConfig{
			Started:             true,
			FailureStepFallback: stepProbeCheckToken,
			Status:              jobstatus.FailedRecoverable,
			Recoverable:         true,
		}).withAction(profile),
	}
}

func (s *Server) startN8NProbeJob(ctx context.Context, profile n8nProbeProfile, accountID string) (*pb.ProbeAccountResponse, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return n8nStartResponse("", fmt.Errorf("account_id is required"), n8nProbeStartResponse)
	}
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: accountID})
	if err != nil {
		return n8nStartResponse("", err, n8nProbeStartResponse)
	}
	accountID = strings.TrimSpace(account.GetAccountId())
	if accountID == "" {
		return n8nStartResponse("", fmt.Errorf("account not found"), n8nProbeStartResponse)
	}
	jobID := newN8NActionJobID()
	err = s.createN8NActionJob(ctx, profile.Start, jobID, accountID, "", map[string]string{n8nDefaultAccountIDParam: accountID})
	return n8nStartResponse(jobID, err, n8nProbeStartResponse)
}

func (s *Server) bindN8NProbeScope(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string) (n8nProbeScope, error) {
	scope := n8nProbeScope{
		n8nActionScope: n8nActionScopeFrom(jobID, accountID, n8nExecutionID),
		ProxyURL:       strings.TrimSpace(proxyURL),
	}
	if scope.JobID == "" || scope.AccountID == "" {
		return scope, fmt.Errorf("job_id and account_id are required")
	}
	if err := s.bindN8NExecution(ctx, scope.JobID, scope.N8NExecutionID); err != nil {
		return scope, err
	}
	return scope, nil
}

func (s *Server) runN8NProbeAtomicStep(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, step string, run n8nProbeStepRunner) (any, error) {
	scope, err := s.bindN8NProbeScope(ctx, jobID, accountID, n8nExecutionID, proxyURL)
	if err != nil {
		return nil, err
	}
	success, data, err := run(ctx, scope)
	result := scope.stepResultMessage(step, success, data)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Server) probePlusTrialAtomic(ctx context.Context, scope n8nProbeScope) (bool, proto.Message, error) {
	out, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: scope.JobID, AccountId: scope.AccountID, ProxyUrl: scope.ProxyURL})
	return out.GetSuccess(), out.GetData(), err
}

func (s *Server) probeTierAtomic(ctx context.Context, scope n8nProbeScope) (bool, proto.Message, error) {
	out, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: scope.JobID, AccountId: scope.AccountID, ProxyUrl: scope.ProxyURL})
	return out.GetSuccess(), out.GetData(), err
}
