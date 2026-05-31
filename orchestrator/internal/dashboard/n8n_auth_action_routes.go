package dashboard

import (
	"context"

	"orchestrator/pb"
)

type n8nAuthStepCall func(context.Context, *pb.N8NAuthStepRequest) (any, error)
type n8nAuthWaitCall func(context.Context, *pb.N8NAuthStepRequest) (any, error)
type n8nAuthOTPWaitCall func(context.Context, *pb.N8NAuthOTPWaitRequest) (any, error)
type n8nAuthCompleteCall func(context.Context, *pb.N8NAuthCompleteRequest) (any, error)
type n8nAuthFinishCall func(context.Context, *pb.N8NAuthFinishRequest) (any, error)
type n8nAuthFailureCall func(context.Context, *pb.N8NAuthFailRequest) (any, error)
type n8nProxyRecordCall func(context.Context, *pb.N8NDynamicProxyRecordRequest) (any, error)
type n8nProxyCheckCall func(context.Context, *pb.N8NDynamicProxyCheckRequest) (any, error)
type n8nProxyFailureCall func(context.Context, *pb.N8NDynamicProxyFailRequest) (any, error)

func (call n8nAuthStepCall) route(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nAuthWaitCall) route(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nAuthOTPWaitCall) route(ctx context.Context, req *pb.N8NAuthOTPWaitRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nAuthCompleteCall) route(ctx context.Context, req *pb.N8NAuthCompleteRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nAuthFinishCall) route(ctx context.Context, req *pb.N8NAuthFinishRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nAuthFailureCall) route(ctx context.Context, req *pb.N8NAuthFailRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nProxyRecordCall) route(ctx context.Context, req *pb.N8NDynamicProxyRecordRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nProxyCheckCall) route(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
	return call(ctx, req)
}

func (call n8nProxyFailureCall) route(ctx context.Context, req *pb.N8NDynamicProxyFailRequest) (any, error) {
	return call(ctx, req)
}

type N8NBrowserAuthActions interface {
	StartN8NBrowserAuth(context.Context, string, *pb.N8NAuthStepRequest) (any, error)
	AwaitN8NBrowserAuthOTP(context.Context, string, *pb.N8NAuthOTPWaitRequest) (any, error)
	CompleteN8NBrowserAuth(context.Context, string, *pb.N8NAuthCompleteRequest) (any, error)
	FinishN8NBrowserAuth(context.Context, string, *pb.N8NAuthFinishRequest) (any, error)
	FailN8NBrowserAuth(context.Context, string, *pb.N8NAuthFailRequest) (any, error)
}

type N8NProtocolAuthActions interface {
	N8NDynamicProxyActions
	CheckN8NProtocolAuthEdge(context.Context, string, *pb.N8NDynamicProxyCheckRequest) (any, error)
	StartN8NProtocolAuth(context.Context, string, *pb.N8NAuthStepRequest) (any, error)
	WaitN8NProtocolAuth(context.Context, string, *pb.N8NAuthStepRequest) (any, error)
	AwaitN8NProtocolAuthOTP(context.Context, string, *pb.N8NAuthOTPWaitRequest) (any, error)
	CompleteN8NProtocolAuth(context.Context, string, *pb.N8NAuthCompleteRequest) (any, error)
	FinishN8NProtocolAuth(context.Context, string, *pb.N8NAuthFinishRequest) (any, error)
	FailN8NProtocolAuth(context.Context, string, *pb.N8NAuthFailRequest) (any, error)
}

type N8NDynamicProxyActions interface {
	N8NDynamicProxySettings(context.Context, string, *pb.N8NAuthStepRequest) (any, error)
	RecordN8NDynamicProxy(context.Context, string, *pb.N8NDynamicProxyRecordRequest) (any, error)
	FailN8NDynamicProxy(context.Context, string, *pb.N8NDynamicProxyFailRequest) (any, error)
	UseN8NDynamicProxy(context.Context, string, *pb.N8NAuthStepRequest) (any, error)
}

type n8nDynamicProxyRoutes struct {
	ProxySettings n8nAuthStepCall
	RecordProxy   n8nProxyRecordCall
	FailProxy     n8nProxyFailureCall
	UseProxy      n8nAuthStepCall
}

