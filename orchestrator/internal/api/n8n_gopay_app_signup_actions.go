package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) StartN8NGoPayAppSignup(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, phone string, otpChannel string, activationID string, stateJSON string) (any, error) {
	params, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	channel := firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), normalizeGoPayOTPChannel(params["otp_channel"]))
	start, err := s.activities.GoPayAppOTPStartActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "signup", StepName: stepGoPayAppSignup, Phone: firstNonEmpty(phone, params["phone"]), OtpChannel: channel, SmsActivationId: strings.TrimSpace(activationID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin, CountryCode: params["country_code"]})
	result := goPayAppSignupResult(jobID, n8nExecutionID, operation, userID, activationID, phone, channel, stepGoPayAppSignup, start, nil, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if start.GetReady() || start.GetAccountTokenReady() || start.GetSignupComplete() {
		if saveErr := s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, result.StateJSON); saveErr != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, result.Data)
		}
		return result, nil
	}
	if !start.GetOtpRequired() {
		err := fmt.Errorf("gopay signup did not request OTP and did not complete")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayAppSignupOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, issuedAfterUnix int64) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	input := goPayAppSignupOTPWaitInput(jobID, goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix)
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayAppSignupOTPCheckResult(jobID, n8nExecutionID, normalizeGoPayAppOperation(operation), goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayAppSignupOTPCheckResult(jobID, n8nExecutionID, normalizeGoPayAppOperation(operation), goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayAppSignupOTPCheckResult(jobID, n8nExecutionID, normalizeGoPayAppOperation(operation), goPayAppUserID(userID), activationID, otpChannel, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) RetryN8NGoPayAppSignup(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, stateJSON string, data map[string]any) (any, error) {
	params, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	channel := firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), normalizeGoPayOTPChannel(params["otp_channel"]))
	retry, err := s.activities.GoPayAppOTPRetryActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "signup", StepName: stepGoPayAppSignupRetry, OtpChannel: channel, SmsActivationId: strings.TrimSpace(activationID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin, CountryCode: params["country_code"]})
	result := goPayAppSignupResult(jobID, n8nExecutionID, operation, userID, activationID, "", channel, stepGoPayAppSignupRetry, retry, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupRetry, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if retry.GetReady() || retry.GetAccountTokenReady() || retry.GetSignupComplete() {
		if saveErr := s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, result.StateJSON); saveErr != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, result.Data)
		}
		return result, nil
	}
	if !retry.GetOtpRequired() {
		err := fmt.Errorf("gopay signup retry did not request OTP")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupRetry, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) RequestN8NGoPayAppSignupOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	channel := normalizeGoPayOTPChannel(otpChannel)
	data := map[string]any{"otp_channel": channel, "activation_id": strings.TrimSpace(activationID)}
	if channel == "sms" && strings.TrimSpace(activationID) != "" {
		out, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: strings.TrimSpace(activationID), Reason: stepGoPayAppSignupRetry})
		data = structMap(out.GetData())
		if err != nil {
			result := goPayAppSignupActionResult(jobID, n8nExecutionID, normalizeGoPayAppOperation(operation), goPayAppUserID(userID), activationID, channel, stepGoPayAppSignupRetry, data, false)
			return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignupRetry, jobstatus.FailedRetryable, false, true, err, data)
		}
	}
	return goPayAppSignupActionResult(jobID, n8nExecutionID, normalizeGoPayAppOperation(operation), goPayAppUserID(userID), activationID, channel, stepGoPayAppSignupRetry, data, true), nil
}

func (s *Server) CompleteN8NGoPayAppSignup(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, stateJSON string, issuedAfterUnix int64, otpSource string, data map[string]any) (any, error) {
	params, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	channel := firstNonEmpty(normalizeGoPayOTPChannel(otpChannel), normalizeGoPayOTPChannel(params["otp_channel"]))
	if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required")
	}
	completed, err := s.activities.GoPayAppOTPCompleteActivity(ctx, pb.GoPayAppOTPCompleteInput{JobId: jobID, Operation: "signup", OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix, OtpSource: strings.TrimSpace(otpSource), Data: structData(data), OtpChannel: channel, SmsActivationId: strings.TrimSpace(activationID), StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := goPayAppSignupResult(jobID, n8nExecutionID, operation, userID, activationID, "", channel, stepGoPayAppSignup, completed, data, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if completed.GetSignupComplete() || completed.GetReady() || completed.GetAccountTokenReady() {
		if saveErr := s.saveGoPayAppStateForUserOperation(ctx, jobID, userID, operation, result.StateJSON); saveErr != nil {
			return result, s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, saveErr, result.Data)
		}
		return result, nil
	}
	err = fmt.Errorf("gopay signup did not complete")
	return result, s.markActionFailed(ctx, jobID, stepGoPayAppSignup, jobstatus.FailedRetryable, false, true, err, result.Data)
}

func goPayAppSignupResult(jobID string, n8nExecutionID string, operation string, userID string, activationID string, phone string, otpChannel string, step string, output pb.GoPayAppOTPOutput, data map[string]any, success bool) *n8nGoPayAppResult {
	if data == nil {
		data = structMap(output.GetData())
	}
	channel := firstNonEmpty(normalizeGoPayOTPChannel(output.GetOtpChannel()), normalizeGoPayOTPChannel(otpChannel))
	return &n8nGoPayAppResult{JobID: strings.TrimSpace(jobID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayApp, Step: step, Operation: normalizeGoPayAppOperation(operation), Success: success, UserID: goPayAppUserID(userID), Phone: firstNonEmpty(output.GetPhone(), phone), ActivationID: strings.TrimSpace(activationID), OTPChannel: channel, OTPRequired: output.GetOtpRequired(), OTPIssuedAfterUnix: output.GetIssuedAfterUnix(), OTPTimeoutSeconds: output.GetTimeoutSeconds(), StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), Ready: output.GetReady(), AccountTokenReady: output.GetAccountTokenReady(), SignupComplete: output.GetSignupComplete(), Data: data}
}

func goPayAppSignupActionResult(jobID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, step string, data map[string]any, success bool) *n8nGoPayAppResult {
	return &n8nGoPayAppResult{JobID: strings.TrimSpace(jobID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayApp, Step: step, Operation: normalizeGoPayAppOperation(operation), Success: success, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: normalizeGoPayOTPChannel(otpChannel), Data: data}
}

func goPayAppSignupOTPWaitInput(jobID string, userID string, activationID string, otpChannel string, issuedAfterUnix int64) pb.OTPWaitInput {
	input := pb.OTPWaitInput{JobId: strings.TrimSpace(jobID), StepName: stepGoPayAppSignup, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam}
	if normalizeGoPayOTPChannel(otpChannel) == "sms" {
		input.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}
		return input
	}
	input.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: goPayAppUserID(userID)}}
	return input
}

func goPayAppSignupOTPCheckResult(jobID string, n8nExecutionID string, operation string, userID string, activationID string, otpChannel string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayAppResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayAppResult{JobID: strings.TrimSpace(jobID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayApp, Step: stepGoPayAppSignup, Operation: normalizeGoPayAppOperation(operation), Success: true, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPChannel: normalizeGoPayOTPChannel(otpChannel), OTPFound: found, OTPSource: strings.TrimSpace(source), OTPIssuedAfterUnix: issuedAfterUnix, Data: data}
}
