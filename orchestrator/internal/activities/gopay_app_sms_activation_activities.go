package activities

import (
	"context"
	"fmt"
)

func (s *Server) GoPayAppSMSCancelBeforeRotationActivity(ctx context.Context, input GoPayAppSMSActivationInput) (GoPayAppSMSActivationOutput, error) {
	output := GoPayAppSMSActivationOutput{ActivationId: input.GetActivationId()}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
		"reason":        input.GetReason(),
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppChangePhoneCancel, false, true)
	_, err := step.run(func() (any, error) {
		failures := int(input.GetFailureCount())
		reason := input.GetReason()
		if reason == "" {
			reason = "change phone code not received"
		}
		if err := s.recordChangePhoneFailure(ctx, input.GetActivationId(), &failures, reason); err != nil {
			output.FailureCount = int32(failures)
			output.MaxFailures = int32(s.changePhoneMaxFailureCount(ctx))
			output.ErrorMessage = err.Error()
			data["failure_count"] = failures
			data["max_failures"] = s.changePhoneMaxFailureCount(ctx)
			data["error_message"] = err.Error()
			return data, err
		}
		output.FailureCount = int32(failures)
		output.MaxFailures = int32(s.changePhoneMaxFailureCount(ctx))
		data["failure_count"] = failures
		data["max_failures"] = s.changePhoneMaxFailureCount(ctx)
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppSMSFinishActivity(ctx context.Context, input GoPayAppSMSActivationInput) (GoPayAppSMSActivationOutput, error) {
	output := GoPayAppSMSActivationOutput{ActivationId: input.GetActivationId()}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
		"reason":        input.GetReason(),
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppSMSFinish, false, true)
	_, err := step.run(func() (any, error) {
		if input.GetActivationId() == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		s.finishSMSActivation(ctx, input.GetActivationId())
		data["finished"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppSMSRequestAdditionalCodeActivity(ctx context.Context, input GoPayAppSMSActivationInput) (GoPayAppSMSActivationOutput, error) {
	output := GoPayAppSMSActivationOutput{ActivationId: input.GetActivationId()}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
		"reason":        input.GetReason(),
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppSMSRequestMore, false, true)
	_, err := step.run(func() (any, error) {
		if s.smsClient == nil {
			err := fmt.Errorf("sms client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		if input.GetActivationId() == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		if err := s.requestAdditionalSMSCode(ctx, input.GetActivationId(), input.GetJobId()); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		data["requested"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