type n8nAuthRoutes struct {
	Start    n8nAuthStepCall
	Wait     n8nAuthWaitCall
	AwaitOTP n8nAuthOTPWaitCall
	Complete n8nAuthCompleteCall
	Finish   n8nAuthFinishCall
	Fail     n8nAuthFailureCall
}

func n8nBrowserAuthRoutes[T N8NBrowserAuthActions](actionID string, actions T) map[string]n8nActionRoute {
	return n8nAuthActionRoutes(n8nAuthRoutes{
		Start: func(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
			return actions.StartN8NBrowserAuth(ctx, actionID, req)
		},
		AwaitOTP: func(ctx context.Context, req *pb.N8NAuthOTPWaitRequest) (any, error) {
			return actions.AwaitN8NBrowserAuthOTP(ctx, actionID, req)
		},
		Complete: func(ctx context.Context, req *pb.N8NAuthCompleteRequest) (any, error) {
			return actions.CompleteN8NBrowserAuth(ctx, actionID, req)
		},
		Finish: func(ctx context.Context, req *pb.N8NAuthFinishRequest) (any, error) {
			return actions.FinishN8NBrowserAuth(ctx, actionID, req)
		},
		Fail: func(ctx context.Context, req *pb.N8NAuthFailRequest) (any, error) {
			return actions.FailN8NBrowserAuth(ctx, actionID, req)
		},
	})
}

func n8nProtocolAuthRoutes[T N8NProtocolAuthActions](actionID string, actions T) map[string]n8nActionRoute {
	return mergeN8NActionRoutes(
		n8nScopedDynamicProxyActionRoutes(actionID, actions, true),
		n8nAuthActionRoutes(n8nAuthRoutes{
			Start: func(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
				return actions.StartN8NProtocolAuth(ctx, actionID, req)
			},
			Wait: func(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
				return actions.WaitN8NProtocolAuth(ctx, actionID, req)
			},
			AwaitOTP: func(ctx context.Context, req *pb.N8NAuthOTPWaitRequest) (any, error) {
				return actions.AwaitN8NProtocolAuthOTP(ctx, actionID, req)
			},
			Complete: func(ctx context.Context, req *pb.N8NAuthCompleteRequest) (any, error) {
				return actions.CompleteN8NProtocolAuth(ctx, actionID, req)
			},
			Finish: func(ctx context.Context, req *pb.N8NAuthFinishRequest) (any, error) {
				return actions.FinishN8NProtocolAuth(ctx, actionID, req)
			},
			Fail: func(ctx context.Context, req *pb.N8NAuthFailRequest) (any, error) {
				return actions.FailN8NProtocolAuth(ctx, actionID, req)
			},
		}),
		map[string]n8nActionRoute{
			"auth-edge-check": n8nProxyCheckActionRoute(func(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
				return actions.CheckN8NProtocolAuthEdge(ctx, actionID, req)
			}),
		},
	)
}

func n8nAuthActionRoutes(routes n8nAuthRoutes) map[string]n8nActionRoute {
	actionRoutes := make(map[string]n8nActionRoute)
	if routes.Start != nil {
		actionRoutes["start"] = n8nAuthStepActionRoute(routes.Start)
	}
	if routes.Wait != nil {
		actionRoutes["wait"] = n8nAuthWaitActionRoute(routes.Wait)
	}
	if routes.AwaitOTP != nil {
		actionRoutes["await-otp"] = n8nAuthOTPWaitActionRoute(routes.AwaitOTP)
	}
	if routes.Complete != nil {
		actionRoutes["complete"] = n8nAuthCompleteActionRoute(routes.Complete)
	}
	if routes.Finish != nil {
		actionRoutes["finish"] = n8nAuthFinishActionRoute(routes.Finish)
	}
	if routes.Fail != nil {
		actionRoutes["fail"] = n8nAuthFailureActionRoute(routes.Fail)
	}
	return actionRoutes
}

