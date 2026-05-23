package activities

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Server) waitSMSOTP(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	target := input.GetSms()
	activationID := ""
	if target != nil {
		activationID = strings.TrimSpace(target.GetActivationId())
	}
	output := OTPWaitOutput{
		ActivationId: activationID,
		Source:       otpWaitChannelSMS,
	}
	data := map[string]any{
		"channel":       otpWaitChannelSMS,
		"activation_id": activationID,
	}
	stepName := input.GetStepName()
	if stepName == "" {
		stepName = stepGoPayAppChangePhoneSMSWait
	}
	step := s.activityStep(ctx, input.GetJobId(), stepName, false, true)
	_, err := step.run(func() (any, error) {
		if s.smsClient == nil {
			err := fmt.Errorf("code receiver client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		if activationID == "" {
			err := fmt.Errorf("activation id missing")
			data["error_message"] = err.Error()
			return data, err
		}
		timeoutSeconds := input.GetTimeoutSeconds()
		if timeoutSeconds <= 0 {
			timeoutSeconds = s.paymentOtpTimeout()
		}
		data["timeout_seconds"] = timeoutSeconds
		step.progress("waiting for sms otp", data)
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "waiting for sms otp", data)
		defer stopHeartbeat()
		code, err := s.waitSMSCode(ctx, activationID, timeoutSeconds)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if strings.TrimSpace(code) != "" {
			output.Found = true
			output.Code = normalizeOTP(code)
			if input.GetOtpParam() != "" {
				if err := s.setJobParams(ctx, input.GetJobId(), map[string]string{
					input.GetOtpParam():         output.GetCode(),
					input.GetSubmittedAtParam(): fmt.Sprintf("%d", time.Now().Unix()),
				}); err != nil {
					data["error_message"] = err.Error()
					return data, err
				}
			}
			data["found"] = true
			return data, nil
		}
		message := "otp not found"
		output.ErrorMessage = message
		data["found"] = false
		data["error_message"] = message
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}
