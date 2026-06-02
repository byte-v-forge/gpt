package activities

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Server) ProtocolAuthEdgeCheckActivity(ctx context.Context, input *ProtocolAuthStartInput) (*ProtocolAuthStartOutput, error) {
	cfg := codexOAuthConfigWithInputProxy(s.codexOAuthSettings(ctx), input)
	mode := strings.TrimSpace(input.GetMode())
	if mode == "" {
		mode = "protocol"
	}
	data := newProtocolAuthEdgeStepData(input, mode)
	output := &ProtocolAuthStartOutput{AccountId: input.GetAccountId(), Data: data.outputData()}
	if strings.TrimSpace(cfg.ProtocolProxyURL) == "" {
		err := fmt.Errorf("protocol proxy url is required")
		data.setResult(false, err)
		output.Data = data.outputData()
		return output, err
	}
	state, err := newProtocolAuthState()
	if err != nil {
		data.setResult(false, err)
		output.Data = data.outputData()
		return output, err
	}
	client, err := s.newAccountGptClient(ctx, input.GetAccountId(), cfg, state)
	if err != nil {
		data.setResult(false, err)
		output.Data = data.outputData()
		return output, err
	}
	defer client.Close()
	checkCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	_, err = protocolAuthChatGPTCSRFAttempt(checkCtx, client, data, 1)
	if err != nil {
		data.setResult(false, err)
		output.Data = data.outputData()
		return output, err
	}
	data.setResult(true, nil)
	output.Data = data.outputData()
	return output, nil
}
