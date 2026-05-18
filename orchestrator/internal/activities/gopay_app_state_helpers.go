package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	pb "orchestrator/pb"
)

func normalizeGoPayWorkflowStateJSON(stateJSON string) string {
	stateJSON = strings.TrimSpace(stateJSON)
	if stateJSON == "" {
		return "{}"
	}
	return stateJSON
}

func goPayWorkflowStatePresent(stateJSON string) bool {
	stateJSON = normalizeGoPayWorkflowStateJSON(stateJSON)
	var state map[string]any
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return false
	}
	return len(state) > 0
}

func goPayWorkflowStateAfter(current, next string) string {
	next = strings.TrimSpace(next)
	if next != "" {
		return next
	}
	return normalizeGoPayWorkflowStateJSON(current)
}

type goPayStateJSONResponse interface {
	GetStateJson() string
}

func responseStateJSON(resp goPayStateJSONResponse) string {
	if resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.GetStateJson())
}

func (s *Server) goPayStatusForState(ctx context.Context, stateJSON string) (*pb.StatusResponse, error) {
	if s.gopayClient == nil {
		return nil, fmt.Errorf("gopay app client not configured")
	}
	resp, err := s.gopayClient.Status(ctx, &pb.StatusRequest{StateJson: normalizeGoPayWorkflowStateJSON(stateJSON)})
	if err != nil {
		return resp, fmt.Errorf("Status: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("Status returned empty response")
	}
	return resp, nil
}

func (s *Server) goPayStatus(ctx context.Context) (*pb.StatusResponse, error) {
	if s.gopayClient == nil {
		return nil, fmt.Errorf("gopay app client not configured")
	}
	stateJSON, err := s.loadGoPayAppState(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := s.gopayClient.Status(ctx, &pb.StatusRequest{StateJson: stateJSON})
	if err == nil {
		err = s.saveGoPayAppState(ctx, resp.GetStateJson())
	}
	if err != nil {
		return resp, fmt.Errorf("Status: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("Status returned empty response")
	}
	return resp, nil
}

func (s *Server) loadGoPayAppState(ctx context.Context) (string, error) {
	return s.loadGoPayAppStateForUser(ctx, goPayAppStateKey)
}

func (s *Server) loadGoPayAppStateForUser(ctx context.Context, stateKey string) (string, error) {
	if s.gopayClient == nil {
		return "{}", fmt.Errorf("gopay-app client not configured")
	}
	resp, err := s.gopayClient.GetGoPayState(ctx, &pb.GetGoPayStateRequest{UserId: stateKey})
	if err != nil {
		return "", fmt.Errorf("GetGoPayState: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("GetGoPayState returned empty response")
	}
	if !resp.GetSuccess() {
		return "", fmt.Errorf("GetGoPayState: %s", resp.GetErrorMessage())
	}
	stateJSON := strings.TrimSpace(resp.GetStateJson())
	if stateJSON == "" {
		stateJSON = "{}"
	}
	return stateJSON, nil
}

func (s *Server) saveGoPayAppState(ctx context.Context, stateJSON string) error {
	return s.saveGoPayAppStateForUser(ctx, goPayAppStateKey, stateJSON)
}

func (s *Server) saveGoPayAppStateForUser(ctx context.Context, stateKey string, stateJSON string) error {
	stateJSON = strings.TrimSpace(stateJSON)
	if stateJSON == "" {
		return nil
	}
	if s.gopayClient == nil {
		return fmt.Errorf("gopay-app client not configured")
	}
	resp, err := s.gopayClient.UpsertGoPayState(ctx, &pb.UpsertGoPayStateRequest{
		UserId:    stateKey,
		StateJson: stateJSON,
	})
	if err != nil {
		return fmt.Errorf("UpsertGoPayState: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("UpsertGoPayState returned empty response")
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("UpsertGoPayState: %s", resp.GetErrorMessage())
	}
	return nil
}

func (s *Server) deleteGoPayAppStateForUser(ctx context.Context, stateKey string) error {
	if s.gopayClient == nil {
		return fmt.Errorf("gopay-app client not configured")
	}
	resp, err := s.gopayClient.DeleteGoPayState(ctx, &pb.DeleteGoPayStateRequest{UserId: stateKey})
	if err != nil {
		return fmt.Errorf("DeleteGoPayState: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("DeleteGoPayState returned empty response")
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("DeleteGoPayState: %s", resp.GetErrorMessage())
	}
	return nil
}

func configuredGoPayPhone() string {
	return normalizeIndonesiaPhone(os.Getenv("GOPAY_PHONE_NUMBER"))
}

func configuredGoPayWAPhone() string {
	return normalizeIndonesiaPhone(os.Getenv("GOPAY_WA_PHONE_NUMBER"))
}

func configuredGoPayPIN() string {
	return strings.TrimSpace(os.Getenv("GOPAY_PIN"))
}

func configuredGoPayCountryCode() string {
	value := strings.TrimSpace(os.Getenv("GOPAY_COUNTRY_CODE"))
	if value == "" {
		value = "62"
	}
	if !strings.HasPrefix(value, "+") {
		value = "+" + value
	}
	return value
}
