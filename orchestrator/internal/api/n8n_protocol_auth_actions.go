package api

import (
	"context"

	"orchestrator/pb"
)

func (s *Server) CheckN8NProtocolAuthEdge(ctx context.Context, actionID string, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.checkN8NProtocolAuthEdge(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetProxyUrl(), profile.Proxy)
}

func (s *Server) StartN8NProtocolAuth(ctx context.Context, actionID string, req *pb.N8NAuthStepRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.startN8NProtocolAuth(ctx, profile.Protocol.Lifecycle, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId())
}

func (s *Server) WaitN8NProtocolAuth(ctx context.Context, actionID string, req *pb.N8NAuthStepRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.waitN8NProtocolAuth(ctx, profile.Protocol.Lifecycle, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetEmail())
}

func (s *Server) AwaitN8NProtocolAuthOTP(ctx context.Context, actionID string, req *pb.N8NAuthOTPWaitRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.awaitN8NAuthChannelOTP(ctx, profile.Protocol.OTP, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetChannel(), req.GetTarget(), req.GetTimeoutSeconds(), req.GetOtpIssuedAfterUnix(), req.GetResumeUrl())
}

func (s *Server) CompleteN8NProtocolAuth(ctx context.Context, actionID string, req *pb.N8NAuthCompleteRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.completeN8NProtocolAuth(ctx, profile.Protocol.Lifecycle, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetOtpSource(), req.GetOtpIssuedAfterUnix())
}

func (s *Server) FinishN8NProtocolAuth(ctx context.Context, actionID string, req *pb.N8NAuthFinishRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.finishN8NAuth(ctx, profile.Protocol.Finish, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetResultRef())
}

func (s *Server) FailN8NProtocolAuth(ctx context.Context, actionID string, req *pb.N8NAuthFailRequest) (any, error) {
	profile, err := n8nProtocolAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.failN8NAuth(ctx, profile.Protocol.Fail, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetErrorMessage(), req.GetData())
}
