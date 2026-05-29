package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/pb"
)

const protocolLoginMode = "login"

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
