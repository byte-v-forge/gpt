package api

import (
	"context"
	"fmt"
	"orchestrator/pb"
)

func (s *Server) userGoPayStatus(ctx context.Context, stateKey string) (*pb.StatusResponse, error) {
	if s.gopayClient == nil {
		return nil, fmt.Errorf("gopay app client not configured")
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return nil, err
	}
	resp, err := s.gopayClient.Status(ctx, &pb.StatusRequest{StateJson: stateJSON})
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return resp, fmt.Errorf("Status: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("Status returned empty response")
	}
	return resp, nil
}

func (s *Server) GoPayUserStatus(ctx context.Context, req *pb.GoPayUserStatusRequest) (*pb.GoPayUserStatusResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserStatusResponse{ErrorMessage: err.Error()}, nil
	}
	status, err := s.userGoPayStatus(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserStatusResponse{ErrorMessage: err.Error(), Status: goPayStatusSnapshot(status, err)}, nil
	}
	return &pb.GoPayUserStatusResponse{Success: true, Status: goPayStatusSnapshot(status, nil)}, nil
}

func (s *Server) GoPayUserClearState(ctx context.Context, req *pb.GoPayUserClearStateRequest) (*pb.GoPayUserClearStateResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserClearStateResponse{ErrorMessage: err.Error()}, nil
	}
	if err := s.deleteGoPayAppStateForUser(ctx, stateKey); err != nil {
		return &pb.GoPayUserClearStateResponse{ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayUserClearStateResponse{Success: true}, nil
}
