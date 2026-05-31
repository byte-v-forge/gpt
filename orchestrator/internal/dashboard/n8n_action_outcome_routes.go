package dashboard

import (
	"context"

	"orchestrator/pb"
)

type N8NCodexOAuthOutcomeActions interface {
	CompleteN8NCodexOAuthAction(ctx context.Context, actionID string, req *pb.N8NCodexOAuthCompleteRequest) (any, error)
	FailN8NCodexOAuthAction(ctx context.Context, actionID string, req *pb.N8NCodexOAuthFailRequest) (any, error)
}

type N8NCodexOAuthBatchOutcomeActions interface {
	CompleteN8NCodexOAuthBatchAction(ctx context.Context, actionID string, req *pb.N8NCodexOAuthBatchCompleteRequest) (any, error)
}

func n8nCodexOAuthAccountOutcomeActionRoutes[T N8NCodexOAuthOutcomeActions](actionID string, actions T) map[string]n8nActionRoute {
	return map[string]n8nActionRoute{
		"complete": n8nProtoRequestActionRoute(newN8NCodexOAuthCompleteRequest, func(ctx context.Context, req *pb.N8NCodexOAuthCompleteRequest) (any, error) {
			return actions.CompleteN8NCodexOAuthAction(ctx, actionID, req)
		}),
		"fail": n8nProtoRequestActionRoute(newN8NCodexOAuthFailRequest, func(ctx context.Context, req *pb.N8NCodexOAuthFailRequest) (any, error) {
			return actions.FailN8NCodexOAuthAction(ctx, actionID, req)
		}),
	}
}

func n8nCodexOAuthBatchOutcomeActionRoutes[T N8NCodexOAuthBatchOutcomeActions](actionID string, actions T) map[string]n8nActionRoute {
	return map[string]n8nActionRoute{
		"complete": n8nProtoRequestActionRoute(newN8NCodexOAuthBatchCompleteRequest, func(ctx context.Context, req *pb.N8NCodexOAuthBatchCompleteRequest) (any, error) {
			return actions.CompleteN8NCodexOAuthBatchAction(ctx, actionID, req)
		}),
	}
}

func newN8NCodexOAuthCompleteRequest() *pb.N8NCodexOAuthCompleteRequest {
	return &pb.N8NCodexOAuthCompleteRequest{}
}

func newN8NCodexOAuthFailRequest() *pb.N8NCodexOAuthFailRequest {
	return &pb.N8NCodexOAuthFailRequest{}
}

func newN8NCodexOAuthBatchCompleteRequest() *pb.N8NCodexOAuthBatchCompleteRequest {
	return &pb.N8NCodexOAuthBatchCompleteRequest{}
}