func n8nScopedDynamicProxyActionRoutes[T N8NDynamicProxyActions](actionID string, actions T, includeUseProxy bool) map[string]n8nActionRoute {
	routes := n8nDynamicProxyRoutes{
		ProxySettings: func(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
			return actions.N8NDynamicProxySettings(ctx, actionID, req)
		},
		RecordProxy: func(ctx context.Context, req *pb.N8NDynamicProxyRecordRequest) (any, error) {
			return actions.RecordN8NDynamicProxy(ctx, actionID, req)
		},
		FailProxy: func(ctx context.Context, req *pb.N8NDynamicProxyFailRequest) (any, error) {
			return actions.FailN8NDynamicProxy(ctx, actionID, req)
		},
	}
	if includeUseProxy {
		routes.UseProxy = func(ctx context.Context, req *pb.N8NAuthStepRequest) (any, error) {
			return actions.UseN8NDynamicProxy(ctx, actionID, req)
		}
	}
	return n8nDynamicProxyActionRoutes(routes)
}

func n8nDynamicProxyActionRoutes(routes n8nDynamicProxyRoutes) map[string]n8nActionRoute {
	actionRoutes := make(map[string]n8nActionRoute)
	if routes.ProxySettings != nil {
		actionRoutes["proxy-settings"] = n8nAuthStepActionRoute(routes.ProxySettings)
	}
	if routes.RecordProxy != nil {
		actionRoutes["record-proxy"] = n8nProxyRecordActionRoute(routes.RecordProxy)
	}
	if routes.FailProxy != nil {
		actionRoutes["fail-proxy"] = n8nProxyFailureActionRoute(routes.FailProxy)
	}
	if routes.UseProxy != nil {
		actionRoutes["use-proxy"] = n8nAuthStepActionRoute(routes.UseProxy)
	}
	return actionRoutes
}

func n8nAuthStepActionRoute(call n8nAuthStepCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NAuthStepRequest, call.route)
}

func n8nAuthWaitActionRoute(call n8nAuthWaitCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NAuthStepRequest, call.route)
}

func n8nAuthOTPWaitActionRoute(call n8nAuthOTPWaitCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NAuthOTPWaitRequest, call.route)
}

func n8nAuthCompleteActionRoute(call n8nAuthCompleteCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NAuthCompleteRequest, call.route)
}

func n8nAuthFinishActionRoute(call n8nAuthFinishCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NAuthFinishRequest, call.route)
}

func n8nAuthFailureActionRoute(call n8nAuthFailureCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NAuthFailRequest, call.route)
}

func n8nProxyRecordActionRoute(call n8nProxyRecordCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NDynamicProxyRecordRequest, call.route)
}

func n8nProxyCheckActionRoute(call n8nProxyCheckCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NDynamicProxyCheckRequest, call.route)
}

func n8nProxyFailureActionRoute(call n8nProxyFailureCall) n8nActionRoute {
	if call == nil {
		return nil
	}
	return n8nProtoRequestActionRoute(newN8NDynamicProxyFailRequest, call.route)
}

func newN8NAuthStepRequest() *pb.N8NAuthStepRequest {
	return &pb.N8NAuthStepRequest{}
}

func newN8NAuthOTPWaitRequest() *pb.N8NAuthOTPWaitRequest {
	return &pb.N8NAuthOTPWaitRequest{}
}

func newN8NAuthCompleteRequest() *pb.N8NAuthCompleteRequest {
	return &pb.N8NAuthCompleteRequest{}
}

func newN8NAuthFinishRequest() *pb.N8NAuthFinishRequest {
	return &pb.N8NAuthFinishRequest{}
}

func newN8NAuthFailRequest() *pb.N8NAuthFailRequest {
	return &pb.N8NAuthFailRequest{}
}

func newN8NDynamicProxyRecordRequest() *pb.N8NDynamicProxyRecordRequest {
	return &pb.N8NDynamicProxyRecordRequest{}
}

func newN8NDynamicProxyCheckRequest() *pb.N8NDynamicProxyCheckRequest {
	return &pb.N8NDynamicProxyCheckRequest{}
}

func newN8NDynamicProxyFailRequest() *pb.N8NDynamicProxyFailRequest {
	return &pb.N8NDynamicProxyFailRequest{}
}
