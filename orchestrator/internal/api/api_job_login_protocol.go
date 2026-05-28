package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const protocolLoginMode = "login"

func (s *Server) runLoginSessionProtocolAction(ctx context.Context, jobID string, accountID string) error {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: strings.TrimSpace(accountID)})
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, nil)
	}
	accountID = account.GetAccountId()
	combined := map[string]any{"account_id": accountID, "driver": "protocol", "mode": protocolLoginMode}

	proxy, err := s.activities.ProtocolUseProxyActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, protocolLoginMode))
	mergeActionData(combined, stepProtocolUseProxy, structMap(proxy.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepProtocolUseProxy, jobstatus.FailedRetryable, false, true, err, combined)
	}

	start, err := s.activities.ProtocolAuthStartActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, protocolLoginMode))
	mergeActionData(combined, stepLoginSessionProtocolStart, structMap(start.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepLoginSessionProtocolStart, jobstatus.FailedRetryable, false, true, err, combined)
	}
	login := start.GetResult()
	flowID := start.GetFlowId()
	email := start.GetEmail()

	var wait *pb.ProtocolAuthWaitOutput
	if login == nil {
		out, err := s.activities.ProtocolAuthWaitActivity(ctx, pb.ProtocolAuthWaitInput{JobId: jobID, AccountId: accountID, FlowId: flowID, Mode: protocolLoginMode, Email: email})
		wait = &out
		mergeActionData(combined, stepLoginSessionProtocol, structMap(out.GetData()))
		if err != nil {
			_ = s.activities.ProtocolAuthCancelActivity(ctx, pb.ProtocolAuthCancelInput{JobId: jobID, FlowId: flowID, Mode: protocolLoginMode})
			return s.markActionFailed(ctx, jobID, stepLoginSessionProtocol, jobstatus.FailedRetryable, false, true, err, combined)
		}
		if out.GetResult() != nil {
			login = out.GetResult()
		}
		if out.GetEmail() != "" {
			email = out.GetEmail()
		}
	}

	if wait != nil && wait.GetOtpRequired() {
		otp, err := s.waitLoginSessionProtocolEmailOTP(ctx, jobID, email, wait.GetOtpTimeoutSeconds(), wait.GetOtpIssuedAfterUnix())
		mergeActionData(combined, stepLoginSessionProtocolOTPWait, otpWaitData(email, wait.GetOtpTimeoutSeconds(), wait.GetOtpIssuedAfterUnix(), otp))
		if err != nil {
			_ = s.activities.ProtocolAuthCancelActivity(ctx, pb.ProtocolAuthCancelInput{JobId: jobID, FlowId: flowID, Mode: protocolLoginMode})
			return s.markActionFailed(ctx, jobID, stepLoginSessionProtocolOTPWait, jobstatus.FailedRetryable, false, true, err, combined)
		}
		completed, err := s.activities.ProtocolAuthCompleteActivity(ctx, pb.ProtocolAuthCompleteInput{
			JobId:              jobID,
			AccountId:          accountID,
			FlowId:             flowID,
			Mode:               protocolLoginMode,
			OtpParam:           registrationOTPParam,
			SubmittedAtParam:   registrationOTPSubmittedAtParam,
			OtpIssuedAfterUnix: wait.GetOtpIssuedAfterUnix(),
			OtpSource:          otp.GetSource(),
		})
		mergeActionData(combined, stepLoginSessionProtocolComplete, structMap(completed.GetData()))
		if err != nil {
			_ = s.activities.ProtocolAuthCancelActivity(ctx, pb.ProtocolAuthCancelInput{JobId: jobID, FlowId: flowID, Mode: protocolLoginMode})
			return s.markActionFailed(ctx, jobID, stepLoginSessionProtocolComplete, jobstatus.FailedRetryable, false, true, err, combined)
		}
		login = &completed
	}

	if login == nil || (strings.TrimSpace(login.GetSessionToken()) == "" && strings.TrimSpace(login.GetAccessToken()) == "") {
		err := fmt.Errorf("protocol login did not return ChatGPT credentials")
		return s.markActionFailed(ctx, jobID, stepLoginSessionProtocolComplete, jobstatus.FailedRetryable, false, true, err, combined)
	}
	mergeActionData(combined, "login_session", structMap(login.GetData()))
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{AccountId: accountID, SessionToken: login.GetSessionToken(), AccessToken: login.GetAccessToken()}); err != nil {
		return s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, combined)
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) waitLoginSessionProtocolEmailOTP(ctx context.Context, jobID string, email string, timeoutSeconds int32, issuedAfterUnix int64) (pb.OTPWaitOutput, error) {
	return s.waitProtocolEmailOTP(ctx, jobID, stepLoginSessionProtocolOTPWait, email, timeoutSeconds, issuedAfterUnix)
}

