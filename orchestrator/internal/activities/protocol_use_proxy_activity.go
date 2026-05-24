package activities

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) ProtocolUseProxyActivity(ctx context.Context, input BrowserAuthStartInput) (BrowserAuthStartOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	mode := strings.TrimSpace(input.GetMode())
	if mode == "" {
		mode = "protocol"
	}
	data := map[string]any{
		"driver":     "protocol",
		"account_id": input.GetAccountId(),
		"mode":       mode,
		"use_proxy":  true,
	}
	output := BrowserAuthStartOutput{AccountId: input.GetAccountId(), Data: protoData(data)}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepProtocolUseProxy, false, true)
	if err != nil {
		return output, err
	}
	step.progress("using protocol proxy", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepProtocolUseProxy, "using protocol proxy", data)
	defer stopHeartbeat()

	if strings.TrimSpace(cfg.ProtocolProxyURL) == "" {
		err = fmt.Errorf("protocol proxy url is required")
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, step.complete(data, err)
	}
	if err = s.startCodexOAuthProtocolProxySession(ctx, cfg, codexOAuthProtocolProxyPurpose(mode), data); err != nil {
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, step.complete(data, err)
	}
	data["protocol_proxy_in_use"] = true
	output.Data = protoData(data)
	logCodexOAuthProtocolProxyUse(input.GetAccountId(), mode, data)
	return output, step.complete(data, nil)
}

func codexOAuthProtocolProxyPurpose(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "protocol"
	}
	return "account_" + strings.NewReplacer(" ", "_", "/", "_", "\\", "_").Replace(mode)
}
