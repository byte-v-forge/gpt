package dashboard

import (
	"context"
	"net/http"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NRegisterActions interface {
	N8NBrowserAuthActions
	StartN8NRegisterAccount(ctx context.Context, actionID string, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error)
}

type N8NRegisterProtocolActions interface {
	N8NProtocolAuthActions
	StartN8NRegisterAccount(ctx context.Context, actionID string, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error)
}

type N8NLoginSessionActions interface {
	N8NBrowserAuthActions
	StartN8NLoginSessionAccount(ctx context.Context, actionID string, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error)
}

type N8NLoginSessionProtocolActions interface {
	N8NProtocolAuthActions
	StartN8NLoginSessionAccount(ctx context.Context, actionID string, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error)
}

type n8nAuthRouteProfile struct{ contracts.ActionProfile }

type n8nRegisterWorkflowActions interface {
	StartN8NRegisterAccount(context.Context, string, *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error)
}

type n8nLoginWorkflowActions interface {
	StartN8NLoginSessionAccount(context.Context, string, *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error)
}

func newRegisterAccountRequest() *pb.RegisterAccountRequest { return &pb.RegisterAccountRequest{} }
func newLoginAccountRequest() *pb.LoginAccountRequest       { return &pb.LoginAccountRequest{} }

func n8nRegisterWorkflowStartConfig[API n8nRegisterWorkflowActions](profile n8nAuthRouteProfile, api API, fail n8nWorkflowStartFailureCall[API, *pb.RegisterAccountResponse]) n8nWorkflowStartConfig[*pb.RegisterAccountRequest, *pb.RegisterAccountResponse] {
	return n8nAuthWorkflowStartConfig(
		profile.WorkflowStartProfile(),
		api,
		n8nProtoJSONWorkflowStartRequest(newRegisterAccountRequest),
		startN8NRegisterWorkflow[API],
		fail,
	)
}

func startN8NRegisterWorkflow[API n8nRegisterWorkflowActions](api API, ctx context.Context, actionID string, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error) {
	return api.StartN8NRegisterAccount(ctx, actionID, req)
}

func n8nLoginWorkflowStartConfig[API n8nLoginWorkflowActions](profile n8nAuthRouteProfile, api API, fail n8nWorkflowStartFailureCall[API, *pb.LoginAccountResponse]) n8nWorkflowStartConfig[*pb.LoginAccountRequest, *pb.LoginAccountResponse] {
	return n8nAuthWorkflowStartConfig(
		profile.WorkflowStartProfile(),
		api,
		n8nProtoJSONWorkflowStartRequest(newLoginAccountRequest),
		startN8NLoginSessionWorkflow[API],
		fail,
	)
}

func startN8NLoginSessionWorkflow[API n8nLoginWorkflowActions](api API, ctx context.Context, actionID string, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error) {
	return api.StartN8NLoginSessionAccount(ctx, actionID, req)
}

func n8nRegisterActionBindings(s *server) []actionHandlerBinding {
	browser := n8nAuthRouteProfileFor(contracts.ActionRegister)
	protocol := n8nAuthRouteProfileFor(contracts.ActionRegisterProtocol)
	var browserActions N8NRegisterActions = s.n8nActions
	var protocolActions N8NRegisterProtocolActions = s.n8nActions
	return []actionHandlerBinding{
		n8nAuthActionWorkflowBinding(
			s,
			browser,
			browserActions,
			n8nBrowserAuthRoutes[N8NRegisterActions],
			n8nRegisterWorkflowStartConfig(browser, browserActions, n8nBrowserAuthWorkflowFailure[N8NRegisterActions, *pb.RegisterAccountResponse]),
		),
		n8nAuthActionWorkflowBinding(
			s,
			protocol,
			protocolActions,
			n8nProtocolAuthRoutes[N8NRegisterProtocolActions],
			n8nRegisterWorkflowStartConfig(protocol, protocolActions, n8nProtocolAuthWorkflowFailure[N8NRegisterProtocolActions, *pb.RegisterAccountResponse]),
		),
	}
}

func n8nLoginSessionActionBindings(s *server) []actionHandlerBinding {
	browser := n8nAuthRouteProfileFor(contracts.ActionLoginSession)
	protocol := n8nAuthRouteProfileFor(contracts.ActionLoginSessionProtocol)
	var browserActions N8NLoginSessionActions = s.n8nActions
	var protocolActions N8NLoginSessionProtocolActions = s.n8nActions
	return []actionHandlerBinding{
		n8nAuthActionWorkflowBinding(
			s,
			browser,
			browserActions,
			n8nBrowserAuthRoutes[N8NLoginSessionActions],
			n8nLoginWorkflowStartConfig(browser, browserActions, n8nBrowserAuthWorkflowFailure[N8NLoginSessionActions, *pb.LoginAccountResponse]),
		),
		n8nAuthActionWorkflowBinding(
			s,
			protocol,
			protocolActions,
			n8nProtocolAuthRoutes[N8NLoginSessionProtocolActions],
			n8nLoginWorkflowStartConfig(protocol, protocolActions, n8nProtocolAuthWorkflowFailure[N8NLoginSessionProtocolActions, *pb.LoginAccountResponse]),
		),
	}
}

func n8nAuthActionWorkflowBinding[API any, Req any, Resp n8nStartedJobResponse](s *server, profile n8nAuthRouteProfile, api API, routes func(string, API) map[string]n8nActionRoute, start n8nWorkflowStartConfig[Req, Resp]) actionHandlerBinding {
	return n8nActionWorkflowBinding(
		s,
		profile.ActionProfile,
		n8nAuthEndpoint(profile, api, routes),
		start,
	)
}

func n8nAuthWorkflowStartConfig[API any, Req any, Resp n8nStartedJobResponse](profile contracts.ActionWorkflowStartProfile, api API, decode func(*http.Request) (Req, error), start n8nWorkflowAccountStartCall[API, Req, Resp], fail n8nWorkflowStartFailureCall[API, Resp]) n8nWorkflowStartConfig[Req, Resp] {
	return n8nWorkflowStartConfigFor(profile, api, decode, start, nil, fail)
}

func n8nBrowserAuthWorkflowFailure[API N8NBrowserAuthActions, Resp n8nStartedJobResponse](api API, ctx context.Context, actionID string, resp Resp, accountID string, err error) {
	_, _ = api.FailN8NBrowserAuth(ctx, actionID, &pb.N8NAuthFailRequest{
		JobId:        resp.GetJobId(),
		AccountId:    accountID,
		ErrorMessage: err.Error(),
		Data:         &pb.N8NAuthFailureData{Reason: n8nTriggerFailedReason},
	})
}

func n8nProtocolAuthWorkflowFailure[API N8NProtocolAuthActions, Resp n8nStartedJobResponse](api API, ctx context.Context, actionID string, resp Resp, accountID string, err error) {
	_, _ = api.FailN8NProtocolAuth(ctx, actionID, &pb.N8NAuthFailRequest{
		JobId:        resp.GetJobId(),
		AccountId:    accountID,
		ErrorMessage: err.Error(),
		Data:         &pb.N8NAuthFailureData{Reason: n8nTriggerFailedReason},
	})
}

func n8nAuthEndpoint[T any](profile n8nAuthRouteProfile, api T, routes func(string, T) map[string]n8nActionRoute) n8nActionEndpoint {
	return n8nActionEndpoint{
		ActionID: profile.ActionID,
		Label:    profile.Label,
		API:      api,
		Routes: func() map[string]n8nActionRoute {
			if routes == nil {
				return nil
			}
			return routes(profile.ActionID, api)
		},
	}
}

func n8nAuthRouteProfileFor(actionID string) n8nAuthRouteProfile {
	return n8nAuthRouteProfile{
		ActionProfile: contracts.ResolveActionProfile(actionID),
	}
}
