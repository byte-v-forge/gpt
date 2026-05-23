package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppChangePhoneCompleteActivity(ctx context.Context, input GoPayAppChangePhoneCompleteInput) (GoPayAppChangePhoneCompleteOutput, error) {
	output := GoPayAppChangePhoneCompleteOutput{
		ActivationId: input.GetActivationId(),
		StateJson:    normalizeGoPayWorkflowStateJSON(input.GetStateJson()),
	}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppChangePhoneComplete, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil {
			err := fmt.Errorf("gopay app client not configured")
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
			err := fmt.Errorf("WaitCode returned empty code")
			data["error_message"] = err.Error()
			return data, err
		}
		completeResp, err := s.gopayClient.ChangePhoneComplete(ctx, &pb.ChangePhoneCompleteRequest{Otp: code, StateJson: output.GetStateJson()})
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(completeResp))
		if err != nil {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err = fmt.Errorf("ChangePhoneComplete: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		if completeResp == nil {
			s.finishSMSActivation(ctx, input.GetActivationId())
			err := fmt.Errorf("ChangePhoneComplete returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		if !completeResp.GetSuccess() {
			failures := int(input.GetFailureCount())
			reason := fmt.Sprintf("ChangePhoneComplete: %s", completeResp.GetErrorMessage())
			if err := s.recordCompletedChangePhoneFailure(ctx, input.GetActivationId(), &failures, reason); err != nil {
				output.FailureCount = int32(failures)
				output.MaxFailures = int32(s.changePhoneMaxFailureCount())
				output.RetryableFailure = true
				output.ErrorMessage = err.Error()
				data["failure_count"] = failures
				data["max_failures"] = s.changePhoneMaxFailureCount()
				data["retryable_failure"] = true
				data["error_message"] = err.Error()
				return data, err
			}
			output.FailureCount = int32(failures)
			output.MaxFailures = int32(s.changePhoneMaxFailureCount())
			output.RetryableFailure = true
			output.ErrorMessage = reason
			data["failure_count"] = failures
			data["max_failures"] = s.changePhoneMaxFailureCount()
			data["retryable_failure"] = true
			data["error_message"] = reason
			return data, nil
		}

		statusAfter, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusAfter))
		data["status_after"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusAfter, statusErr))
		if statusErr != nil {
			data["error_message"] = statusErr.Error()
			return data, statusErr
		}
		output.ChangePhoneComplete = true
		output.Stage = statusAfter.GetStage()
		output.Phone = statusAfter.GetPhone()
		data["change_phone_complete"] = true
		step.progress("phone changed", map[string]any{
			"activation_id": input.GetActivationId(),
		})
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
