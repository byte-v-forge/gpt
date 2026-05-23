package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/pb"
)

func (s *Server) BrowserAuthWaitActivity(ctx context.Context, input BrowserAuthWaitInput) (BrowserAuthWaitOutput, error) {
	output := BrowserAuthWaitOutput{
		AccountId:         input.GetAccountId(),
		FlowId:            input.GetFlowId(),
		Email:             input.GetEmail(),
		OtpTimeoutSeconds: s.registrationOtpTimeout(),
	}
	stepName, err := browserAuthBrowserStepName(input.GetMode())
	if err != nil {
		return output, err
	}
	if strings.TrimSpace(input.GetFlowId()) == "" {
		return output, fmt.Errorf("browser flow_id is required")
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}

	data := map[string]any{
		"account_id": input.GetAccountId(),
		"flow_id":    input.GetFlowId(),
		"mode":       input.GetMode(),
		"email":      input.GetEmail(),
	}
	var heartbeatAt time.Time
	lastStage := ""
	var recordedOTPRequestAt int64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		status, err := s.browserAuthStatus(ctx, input.GetFlowId())
		data["browser_status"] = browserStatusData(status)
		if err != nil {
			output.Data = protoData(data)
			return output, step.complete(data, err)
		}
		if status == nil || !status.GetFound() {
			err := fmt.Errorf("browser %s flow not found", input.GetMode())
			output.Data = protoData(data)
			return output, step.complete(data, err)
		}

		stage := strings.TrimSpace(status.GetStage())
		if stage == "" {
			stage = "running"
		}
		message := strings.TrimSpace(status.GetStatusMessage())
		if message == "" {
			message = "browser auth running"
		}
		progressFields := map[string]any{
			"mode":                               input.GetMode(),
			"flow_id":                            input.GetFlowId(),
			"browser_stage":                      stage,
			"browser_message":                    message,
			"otp_required":                       status.GetOtpRequired(),
			"done":                               status.GetDone(),
			"updated_at_unix":                    status.GetUpdatedAtUnix(),
			"otp_issued_after_unix":              status.GetOtpIssuedAfterUnix(),
			"otp_wait_started_at_unix":           status.GetOtpWaitStartedAtUnix(),
			"otp_request_action_started_at_unix": status.GetOtpRequestActionStartedAtUnix(),
		}
		if stage != lastStage {
			step.progress(message, progressFields)
			lastStage = stage
		} else {
			step.progressEvery(&heartbeatAt, message, progressFields)
		}
		if requestActionStartedAt := status.GetOtpRequestActionStartedAtUnix(); requestActionStartedAt > 0 && requestActionStartedAt != recordedOTPRequestAt {
			if err := s.recordBrowserAuthOTPRequestStep(ctx, input, status, stage, message); err != nil {
				output.Data = protoData(data)
				return output, step.complete(data, err)
			}
			recordedOTPRequestAt = requestActionStartedAt
		}

		if status.GetDone() {
			result := status.GetResult()
			data["browser_complete"] = registerResultData(result)
			output.Data = protoData(data)
			if result == nil {
				err := fmt.Errorf("browser %s completed without result", input.GetMode())
				return output, step.complete(data, err)
			}
			if !result.GetSuccess() {
				err := fmt.Errorf("browser %s failed: %s", input.GetMode(), result.GetErrorMessage())
				return output, step.complete(data, err)
			}
			resultOutput := registerActivityOutputFromResponse(result, data)
			output.Result = &resultOutput
			_, _ = s.browserAuthCancel(ctx, input.GetMode(), input.GetFlowId())
			return output, step.complete(data, nil)
		}

		if status.GetOtpRequired() {
			requestActionStartedAt := status.GetOtpRequestActionStartedAtUnix()
			if requestActionStartedAt <= 0 {
				err := fmt.Errorf("browser %s OTP request action start time missing", input.GetMode())
				output.Data = protoData(data)
				return output, step.complete(data, err)
			}
			output.OtpRequired = true
			output.OtpIssuedAfterUnix = requestActionStartedAt
			output.OtpWaitStartedAtUnix = status.GetOtpWaitStartedAtUnix()
			output.OtpRequestActionStartedAtUnix = requestActionStartedAt
			output.Data = protoData(data)
			return output, step.complete(data, nil)
		}

		select {
		case <-ctx.Done():
			output.Data = protoData(data)
			return output, step.complete(data, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *Server) recordBrowserAuthOTPRequestStep(ctx context.Context, input BrowserAuthWaitInput, status *pb.BrowserFlowStatusResponse, stage string, message string) error {
	stepName, err := browserAuthOTPRequestStepName(input.GetMode())
	if err != nil {
		return err
	}
	data := map[string]any{
		"account_id":                         input.GetAccountId(),
		"flow_id":                            input.GetFlowId(),
		"mode":                               input.GetMode(),
		"email":                              input.GetEmail(),
		"browser_stage":                      stage,
		"browser_message":                    message,
		"otp_issued_after_unix":              status.GetOtpIssuedAfterUnix(),
		"otp_wait_started_at_unix":           status.GetOtpWaitStartedAtUnix(),
		"otp_request_action_started_at_unix": status.GetOtpRequestActionStartedAtUnix(),
		"browser_updated_at_unix":            status.GetUpdatedAtUnix(),
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return err
	}
	step.progress("OTP request action clicked", data)
	return step.complete(data, nil)
}
