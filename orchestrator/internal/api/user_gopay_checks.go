package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) GoPayUserCheckPhone(ctx context.Context, req *pb.GoPayUserCheckPhoneRequest) (*pb.GoPayUserCheckPhoneResponse, error) {
	if _, err := normalizeGoPayUserID(req.GetUserId()); err != nil {
		return &pb.GoPayUserCheckPhoneResponse{ErrorMessage: err.Error()}, nil
	}
	if strings.TrimSpace(req.GetPhone()) == "" {
		return &pb.GoPayUserCheckPhoneResponse{ErrorMessage: "phone is required"}, nil
	}
	if s.gopayClient == nil {
		return &pb.GoPayUserCheckPhoneResponse{ErrorMessage: "gopay-app client not configured"}, nil
	}
	resp, err := s.gopayClient.CheckPhone(ctx, &pb.CheckPhoneRequest{
		Phone:       req.GetPhone(),
		CountryCode: req.GetCountryCode(),
	})
	if err != nil {
		return &pb.GoPayUserCheckPhoneResponse{ErrorMessage: fmt.Sprintf("CheckPhone: %v", err)}, nil
	}
	if resp == nil {
		return &pb.GoPayUserCheckPhoneResponse{ErrorMessage: "CheckPhone returned empty response"}, nil
	}
	status := strings.TrimSpace(resp.GetStatus())
	success := status == "available" || status == "registered"
	return &pb.GoPayUserCheckPhoneResponse{
		Success:      success,
		ErrorMessage: resp.GetErrorMessage(),
		Available:    resp.GetAvailable(),
		Status:       status,
	}, nil
}

func (s *Server) GoPayUserCheckBalance(ctx context.Context, req *pb.GoPayUserCheckBalanceRequest) (*pb.GoPayUserCheckBalanceResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserCheckBalanceResponse{ErrorMessage: err.Error()}, nil
	}
	stateJSON, err := s.loadGoPayAppStateForUser(ctx, stateKey)
	if err != nil {
		return &pb.GoPayUserCheckBalanceResponse{ErrorMessage: err.Error()}, nil
	}
	resp, err := s.gopayClient.CheckTokenValid(ctx, &pb.CheckTokenValidRequest{StateJson: stateJSON})
	if resp == nil && err == nil {
		err = fmt.Errorf("CheckTokenValid returned empty response")
	}
	if err == nil {
		err = s.saveGoPayAppStateForUser(ctx, stateKey, resp.GetStateJson())
	}
	if err != nil {
		return &pb.GoPayUserCheckBalanceResponse{ErrorMessage: fmt.Sprintf("CheckTokenValid: %v", err)}, nil
	}
	return &pb.GoPayUserCheckBalanceResponse{
		Success:         resp.GetSuccess(),
		ErrorMessage:    resp.GetErrorMessage(),
		Stage:           resp.GetStage(),
		Phone:           resp.GetPhone(),
		TokenValid:      resp.GetTokenValid(),
		Refreshed:       resp.GetRefreshed(),
		BalanceAmount:   resp.GetBalanceAmount(),
		HasMinBalance:   resp.GetHasMinBalance(),
		BalanceCurrency: resp.GetBalanceCurrency(),
	}, nil
}
