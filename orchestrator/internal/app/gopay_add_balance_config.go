package app

import (
	"strings"

	"orchestrator/pb"
)

func defaultGoPayAddBalance(cfg orchestratorConfig) *pb.GoPayAddBalance {
	return defaultGoPayAddBalanceForMode(cfg, cfg.GoPayAddBalanceMode)
}

func defaultGoPayAddBalances(cfg orchestratorConfig) map[string]*pb.GoPayAddBalance {
	return map[string]*pb.GoPayAddBalance{
		"manual_transfer": defaultGoPayAddBalanceForMode(cfg, "manual_transfer"),
		"envelope":        defaultGoPayAddBalanceForMode(cfg, "envelope"),
		"rekberinaja":     defaultGoPayAddBalanceForMode(cfg, "rekberinaja"),
	}
}

func defaultGoPayAddBalanceForMode(cfg orchestratorConfig, mode string) *pb.GoPayAddBalance {
	switch normalizeGoPayAddBalanceMode(mode) {
	case "envelope":
		return &pb.GoPayAddBalance{
			Method: &pb.GoPayAddBalance_Envelope{
				Envelope: &pb.GoPayEnvelopeAddBalance{
					Link: strings.TrimSpace(cfg.GoPayAddBalanceEnvelopeLink),
				},
			},
		}
	case "rekberinaja":
		return &pb.GoPayAddBalance{
			Method: &pb.GoPayAddBalance_Rekberinaja{
				Rekberinaja: &pb.GoPayRekberinajaAddBalance{
					EndpointUrl:         strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaEndpoint),
					BearerToken:         strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaToken),
					DeviceId:            strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaDeviceID),
					Store:               strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaStore),
					ProductId:           strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaProductID),
					ServiceId:           strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaServiceID),
					PaymentMethod:       strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaPayment),
					InvoiceEmail:        strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaEmail),
					PromoCode:           strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaPromoCode),
					UsePoin:             cfg.GoPayAddBalanceRekberinajaUsePoin,
					UserAgent:           strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaUserAgent),
					Origin:              strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaOrigin),
					Referer:             strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaReferer),
					RefreshToken:        strings.TrimSpace(cfg.GoPayAddBalanceRekberinajaRefresh),
					FeeTotal:            cfg.GoPayAddBalanceRekberinajaFeeTotal,
					PollTimeoutSeconds:  cfg.GoPayAddBalanceRekberinajaPollTimeout,
					PollIntervalSeconds: cfg.GoPayAddBalanceRekberinajaPollInterval,
				},
			},
		}
	default:
		currency := strings.TrimSpace(cfg.GoPayAddBalanceTransferCurrency)
		if currency == "" {
			currency = "IDR"
		}
		return &pb.GoPayAddBalance{
			Method: &pb.GoPayAddBalance_ManualTransfer{
				ManualTransfer: &pb.GoPayManualTransferAddBalance{
					Instructions: strings.TrimSpace(cfg.GoPayAddBalanceTransferInstructions),
					Amount:       cfg.GoPayAddBalanceTransferAmountRp,
					Currency:     currency,
				},
			},
		}
	}
}

func normalizeGoPayAddBalanceMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "envelope", "claim_envelope", "red_packet", "红包":
		return "envelope"
	case "rekberinaja", "r", "r_platform", "r-platform", "r platform":
		return "rekberinaja"
	case "manual_transfer", "transfer", "qr", "qrcode":
		return "manual_transfer"
	default:
		return "manual_transfer"
	}
}
