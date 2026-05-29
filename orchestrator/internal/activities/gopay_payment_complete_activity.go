package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) GoPayPaymentCompleteActivity(ctx context.Context, input GoPayPaymentCompleteInput) (GoPayActivityOutput, error) {
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPayment, "completing gopay payment", map[string]any{
		"account_id_present": input.GetAccountId() != "",
		"flow_id_present":    input.GetFlowId() != "",
		"otp_source":         input.GetOtpSource(),
		"use_account_token":  input.GetUseAccountToken(),
	})
	defer stopHeartbeat()

	step := s.activityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	data := protoDataMap(input.GetData())
	if data == nil {
		data = map[string]any{}
	}
	data["payment_otp"] = map[string]any{
		"timeout_seconds":    s.paymentOtpTimeout(ctx),
		"issued_after_unix":  input.GetOtpIssuedAfterUnix(),
		"found":              input.GetOtpSource() != "not_required",
		"source":             input.GetOtpSource(),
		"manual_allowed":     true,
		"otp_value_recorded": false,
	}
	otp := ""
	if input.GetOtpSource() == "not_required" {
		step.progress("payment otp not required", nil)
	} else {
		step.progress("payment otp received", map[string]any{
			"source": input.GetOtpSource(),
		})
		var err error
		otp, err = s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetOtpIssuedAfterUnix())
		if err != nil {
			return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
		}
	}

	result, stateJSON, err := s.completeGoPayPayment(ctx, step, stateJSON, input.GetFlowId(), otp, input.GetUseAccountToken(), input.GetPin(), input.GetWaitForManualConfirmation(), data)
	if err != nil {
		return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
	}

	settled := result.GetSuccess() && !result.GetAwaitingManualConfirmation() && result.GetChargeRef() != ""
	data["payment_settled"] = settled
	data["payment_async_pending"] = false
	output := GoPayActivityOutput{
		ChargeRef:                  result.GetChargeRef(),
		SnapToken:                  result.GetSnapToken(),
		PlusTrialEligible:          settled,
		PlusTrialChecked:           settled,
		PlusActive:                 settled,
		AwaitingManualConfirmation: result.GetAwaitingManualConfirmation(),
		FlowId:                     input.GetFlowId(),
		Data:                       protoData(data),
		StateJson:                  stateJSON,
	}
	if result.GetAwaitingManualConfirmation() {
		step.progress("waiting for manual qris payment confirmation", map[string]any{
			"charge_ref": result.GetChargeRef(),
			"qr_present": result.GetQrString() != "" || result.GetQrCodeUrl() != "",
		})
		step.update(data)
		return output, nil
	}
	return output, step.complete(data, nil)
}

func (s *Server) GoPayPaymentManualConfirmActivity(ctx context.Context, input GoPayPaymentManualConfirmInput) (GoPayActivityOutput, error) {
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPayment, "confirming manual gopay payment", map[string]any{
		"account_id_present": input.GetAccountId() != "",
		"flow_id_present":    input.GetFlowId() != "",
	})
	defer stopHeartbeat()

	step := s.activityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	data := protoDataMap(input.GetData())
	if data == nil {
		data = map[string]any{}
	}
	flowID := strings.TrimSpace(input.GetFlowId())
	if flowID == "" {
		err := fmt.Errorf("flow_id is required")
		data["manual_payment_confirmation"] = map[string]any{"required": true, "confirmed": false, "error_message": err.Error()}
		return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
	}
	step.progress("confirming manual qris payment", map[string]any{"flow_id_present": true})
	result, err := s.paymentClient.ConfirmGoPayPayment(ctx, &pb.ConfirmGoPayPaymentRequest{FlowId: flowID})
	data["payment_confirm"] = paymentResultData(result)
	data["payment_result_present"] = result != nil
	if err != nil {
		return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
	}
	if result == nil {
		err = fmt.Errorf("payment confirm returned empty response")
		return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
	}
	if !result.GetSuccess() {
		err = fmt.Errorf("payment confirm failed: %s", result.GetErrorMessage())
		return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
	}
	data["manual_payment_confirmation"] = map[string]any{
		"required":      true,
		"auto_expected": false,
		"confirmed":     true,
	}
	output := GoPayActivityOutput{
		ChargeRef:         result.GetChargeRef(),
		SnapToken:         result.GetSnapToken(),
		PlusTrialEligible: true,
		PlusTrialChecked:  true,
		PlusActive:        true,
		Data:              protoData(data),
		StateJson:         stateJSON,
	}
	return output, step.complete(data, nil)
}
