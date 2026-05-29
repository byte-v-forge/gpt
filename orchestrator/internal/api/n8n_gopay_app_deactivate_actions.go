package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) StartN8NGoPayAppDeactivate(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, stateJSON string) (any, error) {
	_, pin, err := s.goPayAppStepParams(ctx, jobID, n8nExecutionID)
	if err != nil {
		return nil, err
	}
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	activationID = strings.TrimSpace(activationID)
	issuedAfterUnix := time.Now().Unix()
	start, err := s.activities.GoPayAppDeactivateStartActivity(ctx, pb.GoPayAppDeactivateStartInput{JobId: jobID, ActivationId: activationID, StateJson: firstNonEmpty(stateJSON, "{}"), Pin: pin})
	result := goPayAppDeactivateStartResult(jobID, n8nExecutionID, operation, userID, activationID, issuedAfterUnix, start, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppDeactivateStart, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if !start.GetOtpRequired() {
		err := fmt.Errorf("gopay deactivate did not request OTP")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppDeactivateStart, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) CheckN8NGoPayAppDeactivateOTP(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, issuedAfterUnix int64) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	input := goPayAppDeactivateOTPWaitInput(jobID, activationID, issuedAfterUnix)
	manual, err := s.activities.FetchManualOTPActivity(ctx, input)
	if err != nil {
		return nil, err
	}
	if manual.GetFound() {
		return goPayAppDeactivateOTPCheckResult(jobID, n8nExecutionID, operation, userID, activationID, issuedAfterUnix, manual.GetSource(), true, structMap(manual.GetData())), nil
	}
	wait, err := s.activities.OTPWaitActivity(ctx, input)
	if err != nil {
		if isOTPWaitNotReceivedError(err) {
			return goPayAppDeactivateOTPCheckResult(jobID, n8nExecutionID, operation, userID, activationID, issuedAfterUnix, "", false, map[string]any{"error_message": err.Error()}), nil
		}
		return nil, err
	}
	return goPayAppDeactivateOTPCheckResult(jobID, n8nExecutionID, operation, userID, activationID, issuedAfterUnix, wait.GetSource(), wait.GetFound(), structMap(wait.GetData())), nil
}

func (s *Server) CompleteN8NGoPayAppDeactivate(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, stateJSON string, issuedAfterUnix int64, otpSource string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	activationID = strings.TrimSpace(activationID)
	if strings.TrimSpace(otpSource) == "" {
		return nil, fmt.Errorf("otp_source is required")
	}
	complete, err := s.activities.GoPayAppDeactivateCompleteActivity(ctx, pb.GoPayAppDeactivateCompleteInput{JobId: jobID, ActivationId: activationID, StateJson: firstNonEmpty(stateJSON, "{}"), OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix})
	result := goPayAppDeactivateCompleteResult(jobID, n8nExecutionID, operation, userID, activationID, complete, err == nil)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppDeactivateComplete, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	if !complete.GetDeactivateComplete() {
		err := fmt.Errorf("gopay deactivate did not complete")
		return result, s.markActionFailed(ctx, jobID, stepGoPayAppDeactivateComplete, jobstatus.FailedRetryable, false, true, err, result.Data)
	}
	return result, nil
}

func (s *Server) FinishN8NGoPayAppSMS(ctx context.Context, jobID string, n8nExecutionID string, operation string, userID string, activationID string, reason string) (any, error) {
	jobID = strings.TrimSpace(jobID)
	n8nExecutionID = strings.TrimSpace(n8nExecutionID)
	if err := s.bindN8NGoPayExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	operation = normalizeGoPayAppOperation(operation)
	userID = goPayAppUserID(userID)
	activationID = strings.TrimSpace(activationID)
	out, err := s.activities.GoPayAppSMSFinishActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: activationID, Reason: firstNonEmpty(reason, "gopay app sms cleanup")})
	data := structMap(out.GetData())
	if err != nil {
		data["error_message"] = err.Error()
	}
	return &n8nGoPayAppResult{JobID: jobID, N8NExecutionID: n8nExecutionID, Action: actionGoPayApp, Step: stepGoPayAppSMSFinish, Operation: operation, Success: err == nil, UserID: userID, ActivationID: activationID, Data: data}, nil
}

func goPayAppDeactivateStartResult(jobID string, n8nExecutionID string, operation string, userID string, activationID string, issuedAfterUnix int64, output pb.GoPayAppDeactivateStartOutput, success bool) *n8nGoPayAppResult {
	data := structMap(output.GetData())
	data["otp_issued_after_unix"] = issuedAfterUnix
	return &n8nGoPayAppResult{JobID: strings.TrimSpace(jobID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayApp, Step: stepGoPayAppDeactivateStart, Operation: normalizeGoPayAppOperation(operation), Success: success, UserID: goPayAppUserID(userID), ActivationID: firstNonEmpty(output.GetActivationId(), activationID), StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), OTPRequired: output.GetOtpRequired(), OTPIssuedAfterUnix: issuedAfterUnix, OTPTimeoutSeconds: output.GetTimeoutSeconds(), Data: data}
}

func goPayAppDeactivateCompleteResult(jobID string, n8nExecutionID string, operation string, userID string, activationID string, output pb.GoPayAppDeactivateCompleteOutput, success bool) *n8nGoPayAppResult {
	return &n8nGoPayAppResult{JobID: strings.TrimSpace(jobID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayApp, Step: stepGoPayAppDeactivateComplete, Operation: normalizeGoPayAppOperation(operation), Success: success, UserID: goPayAppUserID(userID), ActivationID: firstNonEmpty(output.GetActivationId(), activationID), StateJSON: firstNonEmpty(output.GetStateJson(), "{}"), DeactivateComplete: output.GetDeactivateComplete(), Data: structMap(output.GetData())}
}

func goPayAppDeactivateOTPWaitInput(jobID string, activationID string, issuedAfterUnix int64) pb.OTPWaitInput {
	return pb.OTPWaitInput{JobId: strings.TrimSpace(jobID), StepName: stepGoPayAppDeactivateSMSWait, TimeoutSeconds: 1, IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, Target: &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: strings.TrimSpace(activationID)}}}
}

func goPayAppDeactivateOTPCheckResult(jobID string, n8nExecutionID string, operation string, userID string, activationID string, issuedAfterUnix int64, source string, found bool, data map[string]any) *n8nGoPayAppResult {
	if data == nil {
		data = map[string]any{}
	}
	data["otp_found"] = found
	data["otp_issued_after_unix"] = issuedAfterUnix
	if strings.TrimSpace(source) != "" {
		data["otp_source"] = strings.TrimSpace(source)
	}
	return &n8nGoPayAppResult{JobID: strings.TrimSpace(jobID), N8NExecutionID: strings.TrimSpace(n8nExecutionID), Action: actionGoPayApp, Step: stepGoPayAppDeactivateSMSWait, Operation: normalizeGoPayAppOperation(operation), Success: true, UserID: goPayAppUserID(userID), ActivationID: strings.TrimSpace(activationID), OTPIssuedAfterUnix: issuedAfterUnix, OTPFound: found, OTPSource: strings.TrimSpace(source), Data: data}
}
