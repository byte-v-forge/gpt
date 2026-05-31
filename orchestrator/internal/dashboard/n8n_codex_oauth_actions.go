package dashboard

import (
	"context"
	"net/http"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type N8NCodexOAuthActions interface {
	N8NCodexOAuthFlowActions
	N8NCodexOAuthOutcomeActions
	StartN8NCodexOAuthAccount(ctx context.Context, actionID string, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error)
	BindN8NCodexOAuthExecution(ctx context.Context, req *pb.N8NCodexOAuthBindRequest) (any, error)
}

type N8NCodexOAuthProtocolActions interface {
	N8NDynamicProxyActions
	N8NCodexOAuthFlowActions
	N8NCodexOAuthOutcomeActions
	StartN8NCodexOAuthAccount(ctx context.Context, actionID string, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error)
}

type N8NCodexOAuthAddPhoneActions interface {
	N8NDynamicProxyActions
	N8NCodexOAuthFlowActions
	N8NCodexOAuthOutcomeActions
	StartN8NCodexOAuthAddPhoneAccount(ctx context.Context, actionID string, req *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, string, error)
	CodexOAuthAcquirePhone(ctx context.Context, req *pb.CodexOAuthAcquirePhoneInput) (*pb.CodexOAuthPhoneLease, error)
	CodexOAuthReleasePhone(ctx context.Context, req *pb.CodexOAuthReleasePhoneInput) (any, error)
}

type N8NCodexOAuthBatchActions interface {
	N8NCodexOAuthOutcomeActions
	N8NCodexOAuthBatchOutcomeActions
	StartN8NCodexOAuthBatch(ctx context.Context, actionID string, req *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, error)
	CreateN8NCodexOAuthBatchAddPhoneChild(ctx context.Context, req *pb.N8NCodexOAuthBatchChildRequest) (any, error)
}

type n8nCodexOAuthBindActions interface {
	BindN8NCodexOAuthExecution(context.Context, *pb.N8NCodexOAuthBindRequest) (any, error)
}

type n8nCodexOAuthAccountRouteActions interface {
	N8NCodexOAuthFlowActions
	N8NCodexOAuthOutcomeActions
}

type n8nCodexOAuthAccountStartActions interface {
	N8NCodexOAuthOutcomeActions
	StartN8NCodexOAuthAccount(context.Context, string, *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error)
}

type n8nCodexOAuthAddPhoneStartActions interface {
	StartN8NCodexOAuthAddPhoneAccount(context.Context, string, *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, string, error)
}

type n8nCodexOAuthPhoneActions interface {
	CodexOAuthAcquirePhone(context.Context, *pb.CodexOAuthAcquirePhoneInput) (*pb.CodexOAuthPhoneLease, error)
	CodexOAuthReleasePhone(context.Context, *pb.CodexOAuthReleasePhoneInput) (any, error)
}

type n8nCodexOAuthBatchChildActions interface {
	CreateN8NCodexOAuthBatchAddPhoneChild(context.Context, *pb.N8NCodexOAuthBatchChildRequest) (any, error)
}

func n8nCodexOAuthAddPhoneWorkflowStartConfig(profile n8nCodexOAuthRouteProfile, api N8NCodexOAuthAddPhoneActions) n8nWorkflowStartConfig[*pb.CodexOAuthAddPhoneRequest, *pb.CodexOAuthAddPhoneResponse] {
	return n8nCodexOAuthWorkflowStartConfigFor(
		profile.WorkflowStartProfile(),
		api,
		n8nProtoJSONWorkflowStartRequest(newCodexOAuthAddPhoneRequest),
		startN8NCodexOAuthAddPhoneWorkflow[N8NCodexOAuthAddPhoneActions],
		n8nCodexOAuthAddPhoneWorkflowPayload,
	)
}

func startN8NCodexOAuthAddPhoneWorkflow[API n8nCodexOAuthAddPhoneStartActions](api API, ctx context.Context, actionID string, req *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, string, error) {
	return api.StartN8NCodexOAuthAddPhoneAccount(ctx, actionID, req)
}

func n8nCodexOAuthBatchAddPhoneWorkflowStartConfig(profile n8nCodexOAuthRouteProfile, api N8NCodexOAuthBatchActions) n8nWorkflowStartConfig[*pb.CodexOAuthBatchAddPhoneRequest, *pb.CodexOAuthBatchAddPhoneResponse] {
	return n8nCodexOAuthWorkflowStartConfigFor(
		profile.WorkflowStartProfile(),
		api,
		n8nProtoJSONWorkflowStartRequest(newCodexOAuthBatchAddPhoneRequest),
		startN8NCodexOAuthBatchAddPhoneWorkflow,
		n8nCodexOAuthBatchAddPhoneWorkflowPayload,
	)
}

func startN8NCodexOAuthBatchAddPhoneWorkflow(api N8NCodexOAuthBatchActions, ctx context.Context, actionID string, req *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, string, error) {
	resp, err := api.StartN8NCodexOAuthBatch(ctx, actionID, req)
	return resp, "", err
}

func n8nCodexOAuthAccountWorkflowStartConfig[API n8nCodexOAuthAccountStartActions](profile contracts.ActionWorkflowStartProfile, api API) n8nWorkflowStartConfig[*pb.CodexOAuthRequest, *pb.CodexOAuthResponse] {
	return n8nCodexOAuthWorkflowStartConfigFor(
		profile,
		api,
		n8nProtoJSONWorkflowStartRequest(newCodexOAuthRequest),
		startN8NCodexOAuthAccountWorkflow[API],
		n8nCodexOAuthWorkflowPayload,
	)
}

func startN8NCodexOAuthAccountWorkflow[API n8nCodexOAuthAccountStartActions](api API, ctx context.Context, actionID string, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error) {
	return api.StartN8NCodexOAuthAccount(ctx, actionID, req)
}

func n8nCodexOAuthWorkflowStartConfigFor[API N8NCodexOAuthOutcomeActions, Req any, Resp n8nStartedJobResponse](profile contracts.ActionWorkflowStartProfile, api API, decode func(*http.Request) (Req, error), start n8nWorkflowAccountStartCall[API, Req, Resp], payload func(Req, Resp, string) proto.Message) n8nWorkflowStartConfig[Req, Resp] {
	return n8nWorkflowStartConfigFor(profile, api, decode, start, payload, failN8NCodexOAuthWorkflow[API, Resp])
}

func failN8NCodexOAuthWorkflow[API N8NCodexOAuthOutcomeActions, Resp n8nStartedJobResponse](api API, ctx context.Context, actionID string, resp Resp, accountID string, err error) {
	_, _ = api.FailN8NCodexOAuthAction(ctx, actionID, &pb.N8NCodexOAuthFailRequest{
		JobId:        resp.GetJobId(),
		AccountId:    accountID,
		ErrorMessage: err.Error(),
		Data:         &pb.ActivityCodexOAuthStepData{Reason: n8nTriggerFailedReason},
	})
}

type n8nCodexOAuthRouteProfile struct {
	contracts.ActionProfile
	FlowNames    n8nCodexOAuthFlowRouteNames
	IncludeProxy bool
}

type n8nCodexOAuthRouteProfileDefinition struct {
	ActionID     string
	FlowNames    n8nCodexOAuthFlowRouteNames
	IncludeProxy bool
}

type n8nCodexOAuthRouteBuilder[T any] func(n8nCodexOAuthRouteProfile, T) map[string]n8nActionRoute

var n8nCodexOAuthRouteProfileDefinitions = []n8nCodexOAuthRouteProfileDefinition{
	{
		ActionID: contracts.ActionCodexOAuth,
		FlowNames: n8nCodexOAuthFlowRouteNames{
			Start:    "start-browser",
			Detect:   "detect-browser-stage",
			AddPhone: "add-phone-browser",
			Complete: "complete-browser",
			Stop:     "stop-browser",
		},
	},
	{
		ActionID:     contracts.ActionCodexOAuthProtocol,
		IncludeProxy: true,
		FlowNames: n8nCodexOAuthFlowRouteNames{
			Start:    "start",
			Complete: "complete-protocol",
			Stop:     "stop-protocol",
		},
	},
	{
		ActionID:     contracts.ActionCodexOAuthAddPhone,
		IncludeProxy: true,
		FlowNames: n8nCodexOAuthFlowRouteNames{
			Start:    "start-protocol",
			AddPhone: "add-phone-protocol",
			Complete: "complete-protocol",
			Stop:     "stop-protocol",
		},
	},
	{ActionID: contracts.ActionCodexOAuthBatchAddPhone},
}

func n8nCodexOAuthActionBindings(s *server) []actionHandlerBinding {
	browser := n8nCodexOAuthRouteProfileFor(contracts.ActionCodexOAuth)
	protocol := n8nCodexOAuthRouteProfileFor(contracts.ActionCodexOAuthProtocol)
	addPhone := n8nCodexOAuthRouteProfileFor(contracts.ActionCodexOAuthAddPhone)
	batchAddPhone := n8nCodexOAuthRouteProfileFor(contracts.ActionCodexOAuthBatchAddPhone)
	var browserActions N8NCodexOAuthActions = s.n8nActions
	var protocolActions N8NCodexOAuthProtocolActions = s.n8nActions
	var addPhoneActions N8NCodexOAuthAddPhoneActions = s.n8nActions
	var batchActions N8NCodexOAuthBatchActions = s.n8nActions
	return []actionHandlerBinding{
		n8nCodexOAuthAccountActionBinding(s, browser, browserActions, n8nCodexOAuthActionRoutes),
		n8nCodexOAuthAccountActionBinding(s, protocol, protocolActions, n8nCodexOAuthProtocolActionRoutes),
		n8nCodexOAuthActionBinding(
			s,
			addPhone,
			addPhoneActions,
			n8nCodexOAuthAddPhoneActionRoutes,
			n8nCodexOAuthAddPhoneWorkflowStartConfig(addPhone, addPhoneActions),
		),
		n8nCodexOAuthActionBinding(
			s,
			batchAddPhone,
			batchActions,
			n8nCodexOAuthBatchAddPhoneActionRoutes,
			n8nCodexOAuthBatchAddPhoneWorkflowStartConfig(batchAddPhone, batchActions),
		),
	}
}

func n8nCodexOAuthAccountActionBinding[API n8nCodexOAuthAccountStartActions](s *server, profile n8nCodexOAuthRouteProfile, api API, routes n8nCodexOAuthRouteBuilder[API]) actionHandlerBinding {
	return n8nCodexOAuthActionBinding(
		s,
		profile,
		api,
		routes,
		n8nCodexOAuthAccountWorkflowStartConfig(profile.WorkflowStartProfile(), api),
	)
}

func n8nCodexOAuthActionBinding[API any, Req any, Resp n8nStartedJobResponse](s *server, profile n8nCodexOAuthRouteProfile, api API, routes n8nCodexOAuthRouteBuilder[API], start n8nWorkflowStartConfig[Req, Resp]) actionHandlerBinding {
	return n8nActionWorkflowBinding(
		s,
		profile.ActionProfile,
		n8nCodexOAuthEndpoint(profile, api, routes),
		start,
	)
}

func n8nCodexOAuthEndpoint[T any](profile n8nCodexOAuthRouteProfile, api T, routes n8nCodexOAuthRouteBuilder[T]) n8nActionEndpoint {
	return n8nActionEndpoint{
		ActionID: profile.ActionID,
		Label:    profile.Label,
		API:      api,
		Routes:   func() map[string]n8nActionRoute { return routes(profile, api) },
	}
}

func n8nCodexOAuthActionRoutes(profile n8nCodexOAuthRouteProfile, actions N8NCodexOAuthActions) map[string]n8nActionRoute {
	return mergeN8NActionRoutes(
		n8nCodexOAuthBindActionRoutes(actions),
		n8nCodexOAuthAccountRouteGroups(profile, actions, nil),
	)
}

func n8nCodexOAuthProtocolActionRoutes(profile n8nCodexOAuthRouteProfile, actions N8NCodexOAuthProtocolActions) map[string]n8nActionRoute {
	return n8nCodexOAuthAccountRouteGroups(profile, actions, actions)
}

func n8nCodexOAuthAddPhoneActionRoutes(profile n8nCodexOAuthRouteProfile, actions N8NCodexOAuthAddPhoneActions) map[string]n8nActionRoute {
	return mergeN8NActionRoutes(
		n8nCodexOAuthAccountRouteGroups(profile, actions, actions),
		n8nCodexOAuthPhoneActionRoutes(actions),
	)
}

func n8nCodexOAuthBatchAddPhoneActionRoutes(profile n8nCodexOAuthRouteProfile, actions N8NCodexOAuthBatchActions) map[string]n8nActionRoute {
	return mergeN8NActionRoutes(
		n8nCodexOAuthBatchChildActionRoutes(actions),
		n8nCodexOAuthBatchOutcomeActionRoutes(profile.ActionID, actions),
	)
}

func n8nCodexOAuthAccountRouteGroups(profile n8nCodexOAuthRouteProfile, actions n8nCodexOAuthAccountRouteActions, proxyActions N8NDynamicProxyActions) map[string]n8nActionRoute {
	var proxyRoutes map[string]n8nActionRoute
	if profile.IncludeProxy && proxyActions != nil {
		proxyRoutes = n8nScopedDynamicProxyActionRoutes(profile.ActionID, proxyActions, true)
	}
	return mergeN8NActionRoutes(
		proxyRoutes,
		n8nCodexOAuthFlowActionRoutes(profile.ActionID, actions, profile.FlowNames),
		n8nCodexOAuthAccountOutcomeActionRoutes(profile.ActionID, actions),
	)
}

func n8nCodexOAuthBindActionRoutes[T n8nCodexOAuthBindActions](actions T) map[string]n8nActionRoute {
	return map[string]n8nActionRoute{
		"bind": n8nProtoRequestActionRoute(newN8NCodexOAuthBindRequest, actions.BindN8NCodexOAuthExecution),
	}
}

func n8nCodexOAuthPhoneActionRoutes[T n8nCodexOAuthPhoneActions](actions T) map[string]n8nActionRoute {
	return map[string]n8nActionRoute{
		"acquire-phone": n8nProtoJSONActionRoute(
			newCodexOAuthAcquirePhoneInput,
			actions.CodexOAuthAcquirePhone,
		),
		"release-phone": n8nProtoRequestActionRoute(
			newCodexOAuthReleasePhoneInput,
			actions.CodexOAuthReleasePhone,
		),
	}
}

func n8nCodexOAuthBatchChildActionRoutes[T n8nCodexOAuthBatchChildActions](actions T) map[string]n8nActionRoute {
	return map[string]n8nActionRoute{
		"create-child": n8nProtoRequestActionRoute(newN8NCodexOAuthBatchChildRequest, actions.CreateN8NCodexOAuthBatchAddPhoneChild),
	}
}

func newN8NCodexOAuthBindRequest() *pb.N8NCodexOAuthBindRequest {
	return &pb.N8NCodexOAuthBindRequest{}
}

func newN8NCodexOAuthBatchChildRequest() *pb.N8NCodexOAuthBatchChildRequest {
	return &pb.N8NCodexOAuthBatchChildRequest{}
}

func n8nCodexOAuthRouteProfileFor(actionID string) n8nCodexOAuthRouteProfile {
	surface := contracts.ResolveActionProfile(actionID)
	for _, definition := range n8nCodexOAuthRouteProfileDefinitions {
		if contracts.ResolveActionProfile(definition.ActionID).ActionID != surface.ActionID {
			continue
		}
		return n8nCodexOAuthRouteProfile{
			ActionProfile: surface,
			FlowNames:     definition.FlowNames,
			IncludeProxy:  definition.IncludeProxy,
		}
	}
	return n8nCodexOAuthRouteProfile{ActionProfile: surface}
}
