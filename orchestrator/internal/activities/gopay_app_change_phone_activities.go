package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) GoPayAppChangePhoneGetNumberActivity(ctx context.Context, input GoPayAppChangePhoneGetNumberInput) (GoPayAppChangePhoneGetNumberOutput, error) {
	output := GoPayAppChangePhoneGetNumberOutput{}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppChangePhoneGetNumber, false, true)
	_, err := step.run(func() (any, error) {
		if s.gopayClient == nil || s.smsClient == nil {
			err := fmt.Errorf("gopay app or code receiver client not configured")
			data["error_message"] = err.Error()
			return data, err
		}

		maxFailures := s.changePhoneMaxFailureCount()
		failures := int(input.GetFailureCount())
		if failures < 0 {
			failures = 0
		}
		output.FailureCount = int32(failures)
		output.MaxFailures = int32(maxFailures)
		data["failure_count"] = failures
		data["max_failures"] = maxFailures

		for failures < maxFailures {
			step.progress("acquiring phone number", map[string]any{
				"failures":     failures,
				"max_failures": maxFailures,
			})
			activationID, phone, err := s.acquireSMSNumber(ctx, goPaySMSQuery(), input.GetJobId(), map[string]string{
				"workflow": "gopay_change_phone",
				"job_id":   input.GetJobId(),
			})
			if err != nil {
				message := err.Error()
				if smsNoNumbers(message) {
					data["last_get_number_error"] = message
					data["failure_count"] = failures
					step.progress("no SMS numbers available; retrying GetNumber", map[string]any{
						"failures":     failures,
						"max_failures": maxFailures,
					})
					delay := s.changePhoneGetNumberRetryInterval()
					if delay <= 0 {
						delay = defaultChangePhoneGetNumberRetryDelay
					}
					if err := sleepContext(ctx, delay); err != nil {
						output.FailureCount = int32(failures)
						err = fmt.Errorf("waiting to retry GetNumber after NO_NUMBERS: %w", err)
						data["error_message"] = err.Error()
						return data, err
					}
					continue
				}
				err = fmt.Errorf("GetNumber: %w", err)
				data["error_message"] = err.Error()
				return data, err
			}

			phone = normalizeIndonesiaPhone(phone)
			data["activation_id"] = activationID
			data["phone_present"] = phone != ""
			step.progress("phone number acquired", map[string]any{
				"activation_id": activationID,
				"phone_present": phone != "",
			})
			if phone == "" {
				if err := s.recordChangePhoneFailure(ctx, activationID, &failures, "empty phone from SMS service"); err != nil {
					output.FailureCount = int32(failures)
					data["failure_count"] = failures
					data["error_message"] = err.Error()
					return data, err
				}
				output.FailureCount = int32(failures)
				data["failure_count"] = failures
				continue
			}

			deviceProxy, err := s.gopayClient.GenerateDeviceProxy(ctx, &pb.GenerateDeviceProxyRequest{})
			if err != nil {
				if cancelErr := s.recordChangePhoneFailure(ctx, activationID, &failures, fmt.Sprintf("GenerateDeviceProxy: %v", err)); cancelErr != nil {
					output.FailureCount = int32(failures)
					data["failure_count"] = failures
					data["error_message"] = cancelErr.Error()
					return data, cancelErr
				}
				output.FailureCount = int32(failures)
				data["failure_count"] = failures
				continue
			}
			if deviceProxy == nil || !deviceProxy.GetSuccess() || strings.TrimSpace(deviceProxy.GetStateJson()) == "" {
				reason := "GenerateDeviceProxy failed"
				if deviceProxy != nil && deviceProxy.GetErrorMessage() != "" {
					reason = fmt.Sprintf("%s: %s", reason, deviceProxy.GetErrorMessage())
				}
				if cancelErr := s.recordChangePhoneFailure(ctx, activationID, &failures, reason); cancelErr != nil {
					output.FailureCount = int32(failures)
					data["failure_count"] = failures
					data["error_message"] = cancelErr.Error()
					return data, cancelErr
				}
				output.FailureCount = int32(failures)
				data["failure_count"] = failures
				continue
			}
			data["check_phone_device_proxy"] = map[string]any{
				"proxy_slot":          deviceProxy.GetProxySlot(),
				"dynamic_egress_size": deviceProxy.GetDynamicEgressSize(),
				"proxy_hash":          deviceProxy.GetProxyHash(),
				"device_fingerprint":  deviceProxy.GetDeviceFingerprint(),
			}

			checkResp, err := s.gopayClient.CheckPhone(ctx, &pb.CheckPhoneRequest{
				Phone:     phone,
				StateJson: deviceProxy.GetStateJson(),
			})
			if err != nil {
				if cancelErr := s.recordChangePhoneFailure(ctx, activationID, &failures, fmt.Sprintf("CheckPhone: %v", err)); cancelErr != nil {
					output.FailureCount = int32(failures)
					data["failure_count"] = failures
					data["error_message"] = cancelErr.Error()
					return data, cancelErr
				}
				output.FailureCount = int32(failures)
				data["failure_count"] = failures
				continue
			}
			status := checkPhoneStatus(checkResp)
			data["phone_status"] = status
			if checkResp != nil {
				data["check_phone_proxy_hash"] = checkResp.GetProxyHash()
				data["check_phone_device_fingerprint"] = checkResp.GetDeviceFingerprint()
			}
			step.progress("phone availability checked", map[string]any{
				"activation_id": activationID,
				"status":        status,
			})
			if status != "available" {
				reason := fmt.Sprintf("CheckPhone status=%s", status)
				if checkResp != nil && checkResp.GetErrorMessage() != "" {
					reason = fmt.Sprintf("%s: %s", reason, checkResp.GetErrorMessage())
				}
				if err := s.recordChangePhoneFailure(ctx, activationID, &failures, reason); err != nil {
					output.FailureCount = int32(failures)
					data["failure_count"] = failures
					data["error_message"] = err.Error()
					return data, err
				}
				output.FailureCount = int32(failures)
				data["failure_count"] = failures
				continue
			}

			output.ActivationId = activationID
			output.Phone = phone
			output.FailureCount = int32(failures)
			output.MaxFailures = int32(maxFailures)
			data["failure_count"] = failures
			data["change_phone_number_acquired"] = true
			return data, nil
		}

		err := fmt.Errorf("failed to change phone after %d consecutive failures", maxFailures)
		output.FailureCount = int32(failures)
		data["failure_count"] = failures
		data["error_message"] = err.Error()
		return data, err
	})
	output.Data = protoData(data)
	return output, err
}
