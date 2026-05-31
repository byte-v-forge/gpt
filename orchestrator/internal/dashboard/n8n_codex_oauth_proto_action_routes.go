package dashboard

import (
	"context"

	"orchestrator/pb"
)

type N8NCodexOAuthFlowActions interface {
	CodexOAuthStartFlow(context.Context, string, *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error)
	CodexOAuthDetectFlowStage(context.Context, string, *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitFlowEmail(context.Context, string, *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitFlowPassword(context.Context, string, *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthSubmitFlowEmailOTP(context.Context, string, *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error)
	CodexOAuthAddPhoneFlow(context.Context, string, *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error)
	CodexOAuthCompleteFlow(context.Context, string, *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error)
	CodexOAuthStopFlow(context.Context, string, *pb.CodexOAuthStopBrowserInput) (any, error)
}

type n8nCodexOAuthFlowRouteNames struct {
	Start          string
	Detect         string
	SubmitEmail    string
	SubmitPassword string
	SubmitEmailOTP string
	AddPhone       string
	Complete       string
	Stop           string
}

func n8nCodexOAuthFlowActionRoutes[T N8NCodexOAuthFlowActions](actionID string, actions T, names n8nCodexOAuthFlowRouteNames) map[string]n8nActionRoute {
	routes := map[string]n8nActionRoute{
		firstNonEmptyString(names.Start, "start"): n8nProtoJSONActionRoute(
			newCodexOAuthStartBrowserInput,
			func(ctx context.Context, req *pb.CodexOAuthStartBrowserInput) (*pb.CodexOAuthStartBrowserOutput, error) {
				return actions.CodexOAuthStartFlow(ctx, actionID, req)
			},
		),
		firstNonEmptyString(names.Detect, "detect"): n8nProtoJSONActionRoute(
			newCodexOAuthBrowserStepInput,
			func(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
				return actions.CodexOAuthDetectFlowStage(ctx, actionID, req)
			},
		),
		firstNonEmptyString(names.SubmitEmail, "submit-email"): n8nProtoJSONActionRoute(
			newCodexOAuthBrowserStepInput,
			func(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
				return actions.CodexOAuthSubmitFlowEmail(ctx, actionID, req)
			},
		),
		firstNonEmptyString(names.SubmitPassword, "submit-password"): n8nProtoJSONActionRoute(
			newCodexOAuthBrowserStepInput,
			func(ctx context.Context, req *pb.CodexOAuthBrowserStepInput) (*pb.CodexOAuthBrowserStageOutput, error) {
				return actions.CodexOAuthSubmitFlowPassword(ctx, actionID, req)
			},
		),
		firstNonEmptyString(names.SubmitEmailOTP, "submit-email-otp"): n8nProtoJSONActionRoute(
			newCodexOAuthSubmitEmailOTPInput,
			func(ctx context.Context, req *pb.CodexOAuthSubmitEmailOTPInput) (*pb.CodexOAuthBrowserStageOutput, error) {
				return actions.CodexOAuthSubmitFlowEmailOTP(ctx, actionID, req)
			},
		),
		firstNonEmptyString(names.Complete, "complete"): n8nProtoJSONActionRoute(
			newCodexOAuthCompleteBrowserInput,
			func(ctx context.Context, req *pb.CodexOAuthCompleteBrowserInput) (*pb.CodexOAuthCompleteBrowserOutput, error) {
				return actions.CodexOAuthCompleteFlow(ctx, actionID, req)
			},
		),
		firstNonEmptyString(names.Stop, "stop"): n8nProtoRequestActionRoute(
			newCodexOAuthStopBrowserInput,
			func(ctx context.Context, req *pb.CodexOAuthStopBrowserInput) (any, error) {
				return actions.CodexOAuthStopFlow(ctx, actionID, req)
			},
		),
	}
	if name := firstNonEmptyString(names.AddPhone); name != "" {
		routes[name] = n8nProtoJSONActionRoute(
			newCodexOAuthAddPhoneBrowserInput,
			func(ctx context.Context, req *pb.CodexOAuthAddPhoneBrowserInput) (*pb.CodexOAuthAddPhoneBrowserOutput, error) {
				return actions.CodexOAuthAddPhoneFlow(ctx, actionID, req)
			},
		)
	}
	return routes
}

func newCodexOAuthRequest() *pb.CodexOAuthRequest { return &pb.CodexOAuthRequest{} }

func newCodexOAuthAddPhoneRequest() *pb.CodexOAuthAddPhoneRequest {
	return &pb.CodexOAuthAddPhoneRequest{}
}

func newCodexOAuthBatchAddPhoneRequest() *pb.CodexOAuthBatchAddPhoneRequest {
	return &pb.CodexOAuthBatchAddPhoneRequest{}
}

func newCodexOAuthStartBrowserInput() *pb.CodexOAuthStartBrowserInput {
	return &pb.CodexOAuthStartBrowserInput{}
}

func newCodexOAuthBrowserStepInput() *pb.CodexOAuthBrowserStepInput {
	return &pb.CodexOAuthBrowserStepInput{}
}

func newCodexOAuthSubmitEmailOTPInput() *pb.CodexOAuthSubmitEmailOTPInput {
	return &pb.CodexOAuthSubmitEmailOTPInput{}
}

func newCodexOAuthAddPhoneBrowserInput() *pb.CodexOAuthAddPhoneBrowserInput {
	return &pb.CodexOAuthAddPhoneBrowserInput{}
}

func newCodexOAuthCompleteBrowserInput() *pb.CodexOAuthCompleteBrowserInput {
	return &pb.CodexOAuthCompleteBrowserInput{}
}

func newCodexOAuthStopBrowserInput() *pb.CodexOAuthStopBrowserInput {
	return &pb.CodexOAuthStopBrowserInput{}
}

func newCodexOAuthAcquirePhoneInput() *pb.CodexOAuthAcquirePhoneInput {
	return &pb.CodexOAuthAcquirePhoneInput{}
}

func newCodexOAuthReleasePhoneInput() *pb.CodexOAuthReleasePhoneInput {
	return &pb.CodexOAuthReleasePhoneInput{}
}
