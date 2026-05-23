package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) GoPayPaymentOTPResendActivity(ctx context.Context, input GoPayPaymentOTPResendInput) (GoPayPaymentOTPResendOutput, error) {
	output := GoPayPaymentOTPResendOutput{FlowId: strings.TrimSpace(input.GetFlowId())}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPayment, "resending gopay payment otp", map[string]any{
		"account_id_present": input.GetAccountId() != "",
		"flow_id_present":    output.GetFlowId() != "",
	})
	defer stopHeartbeat()

	step := s.activityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	data := protoDataMap(input.GetData())
	if data == nil {
		data = map[string]any{}
	}
	resends, _ := data["payment_otp_resends"].([]any)
	if output.GetFlowId() == "" {
		err := fmt.Errorf("flow_id is required")
		data["payment_otp_resend"] = map[string]any{"success": false, "error_message": err.Error()}
		output.Data = protoData(data)
		step.update(data)
		return output, err
	}

	step.progress("resending gopay payment otp", map[string]any{
		"flow_id_present": true,
		"resend_attempt":  len(resends) + 1,
	})
	resp, err := s.paymentClient.ResendGoPayOTP(ctx, &pb.ResendGoPayOTPRequest{FlowId: output.GetFlowId()})
	item := paymentOTPResendData(resp)
	resends = append(resends, item)
	data["payment_otp_resend"] = item
	data["payment_otp_resends"] = resends
	if resp != nil {
		output.Success = resp.GetSuccess()
		output.FlowId = resp.GetFlowId()
		output.IssuedAfterUnix = resp.GetIssuedAfterUnix()
	}
	output.Data = protoData(data)
	step.update(data)
	if err != nil {
		return output, err
	}
	if resp == nil {
		return output, fmt.Errorf("payment otp resend returned empty response")
	}
	if !resp.GetSuccess() {
		return output, fmt.Errorf("payment otp resend failed: %s", resp.GetErrorMessage())
	}
	step.progress("gopay payment otp resent", map[string]any{
		"issued_after_unix": output.GetIssuedAfterUnix(),
	})
	return output, nil
}

func (s *Server) GoPayPaymentCancelActivity(ctx context.Context, input GoPayPaymentCancelInput) error {
	if strings.TrimSpace(input.GetFlowId()) == "" {
		return nil
	}
	resp, err := s.paymentClient.CancelGoPay(ctx, &pb.CancelGoPayRequest{FlowId: input.GetFlowId()})
	if err != nil {
		return err
	}
	if resp != nil && !resp.GetSuccess() {
		return fmt.Errorf("payment cancel failed: %s", resp.GetErrorMessage())
	}
	return nil
}
