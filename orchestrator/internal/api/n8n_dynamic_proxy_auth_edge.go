package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

const (
	n8nAuthEdgeCheckTargetCSRF        = "chatgpt_csrf"
	n8nAuthEdgeCheckTargetAuthSession = "chatgpt_auth_session"
	n8nAuthEdgeCheckTargetAccessToken = "chatgpt_access_token"
)

func (s *Server) checkN8NProtocolAuthEdge(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, profile n8nDynamicProxyProfile) (any, error) {
	scope := n8nActionScopeFrom(jobID, accountID, n8nExecutionID)
	profile = profile.normalized()
	if profile.ProtocolMode == "" {
		return nil, fmt.Errorf("protocol mode is required")
	}
	if profile.AuthEdgeCheckTarget == "" {
		return nil, fmt.Errorf("auth edge check target is required")
	}
	if err := s.bindN8NExecution(ctx, scope.JobID, scope.N8NExecutionID); err != nil {
		return nil, err
	}
	proxyURL = strings.TrimSpace(proxyURL)
	base := n8nAuthEdgeBaseData(scope, profile.AuthEdgeCheckTarget)
	if proxyURL == "" {
		return n8nDynamicProxyAuthEdgeResult(scope, "", false, n8nAuthEdgeOutcome(base, false, "", "protocol proxy url is required")), nil
	}
	input := s.protocolAuthStartInput(ctx, scope.JobID, scope.AccountID, profile.ProtocolMode)
	input.ProxyUrl = proxyURL
	out, err := s.activities.ProtocolAuthEdgeCheckActivity(ctx, input)
	if err != nil {
		return n8nDynamicProxyAuthEdgeResult(scope, proxyURL, false, n8nAuthEdgeActivityData(scope, profile.AuthEdgeCheckTarget, out.GetData(), false, err.Error())), nil
	}
	return n8nDynamicProxyAuthEdgeResult(scope, proxyURL, true, n8nAuthEdgeActivityData(scope, profile.AuthEdgeCheckTarget, out.GetData(), true, "")), nil
}

func n8nAuthEdgeBaseData(scope n8nActionScope, target string) *pb.N8NAuthEdgeData {
	return &pb.N8NAuthEdgeData{
		AccountId:           strings.TrimSpace(scope.AccountID),
		N8NExecutionId:      strings.TrimSpace(scope.N8NExecutionID),
		AuthEdgeCheckTarget: strings.TrimSpace(target),
	}
}

func n8nAuthEdgeActivityData(scope n8nActionScope, target string, activityData *pb.ActivityProtocolAuthOutputData, accepted bool, errorMessage string) *pb.N8NAuthEdgeData {
	data := n8nAuthEdgeBaseData(scope, target)
	if source := activityData.GetAuthEdge(); source != nil {
		data.Driver = strings.TrimSpace(source.GetDriver())
		data.Mode = strings.TrimSpace(source.GetMode())
		data.AuthEdgeCheckTarget = firstNonEmpty(source.GetAuthEdgeCheckTarget(), target)
		data.ChatgptCsrfStatus = source.GetChatgptCsrfStatus()
		data.ChatgptCsrfAttempt = source.GetChatgptCsrfAttempt()
		data.ChatgptCsrfEdgeChallenge = source.ChatgptCsrfEdgeChallenge
		data.ChatgptCsrfReady = source.ChatgptCsrfReady
		if source.AuthEdgeAccepted != nil {
			accepted = source.GetAuthEdgeAccepted()
		}
		if strings.TrimSpace(errorMessage) == "" {
			errorMessage = source.GetErrorMessage()
		}
	}
	data.AccountId = strings.TrimSpace(scope.AccountID)
	data.N8NExecutionId = strings.TrimSpace(scope.N8NExecutionID)
	return n8nAuthEdgeOutcome(data, accepted, firstNonEmpty(data.GetAuthEdgeCheckTarget(), target), errorMessage)
}

func n8nAuthEdgeOutcome(data *pb.N8NAuthEdgeData, accepted bool, target string, errorMessage string) *pb.N8NAuthEdgeData {
	if data == nil {
		data = &pb.N8NAuthEdgeData{}
	}
	if target = strings.TrimSpace(target); target != "" {
		data.AuthEdgeCheckTarget = target
	}
	data.AuthEdgeAccepted = protoBool(accepted)
	data.ErrorMessage = strings.TrimSpace(errorMessage)
	return data
}

func protoBool(value bool) *bool {
	return &value
}
