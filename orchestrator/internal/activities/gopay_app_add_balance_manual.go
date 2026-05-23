package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) prepareManualTransferAddBalance(ctx context.Context, step activityStep, transfer *pb.GoPayManualTransferAddBalance, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	if s.gopayClient == nil {
		err := fmt.Errorf("gopay-app client not configured")
		data["error_message"] = err.Error()
		return data, err
	}

	data["method"] = "manual_transfer"
	data["status"] = "awaiting_manual_confirmation"
	data["manual_confirmation_required"] = true
	output.Method = "manual_transfer"
	output.Status = "awaiting_manual_confirmation"
	output.Success = true

	step.progress("fetching qr_id from gopay-app", nil)
	resp, err := s.gopayClient.GetQrId(ctx, &pb.GetQrIdRequest{StateJson: output.GetStateJson()})
	if err != nil {
		err = fmt.Errorf("GetQrId: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	if !resp.GetSuccess() {
		err = fmt.Errorf("GetQrId: %s", resp.GetErrorMessage())
		data["error_message"] = err.Error()
		return data, err
	}

	qrPayload := fmt.Sprintf(`{"qr_id":"%s"}`, resp.GetQrId())
	data["manual_transfer"] = map[string]any{
		"configured":         true,
		"qr_payload":         qrPayload,
		"qr_payload_present": true,
		"qr_image_present":   false,
		"instructions":       strings.TrimSpace(transfer.GetInstructions()),
		"amount":             transfer.GetAmount(),
		"currency":           strings.TrimSpace(transfer.GetCurrency()),
	}
	step.progress("waiting for manual gopay transfer confirmation", map[string]any{
		"qr_payload_present": true,
	})
	return data, nil
}
