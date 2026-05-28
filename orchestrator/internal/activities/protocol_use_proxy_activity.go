package activities

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) ProtocolUseProxyActivity(ctx context.Context, input ProtocolAuthStartInput) (ProtocolAuthStartOutput, error) {
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
	geo := codexOAuthProtocolProxyGeoFromInput(input.GetCountryCode(), input.GetRegion())
	recordCodexOAuthProtocolProxyRequestGeo(data, geo)
	output := ProtocolAuthStartOutput{AccountId: input.GetAccountId(), Data: protoData(data)}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepProtocolUseProxy, false, true)
	if err != nil {
		return output, err
	}
	step.progress("using protocol proxy", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepProtocolUseProxy, "using protocol proxy", data)
	defer stopHeartbeat()

	if strings.TrimSpace(input.GetProxyUrl()) != "" {
		cfg.ProtocolProxyURL = strings.TrimSpace(input.GetProxyUrl())
	}
	if strings.TrimSpace(cfg.ProtocolProxyURL) == "" {
		err = fmt.Errorf("protocol proxy url is required")
		data["error_message"] = err.Error()
		output.Data = protoData(data)
		return output, step.complete(data, err)
	}
	data["protocol_proxy_in_use"] = true
	data["protocol_proxy_url_source"] = "proxy_runtime"
	output.Data = protoData(data)
	logCodexOAuthProtocolProxyUse(input.GetAccountId(), mode, data)
	return output, step.complete(data, nil)
}
