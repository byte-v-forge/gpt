package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) AcquireN8NGoPayPaymentSignupPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, failureCount int32) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	phone, err := s.activities.GoPayAppAcquireSignupPhoneActivity(ctx, pb.GoPayAppAcquireSignupPhoneInput{JobId: jobID, FailureCount: failureCount})
	data := structMap(phone.GetData())
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppSignupPhone, Success: err == nil, Phone: phone.GetPhone(), ActivationID: phone.GetActivationId(), StateJSON: phone.GetStateJson(), FailureCount: phone.GetFailureCount(), MaxFailures: firstNonZeroInt32(phone.GetMaxFailures(), goPayAppSignupMaxPhoneAttemptsAPI), OTPTimeoutSeconds: phone.GetOtpTimeoutSeconds(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupPhone, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) GenerateN8NGoPayPaymentSignupDeviceProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.goPayPaymentParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	proxy, err := s.activities.GoPayAppGenerateDeviceProxyActivity(ctx, pb.GoPayAppGenerateDeviceProxyInput{JobId: jobID, AccountId: accountID, CountryCode: params["country_code"]})
	data := structMap(proxy.GetData())
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppGenerateDeviceProxy, Success: err == nil, StateJSON: proxy.GetStateJson(), ProxyHash: proxy.GetProxyHash(), DeviceFingerprint: proxy.GetDeviceFingerprint(), Data: data}
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppGenerateDeviceProxy, jobstatus.FailedRetryable, false, true, err, data)
	}
	if strings.TrimSpace(proxy.GetStateJson()) == "" {
		err := fmt.Errorf("generated device proxy state_json missing")
		result.Success = false
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppGenerateDeviceProxy, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayPaymentSignupPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, activationID string, phone string, stateJSON string, proxyHash string, deviceFingerprint string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	params, err := s.goPayPaymentParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	check, err := s.activities.GoPayAppCheckSignupPhoneActivity(ctx, pb.GoPayAppCheckSignupPhoneInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), Phone: strings.TrimSpace(phone), CountryCode: params["country_code"], StateJson: firstNonEmpty(stateJSON, "{}")})
	data := structMap(check.GetData())
	expected := pb.GoPayAppGenerateDeviceProxyOutput{ProxyHash: strings.TrimSpace(proxyHash), DeviceFingerprint: strings.TrimSpace(deviceFingerprint)}
	matched := err == nil && goPayAppDeviceProxyMatchedAPI(expected, check)
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppCheckPhone, Success: err == nil && matched, Phone: firstNonEmpty(check.GetPhone(), phone), ActivationID: firstNonEmpty(check.GetActivationId(), activationID), StateJSON: firstNonEmpty(check.GetStateJson(), stateJSON), PhoneAccepted: err == nil && matched && check.GetAvailable(), DeviceProxyMatched: matched, ProxyHash: check.GetProxyHash(), DeviceFingerprint: check.GetDeviceFingerprint(), Data: data}
	if err != nil {
		if isGoPaySignupPhoneRotatableErrorAPI(err) {
			result.RetryableFailure = true
			result.RotatableFailure = true
			result.Success = true
			data["error_message"] = err.Error()
			return result, nil
		}
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppCheckPhone, jobstatus.FailedRetryable, false, true, err, data)
	}
	if !matched {
		err := fmt.Errorf("generated device proxy mismatch after check_phone")
		data["error_message"] = err.Error()
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppCheckPhone, jobstatus.FailedRetryable, false, true, err, data)
	}
	return result, nil
}

