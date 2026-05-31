package api

import (
	"context"
	"strings"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) markN8NDynamicProxyPreflightFailedMessage(ctx context.Context, jobID string, err error, data proto.Message) error {
	return s.markN8NActionFailure(ctx, jobID, n8nDynamicProxyPreflightFailureRecordMessage(data), err)
}

func (s *Server) storeN8NDynamicProxyPreflightFailureMessage(ctx context.Context, jobID string, errorMessage string, data proto.Message) error {
	record := n8nDynamicProxyPreflightFailureRecordMessage(data)
	record.ErrorMessage = firstNonEmpty(errorMessage, "dynamic proxy preflight failed")
	return s.storeN8NActionFailure(ctx, jobID, record)
}

func n8nDynamicProxyPreflightFailureRecordMessage(data proto.Message) n8nActionFailureRecord {
	return n8nActionFailureRecord{
		Step:          contracts.StepDynamicIPPreflight,
		Status:        jobstatus.FailedRetryable,
		Retryable:     true,
		ResultMessage: data,
	}
}

func n8nDynamicProxyPreflightResult(scope n8nActionScope, success bool, proxyURL string, data *pb.N8NDynamicProxyPreflightData) *pb.N8NDynamicProxyResult {
	return n8nDynamicProxyStepResult(scope, contracts.StepDynamicIPPreflight, success, proxyURL, data, nil)
}

func n8nDynamicProxyAuthEdgeResult(scope n8nActionScope, proxyURL string, success bool, data *pb.N8NAuthEdgeData) *pb.N8NDynamicProxyResult {
	return n8nDynamicProxyStepResult(scope, contracts.StepProtocolAuthEdgeCheck, success, proxyURL, nil, data)
}

func n8nDynamicProxyStepResult(scope n8nActionScope, step string, success bool, proxyURL string, preflightData *pb.N8NDynamicProxyPreflightData, authEdgeData *pb.N8NAuthEdgeData) *pb.N8NDynamicProxyResult {
	scope = n8nActionScopeFrom(scope.JobID, scope.AccountID, scope.N8NExecutionID)
	return &pb.N8NDynamicProxyResult{
		JobId:          scope.JobID,
		AccountId:      scope.AccountID,
		N8NExecutionId: scope.N8NExecutionID,
		Step:           strings.TrimSpace(step),
		Success:        success,
		ProxyUrl:       strings.TrimSpace(proxyURL),
		PreflightData:  preflightData,
		AuthEdgeData:   authEdgeData,
	}
}
