package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppChangePhoneStartActivity(ctx context.Context, input GoPayAppChangePhoneStartInput) (GoPayAppChangePhoneStartOutput, error) {
	output := GoPayAppChangePhoneStartOutput{
		ActivationId: strings.TrimSpace(input.GetActivationId()),
		Phone:        normalizeIndonesiaPhone(input.GetPhone()),
		StateJson:    normalizeGoPayWorkflowStateJSON(input.GetStateJson()),
	}
	data := map[string]any{
		"activation_id": output.GetActivationId(),
		"phone_present": output.GetPhone() != "",
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppChangePhoneStart, false, true)
	_, err := step.run(func() (any, error) {
		failStart := func(err error, retryable bool) (any, error) {
			output.ErrorMessage = err.Error()
			output.RetryableFailure = retryable
			data["error_message"] = err.Error()
			if retryable {
				data["retryable_failure"] = true
			}
			return data, nil
		}
		failures := int(input.GetFailureCount())
		if failures < 0 {
			failures = 0
		}
		maxFailures := s.changePhoneMaxFailureCount(ctx)
		otpWaitSeconds := s.paymentOtpTimeout(ctx)
		otpRetryAttempts := s.changePhoneOTPRetryCount(ctx)
		output.FailureCount = int32(failures)
		output.MaxFailures = int32(maxFailures)
		output.OtpTimeoutSeconds = otpWaitSeconds
		output.OtpRetryAttempts = int32(otpRetryAttempts)
		data["failure_count"] = failures
		data["max_failures"] = maxFailures
		data["otp_timeout_seconds"] = otpWaitSeconds
		data["otp_retry_attempts"] = otpRetryAttempts

		if s.changePhoneDisabledValue(ctx) {
			err := fmt.Errorf("gopay change phone disabled by gopay.change_phone_disabled")
			return failStart(err, false)
		}
		if s.gopayClient == nil {
			err := fmt.Errorf("gopay app client not configured")
			return failStart(err, false)
		}
		if output.GetPhone() == "" {
			err := fmt.Errorf("change phone number missing")
			return failStart(err, false)
		}

		statusBefore, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusBefore))
		data["status_before"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusBefore, statusErr))
		if statusErr != nil {
			return failStart(statusErr, false)
		}

		pin := strings.TrimSpace(input.GetPin())
		if pin == "" {
			err := fmt.Errorf("gopay pin is required")
			return failStart(err, false)
		}

		changeResp, err := s.gopayClient.ChangePhoneStart(ctx, &pb.ChangePhoneStartRequest{
			NewPhone:    output.GetPhone(),
			Pin:         pin,
			CountryCode: input.GetCountryCode(),
			StateJson:   output.GetStateJson(),
		})
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(changeResp))
		if err != nil {
			err = fmt.Errorf("ChangePhoneStart: %w", err)
			return failStart(err, false)
		}
		if changeResp == nil {
			err := fmt.Errorf("ChangePhoneStart returned empty response")
			return failStart(err, false)
		}
		if !changeResp.GetSuccess() {
			reason := fmt.Sprintf("ChangePhoneStart: %s", changeResp.GetErrorMessage())
			if output.GetActivationId() != "" && changePhoneStartRetryableError(changeResp.GetErrorMessage()) {
				output.FailureCount = int32(failures)
				data["failure_count"] = failures
				return failStart(fmt.Errorf("%s", reason), true)
			}
			err := fmt.Errorf("%s", reason)
			return failStart(err, false)
		}

		step.progress("change phone otp sent", map[string]any{
			"activation_id": output.GetActivationId(),
		})
		if output.GetActivationId() != "" {
			if err := s.markSMSMessageSent(ctx, output.GetActivationId(), input.GetJobId()); err != nil {
				return failStart(err, false)
			}
		}

		data["change_phone_start_complete"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
