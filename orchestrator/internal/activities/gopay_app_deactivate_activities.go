package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppDeactivateStartActivity(ctx context.Context, input GoPayAppDeactivateStartInput) (GoPayAppDeactivateStartOutput, error) {
	output := GoPayAppDeactivateStartOutput{
		ActivationId:   input.GetActivationId(),
		TimeoutSeconds: s.paymentOtpTimeout(ctx),
		StateJson:      normalizeGoPayWorkflowStateJSON(input.GetStateJson()),
	}
	data := map[string]any{"activation_id": input.GetActivationId()}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppDeactivateStart, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil || s.smsClient == nil {
			err := fmt.Errorf("gopay app or code receiver client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		if input.GetActivationId() == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		pin := strings.TrimSpace(input.GetPin())
		if pin == "" {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("gopay pin is required")
			data["error_message"] = err.Error()
			return data, err
		}
		resp, err := s.gopayClient.DeactivateStart(ctx, &pb.DeactivateStartRequest{Pin: pin, StateJson: output.GetStateJson()})
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(resp))
		data["deactivate_start"] = deactivateStartData(resp)
		if err != nil {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err = fmt.Errorf("DeactivateStart: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		if resp == nil {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("DeactivateStart returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		if !resp.GetSuccess() {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("DeactivateStart: %s", resp.GetErrorMessage())
			data["error_message"] = err.Error()
			return data, err
		}
		if !resp.GetOtpSent() {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("DeactivateStart did not send OTP")
			data["error_message"] = err.Error()
			return data, err
		}
		output.OtpRequired = true
		data["otp_required"] = true
		data["timeout_seconds"] = output.GetTimeoutSeconds()
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppDeactivateCompleteActivity(ctx context.Context, input GoPayAppDeactivateCompleteInput) (GoPayAppDeactivateCompleteOutput, error) {
	output := GoPayAppDeactivateCompleteOutput{
		ActivationId: input.GetActivationId(),
		StateJson:    normalizeGoPayWorkflowStateJSON(input.GetStateJson()),
	}
	data := map[string]any{"activation_id": input.GetActivationId()}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppDeactivateComplete, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil || s.smsClient == nil {
			err := fmt.Errorf("gopay app or code receiver client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		if input.GetActivationId() == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		code := strings.TrimSpace(input.GetCode())
		if code == "" && strings.TrimSpace(input.GetOtpParam()) != "" {
			var consumeErr error
			code, consumeErr = s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetIssuedAfterUnix())
			if consumeErr != nil {
				data["error_message"] = consumeErr.Error()
				return data, consumeErr
			}
		}
		if code == "" {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("WaitCode deactivate returned empty code")
			data["error_message"] = err.Error()
			return data, err
		}
		resp, err := s.gopayClient.DeactivateComplete(ctx, &pb.DeactivateCompleteRequest{Otp: code, StateJson: output.GetStateJson()})
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(resp))
		data["deactivate_complete"] = deactivateCompleteData(resp)
		if err != nil {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err = fmt.Errorf("DeactivateComplete: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		if resp == nil {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("DeactivateComplete returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		if !resp.GetSuccess() {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("DeactivateComplete: %s", resp.GetErrorMessage())
			data["error_message"] = err.Error()
			return data, err
		}
		s.finishSMSActivation(ctx, input.GetActivationId())
		output.DeactivateComplete = true
		data["deactivate_complete"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func deactivateStartData(resp *pb.DeactivateStartResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present": true,
		"success":          resp.GetSuccess(),
		"error_message":    resp.GetErrorMessage(),
		"otp_sent":         resp.GetOtpSent(),
	}
}

func deactivateCompleteData(resp *pb.DeactivateCompleteResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present": true,
		"success":          resp.GetSuccess(),
		"error_message":    resp.GetErrorMessage(),
		"deactivated_at":   resp.GetDeactivatedAt(),
	}
}
