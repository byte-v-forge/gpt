package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppAcquireSignupPhoneActivity(ctx context.Context, input GoPayAppAcquireSignupPhoneInput) (GoPayAppAcquireSignupPhoneOutput, error) {
	output := GoPayAppAcquireSignupPhoneOutput{}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppSignupPhone, false, true)
	_, err := step.run(func() (any, error) {
		if s.smsClient == nil {
			err := fmt.Errorf("sms client not configured")
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

		step.progress("acquiring gopay signup phone", map[string]any{
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

		data["signup_phone_acquired"] = true
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppGenerateDeviceProxyActivity(ctx context.Context, input GoPayAppGenerateDeviceProxyInput) (GoPayAppGenerateDeviceProxyOutput, error) {
	output := GoPayAppGenerateDeviceProxyOutput{}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppGenerateDeviceProxy, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil {
			err := fmt.Errorf("gopay app client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		resp, err := s.gopayClient.GenerateDeviceProxy(ctx, &pb.GenerateDeviceProxyRequest{})
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if resp == nil {
			err := fmt.Errorf("GenerateDeviceProxy returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		output.StateJson = resp.GetStateJson()
		output.ProxySlot = resp.GetProxySlot()
		output.DynamicEgressSize = resp.GetDynamicEgressSize()
		output.ProxyHash = resp.GetProxyHash()
		output.DeviceFingerprint = resp.GetDeviceFingerprint()
		data["proxy_slot"] = output.GetProxySlot()
		data["dynamic_egress_size"] = output.GetDynamicEgressSize()
		data["proxy_hash"] = output.GetProxyHash()
		data["device_fingerprint"] = output.GetDeviceFingerprint()
		data["state_diagnostics"] = goPayAppStateDiagnostics(output.GetStateJson())
		data["state_generated"] = output.GetStateJson() != ""
		if !resp.GetSuccess() {
			err := fmt.Errorf("GenerateDeviceProxy: %s", resp.GetErrorMessage())
			data["error_message"] = err.Error()
			return data, err
		}
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppCheckSignupPhoneActivity(ctx context.Context, input GoPayAppCheckSignupPhoneInput) (GoPayAppCheckSignupPhoneOutput, error) {
	output := GoPayAppCheckSignupPhoneOutput{
		ActivationId: input.GetActivationId(),
		Phone:        input.GetPhone(),
		StateJson:    input.GetStateJson(),
	}
	data := map[string]any{
		"activation_id": input.GetActivationId(),
		"phone_present": input.GetPhone() != "",
		"state_present": strings.TrimSpace(input.GetStateJson()) != "",
	}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppCheckPhone, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil {
			err := fmt.Errorf("gopay app client not configured")
			data["error_message"] = err.Error()
			return data, err
		}
		if input.GetPhone() == "" {
			err := fmt.Errorf("signup phone missing")
			data["error_message"] = err.Error()
			return data, err
		}
		if strings.TrimSpace(input.GetStateJson()) == "" {
			err := fmt.Errorf("generated device proxy state_json missing")
			data["error_message"] = err.Error()
			return data, err
		}
		resp, err := s.gopayClient.CheckPhone(ctx, &pb.CheckPhoneRequest{
			Phone:       input.GetPhone(),
			CountryCode: input.GetCountryCode(),
			StateJson:   input.GetStateJson(),
		})
		if err != nil {
			err = fmt.Errorf("CheckPhone: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		if resp == nil {
			err := fmt.Errorf("CheckPhone returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		status := checkPhoneStatus(resp)
		output.Status = status
		output.Available = resp.GetAvailable()
		output.StateJson = resp.GetStateJson()
		output.ProxySlot = resp.GetProxySlot()
		output.DynamicEgressSize = resp.GetDynamicEgressSize()
		output.ProxyHash = resp.GetProxyHash()
		output.DeviceFingerprint = resp.GetDeviceFingerprint()
		data["phone_status"] = status
		data["available"] = output.GetAvailable()
		data["proxy_slot"] = output.GetProxySlot()
		data["dynamic_egress_size"] = output.GetDynamicEgressSize()
		data["proxy_hash"] = output.GetProxyHash()
		data["device_fingerprint"] = output.GetDeviceFingerprint()
		data["state_diagnostics"] = goPayAppStateDiagnostics(output.GetStateJson())
		data["state_pinned"] = output.GetStateJson() != ""
		checkError := strings.TrimSpace(resp.GetErrorMessage())
		if checkError != "" {
			data["check_phone_error_message"] = checkError
		}
		if status == "registered" {
			err := fmt.Errorf("signup phone already registered")
			data["error_message"] = err.Error()
			return data, err
		}
		if status != "available" {
			message := status
			if checkError != "" {
				message += ": " + checkError
			}
			err := fmt.Errorf("signup phone unavailable: %s", message)
			data["error_message"] = err.Error()
			return data, err
		}
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