func (s *Server) StartN8NGoPayPaymentSignup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, phone string, stateJSON string) (any, error) {
	params, pin, err := s.goPayPaymentParamsWithPIN(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID))
	if err != nil {
		return nil, err
	}
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	userID = goPayAppUserID(firstNonEmpty(userID, params["user_id"]))
	start, err := s.activities.GoPayAppOTPStartActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "signup", StepName: stepGoPayAppSignup, Phone: strings.TrimSpace(phone), OtpChannel: "sms", SmsActivationId: strings.TrimSpace(activationID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin, CountryCode: params["country_code"]})
	result := goPayPaymentSignupOTPResult(jobID, accountID, n8nExecutionID, userID, activationID, phone, "sms", stepGoPayAppSignup, start, nil, err == nil)
	if err != nil {
		if isGoPaySignupPhoneRotatableErrorAPI(err) || isGoPaySignupOTPNotReceivedAPI(err) {
			return goPayPaymentSignupRecoverableResult(result, err), nil
		}
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if start.GetReady() || start.GetAccountTokenReady() || start.GetSignupComplete() {
		return result, nil
	}
	if !start.GetOtpRequired() {
		err := fmt.Errorf("gopay signup did not request OTP and did not complete")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayPaymentSignupOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, issuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	input := goPayPaymentSignupOTPWaitInput(jobID, goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix)
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayPaymentSignupOTPCheckResult(jobID, accountID, n8nExecutionID, goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayPaymentSignupOTPCheckResult(jobID, accountID, n8nExecutionID, goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayPaymentSignupOTPCheckResult(jobID, accountID, n8nExecutionID, goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) RetryN8NGoPayPaymentSignup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, stateJSON string, data map[string]any) (any, error) {
	params, pin, err := s.goPayPaymentParamsWithPIN(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID))
	if err != nil {
		return nil, err
	}
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	userID = goPayAppUserID(firstNonEmpty(userID, params["user_id"]))
	otpChannel = firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), "sms")
	retry, err := s.activities.GoPayAppOTPRetryActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "signup", StepName: stepGoPayAppSignupRetry, OtpChannel: otpChannel, SmsActivationId: strings.TrimSpace(activationID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin, CountryCode: params["country_code"]})
	result := goPayPaymentSignupOTPResult(jobID, accountID, n8nExecutionID, userID, activationID, "", otpChannel, stepGoPayAppSignupRetry, retry, data, err == nil)
	if err != nil {
		if isGoPaySignupPhoneRotatableErrorAPI(err) || isGoPaySignupOTPNotReceivedAPI(err) {
			return goPayPaymentSignupRecoverableResult(result, err), nil
		}
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupRetry, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if retry.GetReady() || retry.GetAccountTokenReady() || retry.GetSignupComplete() {
		return result, nil
	}
	if !retry.GetOtpRequired() {
		err := fmt.Errorf("gopay signup retry did not request OTP")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupRetry, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) RequestN8NGoPayPaymentSignupOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	otpChannel = firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), "sms")
	data := map[string]any{"otp_channel": otpChannel, "activation_id": strings.TrimSpace(activationID)}
	if otpChannel == "sms" && strings.TrimSpace(activationID) != "" {
		out, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), Reason: stepGoPayAppSignupRetry})
		data = structMap(out.GetData())
		if err != nil {
			result := goPayPaymentSignupActionResult(jobID, accountID, n8nExecutionID, goPayAppUserID(userID), activationID, otpChannel, stepGoPayAppSignupRetry, data, false)
			return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupRetry, jobstatus.FailedRetryable, false, true, err, data)
		}
	}
	return goPayPaymentSignupActionResult(jobID, accountID, n8nExecutionID, goPayAppUserID(userID), activationID, otpChannel, stepGoPayAppSignupRetry, data, true), nil
}

func (s *Server) CompleteN8NGoPayPaymentSignup(ctx context.Context, jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	params, pin, err := s.goPayPaymentParamsWithPIN(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID))
	if err != nil {
		return nil, err
	}
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	userID = goPayAppUserID(firstNonEmpty(userID, params["user_id"]))
	otpChannel = firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), "sms")
	if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required")
	}
	completed, err := s.activities.GoPayAppOTPCompleteActivity(ctx, pb.GoPayAppOTPCompleteInput{JobId: jobID, Operation: "signup", OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix, OtpSource: strings.TrimSpace(otpSource), Data: structData(data), OtpChannel: otpChannel, SmsActivationId: strings.TrimSpace(activationID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := goPayPaymentSignupOTPResult(jobID, accountID, n8nExecutionID, userID, activationID, "", otpChannel, stepGoPayAppSignup, completed, data, err == nil)
	if err != nil {
		if isGoPaySignupPhoneRotatableErrorAPI(err) || isGoPaySignupOTPNotReceivedAPI(err) {
			return goPayPaymentSignupRecoverableResult(result, err), nil
		}
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if completed.GetSignupComplete() || completed.GetReady() || completed.GetAccountTokenReady() {
		return result, nil
	}
	err = fmt.Errorf("gopay signup did not complete")
	return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
}

func (s *Server) DiscardN8NGoPayPaymentSignupPhone(ctx context.Context, jobID string, accountID string, n8nExecutionID string, activationID string, failureCount int32, reason string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NGoPayQRISIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.GoPayAppDiscardSignupPhoneActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), FailureCount: failureCount, Reason: firstNonEmpty(reason, "discard signup phone")})
	data := structMap(out.GetData())
	result := &n8nGoPayPaymentStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionGoPayPayment, Step: stepGoPayAppSignupPhoneCancel, Success: err == nil, ActivationID: firstNonEmpty(out.GetActivationId(), activationID), FailureCount: out.GetFailureCount(), Data: data}
	if err != nil {
		data["error_message"] = err.Error()
		return result, nil
	}
	return result, nil
}

