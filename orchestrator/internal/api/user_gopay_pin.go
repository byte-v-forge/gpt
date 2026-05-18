package api

import (
	"context"
	"fmt"
	"orchestrator/pb"
	"strings"
)

func (s *Server) GoPayUserCreatePinStart(ctx context.Context, req *pb.GoPayUserCreatePinStartRequest) (*pb.GoPayUserCreatePinStartResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserCreatePinStartResponse{ErrorMessage: err.Error()}, nil
	}
	if strings.TrimSpace(req.GetPin()) == "" {
		return &pb.GoPayUserCreatePinStartResponse{ErrorMessage: "pin is required"}, nil
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserCreatePinStartResponse{ErrorMessage: err.Error()}, nil
	}
	resp, err := s.gopayClient.CreatePinStart(ctx, &pb.CreatePinStartRequest{Pin: req.GetPin(), OtpChannel: req.GetOtpChannel(), StateJson: stateJSON})
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return &pb.GoPayUserCreatePinStartResponse{ErrorMessage: fmt.Sprintf("CreatePinStart: %v", err)}, nil
	}
	return &pb.GoPayUserCreatePinStartResponse{
		Success:            resp.GetSuccess(),
		ErrorMessage:       resp.GetErrorMessage(),
		OtpSent:            resp.GetOtpSent(),
		VerificationId:     resp.GetVerificationId(),
		VerificationMethod: resp.GetVerificationMethod(),
	}, nil
}

func (s *Server) GoPayUserCreatePinComplete(ctx context.Context, req *pb.GoPayUserCreatePinCompleteRequest) (*pb.GoPayUserCreatePinCompleteResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserCreatePinCompleteResponse{ErrorMessage: err.Error()}, nil
	}
	if strings.TrimSpace(req.GetOtp()) == "" {
		return &pb.GoPayUserCreatePinCompleteResponse{ErrorMessage: "otp is required"}, nil
	}
	if strings.TrimSpace(req.GetPin()) == "" {
		return &pb.GoPayUserCreatePinCompleteResponse{ErrorMessage: "pin is required"}, nil
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserCreatePinCompleteResponse{ErrorMessage: err.Error()}, nil
	}
	resp, err := s.gopayClient.CreatePinComplete(ctx, &pb.CreatePinCompleteRequest{
		Otp:       req.GetOtp(),
		Pin:       req.GetPin(),
		StateJson: stateJSON,
	})
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return &pb.GoPayUserCreatePinCompleteResponse{ErrorMessage: fmt.Sprintf("CreatePinComplete: %v", err)}, nil
	}
	return &pb.GoPayUserCreatePinCompleteResponse{
		Success:          resp.GetSuccess(),
		ErrorMessage:     resp.GetErrorMessage(),
		Phone:            resp.GetPhone(),
		PinSetupComplete: resp.GetPinSetupComplete(),
	}, nil
}
