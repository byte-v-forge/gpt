package dashboard

import (
	"context"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NProbeActions interface {
	N8NDynamicProxyActions
	StartN8NProbeAccount(ctx context.Context, accountID string) (*pb.ProbeAccountResponse, error)
	CheckN8NProbeAuthEdge(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error)
	CheckN8NProbeToken(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error)
	ProbeN8NPlusTrial(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error)
	ProbeN8NTier(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error)
	CompleteN8NProbeAccount(ctx context.Context, req *pb.N8NProbeCompleteRequest) (any, error)
	FailN8NProbeAccount(ctx context.Context, req *pb.N8NProbeFailRequest) (any, error)
}

func n8nProbeWorkflowStartConfig(surface contracts.ActionProfile, api N8NProbeActions) n8nWorkflowStartConfig[*pb.ProbeAccountRequest, *pb.ProbeAccountResponse] {
	return n8nWorkflowStartConfigFor(
		surface.WorkflowStartProfile(),
		api,
		n8nProtoJSONWorkflowStartRequest(newProbeAccountRequest),
		startN8NProbeWorkflow,
		nil,
		failN8NProbeWorkflow,
	)
}

func newProbeAccountRequest() *pb.ProbeAccountRequest { return &pb.ProbeAccountRequest{} }

func startN8NProbeWorkflow(api N8NProbeActions, ctx context.Context, _ string, req *pb.ProbeAccountRequest) (*pb.ProbeAccountResponse, string, error) {
	resp, err := api.StartN8NProbeAccount(ctx, req.GetAccountId())
	return resp, req.GetAccountId(), err
}

func failN8NProbeWorkflow(api N8NProbeActions, ctx context.Context, _ string, resp *pb.ProbeAccountResponse, accountID string, err error) {
	_, _ = api.FailN8NProbeAccount(ctx, &pb.N8NProbeFailRequest{
		JobId:        resp.GetJobId(),
		AccountId:    accountID,
		ErrorMessage: err.Error(),
		Data:         &pb.N8NProbeTokenData{Reason: n8nTriggerFailedReason},
	})
}

func n8nProbeActionBindings(s *server) []actionHandlerBinding {
	profile := contracts.ResolveActionProfile(contracts.ActionProbeAccount)
	var actions N8NProbeActions = s.n8nActions
	return []actionHandlerBinding{
		n8nActionWorkflowBinding(
			s,
			profile,
			n8nProbeActionEndpoint(profile, actions),
			n8nProbeWorkflowStartConfig(profile, actions),
		),
	}
}

func n8nProbeActionEndpoint(profile contracts.ActionProfile, actions N8NProbeActions) n8nActionEndpoint {
	return n8nActionEndpoint{
		ActionID: profile.ActionID,
		Label:    profile.Label,
		API:      actions,
		Routes:   func() map[string]n8nActionRoute { return n8nProbeActionRoutes(profile, actions) },
	}
}

func n8nProbeActionRoutes(profile contracts.ActionProfile, actions N8NProbeActions) map[string]n8nActionRoute {
	return mergeN8NActionRoutes(
		n8nScopedDynamicProxyActionRoutes(profile.ActionID, actions, false),
		map[string]n8nActionRoute{
			"auth-edge-check": n8nProxyCheckActionRoute(actions.CheckN8NProbeAuthEdge),
			"check-token":     n8nProxyCheckActionRoute(actions.CheckN8NProbeToken),
			"plus-trial":      n8nProxyCheckActionRoute(actions.ProbeN8NPlusTrial),
			"tier":            n8nProxyCheckActionRoute(actions.ProbeN8NTier),
			"complete":        n8nProtoRequestActionRoute(newN8NProbeCompleteRequest, actions.CompleteN8NProbeAccount),
			"fail":            n8nProtoRequestActionRoute(newN8NProbeFailRequest, actions.FailN8NProbeAccount),
		},
	)
}

func newN8NProbeCompleteRequest() *pb.N8NProbeCompleteRequest {
	return &pb.N8NProbeCompleteRequest{}
}

func newN8NProbeFailRequest() *pb.N8NProbeFailRequest {
	return &pb.N8NProbeFailRequest{}
}
