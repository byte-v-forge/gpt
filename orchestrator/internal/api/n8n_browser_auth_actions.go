package api

import (
	"context"

	"orchestrator/pb"
)

func (s *Server) StartN8NBrowserAuth(ctx context.Context, actionID string, req *pb.N8NAuthStepRequest) (any, error) {
	profile, err := n8nBrowserAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.startN8NBrowserAuth(ctx, profile.Lifecycle, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId())
}

func (s *Server) AwaitN8NBrowserAuthOTP(ctx context.Context, actionID string, req *pb.N8NAuthOTPWaitRequest) (any, error) {
	profile, err := n8nBrowserAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.awaitN8NAuthChannelOTP(ctx, profile.OTP, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetChannel(), req.GetTarget(), req.GetTimeoutSeconds(), req.GetOtpIssuedAfterUnix(), req.GetResumeUrl())
}

func (s *Server) CompleteN8NBrowserAuth(ctx context.Context, actionID string, req *pb.N8NAuthCompleteRequest) (any, error) {
	profile, err := n8nBrowserAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.completeN8NBrowserAuth(ctx, profile.Lifecycle, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetOtpSource(), req.GetOtpIssuedAfterUnix())
}

func (s *Server) FinishN8NBrowserAuth(ctx context.Context, actionID string, req *pb.N8NAuthFinishRequest) (any, error) {
	profile, err := n8nBrowserAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.finishN8NAuth(ctx, profile.Finish, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetResultRef())
}

func (s *Server) FailN8NBrowserAuth(ctx context.Context, actionID string, req *pb.N8NAuthFailRequest) (any, error) {
	profile, err := n8nBrowserAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.failN8NAuth(ctx, profile.Fail, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetFlowId(), req.GetErrorMessage(), req.GetData())
}