func (s *Server) goPayPaymentParams(ctx context.Context, jobID string, n8nExecutionID string) (map[string]string, error) {
	if err := s.bindN8NGoPayExecution(ctx, strings.TrimSpace(jobID), strings.TrimSpace(n8nExecutionID)); err != nil {
		return nil, err
	}
	return s.jobStore.Params(ctx, strings.TrimSpace(jobID))
}

func (s *Server) goPayPaymentParamsWithPIN(ctx context.Context, jobID string, n8nExecutionID string) (map[string]string, string, error) {
	params, err := s.goPayPaymentParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, "", err
	}
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return nil, "", err
	}
	return params, pin, nil
}

func goPayPaymentSignupOTPResult(jobID string, accountID string, n8nExecutionID string, userID string, activationID string, phone string, otpChannel string, step string, output pb.GoPayAppOTPOutput, data map[string]any, success bool) *n8nGoPayPaymentStepResult {
	if data == nil {
		data = structMap(output.GetData())
	}
	channel := firstNonEmpty(normalizeGoPayOTPChannel(output.GetOtpChannel()), normalizeGoPayOTPChannel(otpChannel), "sms")
	return &n8nGoPayPaymentStepResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayPayment, Step: step, Success: success, Ready: output.GetReady(), UserID: goPayAppUserID(userID), Phone: firstNonEmpty(output.GetPhone(), phone), ActivationID: strings.TrimSpace(activationID), StateJSON: output.GetStateJson(), AccountTokenReady: output.GetAccountTokenReady(), SignupComplete: output.GetSignupComplete(), OTPRequired: output.GetOtpRequired(), OTPChannel: channel, OTPIssuedAfterUnix: output.GetIssuedAfterUnix(), OTPTimeoutSeconds: output.GetTimeoutSeconds(), Data: data}
}

func goPayPaymentSignupActionResult(jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, step string, data map[string]any, success bool) *n8nGoPayPaymentStepResult {
	return &n8nGoPayPaymentStepResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayPayment, Step: step, Success: success, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), "sms"), Data: data}
}

func goPayPaymentSignupRecoverableResult(result *n8nGoPayPaymentStepResult, err error) *n8nGoPayPaymentStepResult {
	result.Success = true
	result.RetryableFailure = true
	result.RotatableFailure = true
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	result.Data["error_message"] = err.Error()
	return result
}

func goPayPaymentSignupOTPWaitInput(jobID string, userID string, activationID string, otpChannel string, issuedAfterUnix int64) pb.OTPWaitInput {
	input := pb.OTPWaitInput{JobId: strings.TrimSpace(jobID), StepName: stepGoPayAppSignup, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam}
	if normalizeGoPayOTPChannel(otpChannel) != "wa" {
		input.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}
		return input
	}
	input.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: goPayAppUserID(userID)}}
	return input
}

func goPayPaymentSignupOTPCheckResult(jobID string, accountID string, n8nExecutionID string, userID string, activationID string, otpChannel string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayPaymentStepResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayPaymentStepResult{JobID: strings.TrimSpace(jobID), AccountID: strings.TrimSpace(accountID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayPayment, Step: stepGoPayAppSignup, Success: true, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), "sms"), OTPFound: found, OTPSource: strings.TrimSpace(source), OTPIssuedAfterUnix: issuedAfterUnix, Data: data}
}

func firstNonZeroInt32(values ...int32) int32 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
