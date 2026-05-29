package activities

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Server) ProtocolAuthEdgeCheckActivity(ctx context.Context, input ProtocolAuthStartInput) (ProtocolAuthStartOutput, error) {
	cfg := codexOAuthConfigWithInputProxy(s.codexOAuthSettings(ctx), input)
	mode := strings.TrimSpace(input.GetMode())
	if mode == "" {
		mode = "protocol"
	}
	data := map[string]any{
		"driver":                 "protocol",
		"mode":                   mode,
		"account_id":             input.GetAccountId(),
		"auth_edge_check_target": "chatgpt_csrf",
	}
	output := ProtocolAuthStartOutput{AccountId: input.GetAccountId(), Data: protoData(data)}
	if strings.TrimSpace(cfg.ProtocolProxyURL) == "" {
		err := fmt.Errorf("protocol proxy url is required")
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, err
	}
	state, err := newProtocolAuthState()
	if err != nil {
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, err
	}
	client, err := s.newAccountGptClient(ctx, input.GetAccountId(), cfg, state)
	if err != nil {
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, err
	}
	defer client.Close()
	checkCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	_, err = protocolAuthChatGPTCSRFAttempt(checkCtx, client, data, 1)
	if err != nil {
		data["auth_edge_accepted"] = false
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, err
	}
	data["auth_edge_accepted"] = true
	data["error_message"] = ""
	output.Data = protoData(data)
	return output, nil
}