func (s *Server) waitProtocolEmailOTP(ctx context.Context, jobID string, stepName string, email string, timeoutSeconds int32, issuedAfterUnix int64) (pb.OTPWaitOutput, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	baseInput := pb.OTPWaitInput{
		JobId:            jobID,
		StepName:         stepName,
		Target:           &pb.OTPWaitInput_Email{Email: &pb.OTPWaitEmailTarget{Email: email}},
		TimeoutSeconds:   timeoutSeconds,
		IssuedAfterUnix:  issuedAfterUnix,
		OtpParam:         registrationOTPParam,
		SubmittedAtParam: registrationOTPSubmittedAtParam,
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: jobID, StepName: stepName, Recoverable: false, Retryable: true, Detail: structData(otpWaitStartData(email, timeoutSeconds, issuedAfterUnix))}); err != nil {
		return pb.OTPWaitOutput{}, err
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	lastErr := ""
	for {
		manual, err := s.activities.FetchManualOTPActivity(ctx, baseInput)
		if err != nil {
			lastErr = err.Error()
		} else if manual.GetFound() {
			return manual, s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{JobId: jobID, StepName: stepName, Recoverable: false, Retryable: true, Result: structData(otpWaitData(email, timeoutSeconds, issuedAfterUnix, manual))})
		}

		remaining := int32(time.Until(deadline).Seconds())
		if remaining <= 0 {
			break
		}
		input := baseInput
		input.TimeoutSeconds = minOTPWaitChunkSeconds(remaining)
		out, err := s.activities.OTPWaitActivity(ctx, input)
		if err != nil {
			lastErr = err.Error()
		} else if out.GetFound() {
			return out, s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{JobId: jobID, StepName: stepName, Recoverable: false, Retryable: true, Result: structData(otpWaitData(email, timeoutSeconds, issuedAfterUnix, out))})
		}
		select {
		case <-ctx.Done():
			return pb.OTPWaitOutput{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if lastErr != "" {
		return pb.OTPWaitOutput{ErrorMessage: lastErr}, fmt.Errorf("otp not received after %ds: %s", timeoutSeconds, lastErr)
	}
	return pb.OTPWaitOutput{}, fmt.Errorf("otp not received after %ds", timeoutSeconds)
}

func minOTPWaitChunkSeconds(remaining int32) int32 {
	if remaining < 15 {
		return remaining
	}
	return 15
}

func mergeActionData(dst map[string]any, key string, value map[string]any) {
	if len(value) == 0 {
		return
	}
	dst[key] = value
	for nestedKey, nestedValue := range value {
		if _, exists := dst[nestedKey]; !exists {
			dst[nestedKey] = nestedValue
		}
	}
}

func otpWaitStartData(email string, timeoutSeconds int32, issuedAfterUnix int64) map[string]any {
	return map[string]any{
		"channel":           "email",
		"email":             strings.TrimSpace(email),
		"timeout_seconds":   timeoutSeconds,
		"issued_after_unix": issuedAfterUnix,
	}
}

func otpWaitData(email string, timeoutSeconds int32, issuedAfterUnix int64, output pb.OTPWaitOutput) map[string]any {
	data := otpWaitStartData(email, timeoutSeconds, issuedAfterUnix)
	for key, value := range structMap(output.GetData()) {
		data[key] = value
	}
	data["found"] = output.GetFound()
	if output.GetSource() != "" {
		data["source"] = output.GetSource()
	}
	if output.GetActivationId() != "" {
		data["activation_id"] = output.GetActivationId()
	}
	if output.GetErrorMessage() != "" {
		data["error_message"] = output.GetErrorMessage()
	}
	return data
}
