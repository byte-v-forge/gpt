package activities

import (
	"context"
	"fmt"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppChangePhoneRetryActivity(ctx context.Context, input GoPayAppChangePhoneRetryInput) (GoPayAppChangePhoneRetryOutput, error) {
	output := GoPayAppChangePhoneRetryOutput{ActivationId: input.GetActivationId(), StateJson: normalizeGoPayWorkflowStateJSON(input.GetStateJson())}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
		"otp_attempt":   input.GetOtpAttempt(),
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppChangePhoneRetry, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil {
			err := fmt.Errorf("gopay app client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		retryResp, err := s.gopayClient.ChangePhoneRetry(ctx, &pb.ChangePhoneRetryRequest{StateJson: output.GetStateJson()})
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(retryResp))
		if err != nil {
			err = fmt.Errorf("ChangePhoneRetry: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		if retryResp == nil {
			err := fmt.Errorf("ChangePhoneRetry returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		output.OtpSent = retryResp.GetSuccess() && retryResp.GetOtpSent()
		if !retryResp.GetSuccess() {
			output.ErrorMessage = retryResp.GetErrorMessage()
			data["error_message"] = retryResp.GetErrorMessage()
		}
		data["otp_sent"] = output.GetOtpSent()
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
