package api

import (
	"context"
	"strings"

	"orchestrator/internal/gptsettings"
	"orchestrator/pb"
)

func (s *Server) pluginValues(ctx context.Context, pluginKey string) map[string]string {
	defaults := s.pluginDefaultValues(pluginKey)
	if s.gptSettings == nil {
		return defaults
	}
	settings, err := s.gptSettings.Get(ctx)
	if err != nil {
		return defaults
	}
	return gptsettings.MergePluginValues(defaults, gptsettings.PluginValues(settings, pluginKey))
}

func (s *Server) pluginDefaultValues(pluginKey string) map[string]string {
	if s == nil || s.actionRegistry == nil {
		return nil
	}
	return s.actionRegistry.PluginDefaults(pluginKey)
}

func (s *Server) goPayPluginValues(ctx context.Context) map[string]string {
	return s.pluginValues(ctx, "gopay")
}

func (s *Server) goPayAddBalanceConfirmTimeout(ctx context.Context, requestValue int32) int32 {
	if requestValue > 0 {
		return requestValue
	}
	return gptsettings.Int32Value(s.goPayPluginValues(ctx), "add_balance_confirm_timeout_seconds", 0)
}

func (s *Server) configuredGoPayAddBalance(ctx context.Context, mode string) *pb.GoPayAddBalance {
	values := s.goPayPluginValues(ctx)
	switch normalizeGoPayAddBalanceMode(mode) {
	case "envelope":
		return &pb.GoPayAddBalance{Method: &pb.GoPayAddBalance_Envelope{Envelope: &pb.GoPayEnvelopeAddBalance{
			Link: gptsettings.StringValue(values, "add_balance_envelope_link", ""),
		}}}
	default:
		currency := gptsettings.StringValue(values, "add_balance_transfer_currency", "")
		return &pb.GoPayAddBalance{Method: &pb.GoPayAddBalance_ManualTransfer{ManualTransfer: &pb.GoPayManualTransferAddBalance{
			Instructions: gptsettings.StringValue(values, "add_balance_transfer_instructions", ""),
			Amount:       int64(gptsettings.IntValue(values, "add_balance_transfer_amount_rp", 0)),
			Currency:     currency,
		}}}
	}
}

func (s *Server) configuredGoPayAddBalanceByMethod(ctx context.Context, method string) *pb.GoPayAddBalance {
	return s.configuredGoPayAddBalance(ctx, method)
}

func normalizeGoPayAddBalanceMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "envelope", "claim_envelope", "red_packet", "红包":
		return "envelope"
	case "manual_transfer", "transfer", "qr", "qrcode":
		return "manual_transfer"
	default:
		return "manual_transfer"
	}
}
