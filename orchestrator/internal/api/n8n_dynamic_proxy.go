package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"orchestrator/internal/accountproxyusage"
	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

func (s *Server) RecordN8NDynamicProxy(ctx context.Context, actionID string, req *pb.N8NDynamicProxyRecordRequest) (any, error) {
	profile, err := n8nDynamicProxyProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.recordN8NDynamicProxy(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetProxyUrl(), req.GetData(), profile)
}

func (s *Server) recordN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data *pb.N8NDynamicProxyPreflightData, profile n8nDynamicProxyProfile) (any, error) {
	scope := n8nActionScopeFrom(jobID, accountID, n8nExecutionID)
	profile = profile.normalized()
	if profile.Purpose == "" {
		return nil, fmt.Errorf("dynamic proxy purpose is required")
	}
	if err := s.bindN8NExecution(ctx, scope.JobID, scope.N8NExecutionID); err != nil {
		return nil, err
	}
	data = n8nDynamicProxyPreflightData(scope, profile, data, true, "")
	proxyURL = strings.TrimSpace(proxyURL)
	parsed, err := url.Parse(proxyURL)
	if proxyURL == "" || err != nil || parsed.Scheme == "" || parsed.Host == "" {
		err = fmt.Errorf("proxy_runtime returned invalid proxy url")
		return nil, s.markN8NDynamicProxyPreflightFailedMessage(ctx, scope.JobID, err, n8nDynamicProxyPreflightData(scope, profile, data, false, err.Error()))
	}
	if err := s.setJobParams(ctx, scope.JobID, map[string]string{protocolProxyURLParam: proxyURL}); err != nil {
		return nil, err
	}
	if s.accountProxyUsages == nil {
		return nil, fmt.Errorf("account proxy usage store is not configured")
	}
	if err := s.accountProxyUsages.Record(ctx, accountproxyusage.RecordInput{JobID: scope.JobID, AccountID: scope.AccountID, N8NExecutionID: scope.N8NExecutionID, Purpose: profile.Purpose, ProxyURL: proxyURL, Data: data}); err != nil {
		return nil, err
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: scope.JobID, StepName: contracts.StepDynamicIPPreflight, Recoverable: false, Retryable: true, Detail: jobDataMessage(data)}); err != nil {
		return nil, err
	}
	if err := s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{
		JobId:       scope.JobID,
		StepName:    contracts.StepDynamicIPPreflight,
		Recoverable: false,
		Retryable:   true,
		Result:      jobDataMessage(data),
	}); err != nil {
		return nil, err
	}
	return n8nDynamicProxyPreflightResult(scope, true, proxyURL, data), nil
}

func (s *Server) FailN8NDynamicProxy(ctx context.Context, actionID string, req *pb.N8NDynamicProxyFailRequest) (any, error) {
	profile, err := n8nDynamicProxyProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.failN8NDynamicProxy(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetErrorMessage(), req.GetData(), profile)
}

func (s *Server) UseN8NDynamicProxy(ctx context.Context, actionID string, req *pb.N8NAuthStepRequest) (any, error) {
	profile, err := n8nDynamicProxyProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.useN8NProtocolProxy(ctx, profile, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId())
}

func (s *Server) failN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data *pb.N8NDynamicProxyPreflightData, profile n8nDynamicProxyProfile) (any, error) {
	scope := n8nActionScopeFrom(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, scope.JobID, scope.N8NExecutionID); err != nil {
		return nil, err
	}
	data = n8nDynamicProxyPreflightData(scope, profile, data, false, errorMessage)
	if err := s.storeN8NDynamicProxyPreflightFailureMessage(ctx, scope.JobID, data.GetErrorMessage(), data); err != nil {
		return nil, err
	}
	return n8nDynamicProxyPreflightResult(scope, false, "", data), nil
}

func n8nDynamicProxyPreflightData(scope n8nActionScope, profile n8nDynamicProxyProfile, data *pb.N8NDynamicProxyPreflightData, success bool, errorMessage string) *pb.N8NDynamicProxyPreflightData {
	if data == nil {
		data = &pb.N8NDynamicProxyPreflightData{}
	}
	data.AccountId = strings.TrimSpace(scope.AccountID)
	data.N8NExecutionId = strings.TrimSpace(scope.N8NExecutionID)
	if purpose := strings.TrimSpace(profile.Purpose); purpose != "" {
		data.Purpose = purpose
	}
	data.Success = success
	data.Accepted = success
	if success {
		data.ErrorMessage = ""
	} else {
		data.ErrorMessage = firstNonEmpty(errorMessage, data.GetErrorMessage(), "dynamic proxy preflight failed")
	}
	if !success && strings.TrimSpace(data.GetReason()) == "" {
		data.Reason = "dynamic_proxy_preflight_failed"
	}
	return data
}
