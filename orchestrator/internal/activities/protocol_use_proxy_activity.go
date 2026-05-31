package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/contracts"
	"strings"

	"orchestrator/pb"
)

func (s *Server) ProtocolUseProxyActivity(ctx context.Context, input ProtocolAuthStartInput) (ProtocolAuthStartOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	mode := strings.TrimSpace(input.GetMode())
	if mode == "" {
		mode = "protocol"
	}
	data := &pb.ActivityProtocolProxyUseData{
		Driver:    "protocol",
		AccountId: input.GetAccountId(),
		Mode:      mode,
		UseProxy:  boolPtr(true),
	}
	geo := codexOAuthProtocolProxyGeoFromInput(input.GetCountryCode(), input.GetRegion())
	recordCodexOAuthProtocolProxyRequestGeo(data, geo)
	output := ProtocolAuthStartOutput{AccountId: input.GetAccountId(), Data: protocolAuthOutputData(data)}
	step, err := s.startActivityStep(ctx, input.GetJobId(), contracts.StepProtocolUseProxy, false, true)
	if err != nil {
		return output, err
	}
	step.progress("using protocol proxy", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepProtocolUseProxy, "using protocol proxy", data)
	defer stopHeartbeat()

	if strings.TrimSpace(input.GetProxyUrl()) != "" {
		cfg.ProtocolProxyURL = strings.TrimSpace(input.GetProxyUrl())
	}
	if strings.TrimSpace(cfg.ProtocolProxyURL) == "" {
		err = fmt.Errorf("protocol proxy url is required")
		data.ErrorMessage = err.Error()
		output.Data = protocolAuthOutputData(data)
		return output, step.complete(data, err)
	}
	data.ProtocolProxyInUse = boolPtr(true)
	data.ProtocolProxyUrlSource = "proxy_runtime"
	output.Data = protocolAuthOutputData(data)
	logCodexOAuthProtocolProxyUse(input.GetAccountId(), mode, data.GetProtocolProxyUrlSource())
	return output, step.complete(data, nil)
}
