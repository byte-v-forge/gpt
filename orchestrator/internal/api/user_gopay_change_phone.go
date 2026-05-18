package api

import (
	"context"
	"fmt"
	"orchestrator/pb"
	"strings"
)

func (s *Server) GoPayUserChangePhoneStart(ctx context.Context, req *pb.GoPayUserChangePhoneStartRequest) (*pb.GoPayUserChangePhoneStartResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserChangePhoneStartResponse{ErrorMessage: err.Error()}, nil
	}
	if strings.TrimSpace(req.GetNewPhone()) == "" {
		return &pb.GoPayUserChangePhoneStartResponse{ErrorMessage: "new_phone is required"}, nil
	}
	if strings.TrimSpace(req.GetPin()) == "" {
		return &pb.GoPayUserChangePhoneStartResponse{ErrorMessage: "pin is required"}, nil
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserChangePhoneStartResponse{ErrorMessage: err.Error()}, nil
	}
	resp, err := s.gopayClient.ChangePhoneStart(ctx, &pb.ChangePhoneStartRequest{
		NewPhone:  req.GetNewPhone(),
		Pin:       req.GetPin(),
		StateJson: stateJSON,
	})
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return &pb.GoPayUserChangePhoneStartResponse{ErrorMessage: fmt.Sprintf("ChangePhoneStart: %v", err)}, nil
	}
	return &pb.GoPayUserChangePhoneStartResponse{
		Success:      resp.GetSuccess(),
		ErrorMessage: resp.GetErrorMessage(),
		NewPhone:     resp.GetNewPhone(),
		OtpSent:      resp.GetOtpSent(),
	}, nil
}

func (s *Server) GoPayUserChangePhoneComplete(ctx context.Context, req *pb.GoPayUserChangePhoneCompleteRequest) (*pb.GoPayUserChangePhoneCompleteResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserChangePhoneCompleteResponse{ErrorMessage: err.Error()}, nil
	}
	if strings.TrimSpace(req.GetOtp()) == "" {
		return &pb.GoPayUserChangePhoneCompleteResponse{ErrorMessage: "otp is required"}, nil
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserChangePhoneCompleteResponse{ErrorMessage: err.Error()}, nil
	}
	resp, err := s.gopayClient.ChangePhoneComplete(ctx, &pb.ChangePhoneCompleteRequest{Otp: req.GetOtp(), StateJson: stateJSON})
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return &pb.GoPayUserChangePhoneCompleteResponse{ErrorMessage: fmt.Sprintf("ChangePhoneComplete: %v", err)}, nil
	}
	return &pb.GoPayUserChangePhoneCompleteResponse{Success: resp.GetSuccess(), ErrorMessage: resp.GetErrorMessage()}, nil
}

func (s *Server) GoPayUserChangePhoneRetry(ctx context.Context, req *pb.GoPayUserChangePhoneRetryRequest) (*pb.GoPayUserChangePhoneRetryResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserChangePhoneRetryResponse{ErrorMessage: err.Error()}, nil
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserChangePhoneRetryResponse{ErrorMessage: err.Error()}, nil
	}
	resp, err := s.gopayClient.ChangePhoneRetry(ctx, &pb.ChangePhoneRetryRequest{StateJson: stateJSON})
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return &pb.GoPayUserChangePhoneRetryResponse{ErrorMessage: fmt.Sprintf("ChangePhoneRetry: %v", err)}, nil
	}
	return &pb.GoPayUserChangePhoneRetryResponse{
		Success:      resp.GetSuccess(),
		ErrorMessage: resp.GetErrorMessage(),
		OtpSent:      resp.GetOtpSent(),
	}, nil
}
