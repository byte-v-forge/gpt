package activities

import (
	"context"
	"fmt"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppAcquireSignupPhoneActivity(ctx context.Context, input GoPayAppAcquireSignupPhoneInput) (GoPayAppAcquireSignupPhoneOutput, error) {
	output := GoPayAppAcquireSignupPhoneOutput{}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppSignupPhone, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil || s.smsClient == nil {
			err := fmt.Errorf("gopay app or sms client not configured")
			data["error_message"] = err.Error()
			return data, err
		}

		failures := int(input.GetFailureCount())
		if failures < 0 {
			failures = 0
		}
		otpWaitSeconds := s.paymentOtpTimeout()
		data["failure_count"] = failures
		data["otp_timeout_seconds"] = otpWaitSeconds

		step.progress("acquiring unregistered gopay phone", map[string]any{
			"failure_count": failures,
		})
		activationID, phone, err := s.acquireSMSNumber(ctx, goPaySMSRequest(input.GetJobId(), map[string]string{
			"workflow": "gopay_signup",
			"job_id":   input.GetJobId(),
		}))
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}

		phone = normalizeIndonesiaPhone(phone)
		failures++
		output.ActivationId = activationID
		output.Phone = phone
		output.FailureCount = int32(failures)
		output.OtpTimeoutSeconds = otpWaitSeconds
		data["activation_id"] = activationID
		data["phone_present"] = phone != ""
		data["failure_count"] = failures
		if phone == "" {
			err := fmt.Errorf("signup phone missing")
			if cancelErr := s.cancelSMSActivationForFailure(ctx, activationID, "discard signup phone"); cancelErr != nil {
				err = fmt.Errorf("%w; cleanup: %v", err, cancelErr)
			}
			data["error_message"] = err.Error()
			return data, err
		}

		checkResp, err := s.gopayClient.CheckPhone(ctx, &pb.CheckPhoneRequest{Phone: phone})
		if err != nil {
			data["phone_status"] = "error"
			err = fmt.Errorf("CheckPhone: %w", err)
			if cancelErr := s.cancelSMSActivationForFailure(ctx, activationID, "discard signup phone"); cancelErr != nil {
				err = fmt.Errorf("%w; cleanup: %v", err, cancelErr)
			}
			data["error_message"] = err.Error()
			return data, err
		}
		status := checkPhoneStatus(checkResp)
		data["phone_status"] = status
		step.progress("gopay phone availability checked", map[string]any{
			"activation_id": activationID,
			"status":        status,
		})
		if status == "registered" {
			err := fmt.Errorf("signup phone already registered")
			if cancelErr := s.cancelSMSActivationForFailure(ctx, activationID, "signup phone already registered"); cancelErr != nil {
				err = fmt.Errorf("%w; cleanup: %v", err, cancelErr)
			}
			data["error_message"] = err.Error()
			return data, err
		}
		if status != "available" {
			err := fmt.Errorf("signup phone unavailable: %s", status)
			if cancelErr := s.cancelSMSActivationForFailure(ctx, activationID, "discard signup phone"); cancelErr != nil {
				err = fmt.Errorf("%w; cleanup: %v", err, cancelErr)
			}
			data["error_message"] = err.Error()
			return data, err
		}

		data["signup_phone_acquired"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppDiscardSignupPhoneActivity(ctx context.Context, input GoPayAppSMSActivationInput) (GoPayAppSMSActivationOutput, error) {
	output := GoPayAppSMSActivationOutput{
		ActivationId: input.GetActivationId(),
		FailureCount: input.GetFailureCount(),
	}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
		"reason":        input.GetReason(),
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppSignupPhoneCancel, false, true)
	_, err := step.run(func() (any, error) {
		if input.GetActivationId() == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		reason := input.GetReason()
		if reason == "" {
			reason = "discard signup phone"
		}
		if err := s.cancelSMSActivationForFailure(ctx, input.GetActivationId(), reason); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		data["canceled"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
