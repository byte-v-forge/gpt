package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/manualinput"
	"orchestrator/pb"
)

func (s *Server) BrowserAuthResendOTPActivity(ctx context.Context, input BrowserAuthResendOTPInput) (BrowserAuthResendOTPOutput, error) {
	output := BrowserAuthResendOTPOutput{
		FlowId:            input.GetFlowId(),
		OtpTimeoutSeconds: s.registrationOtpTimeout(),
	}
	stepName, err := browserAuthOTPRequestStepName(input.GetMode())
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
	}
	step.progress("resending email OTP", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "resending email OTP", data)
	defer stopHeartbeat()

	resp, err := s.browserAuthResendOTP(ctx, input.GetMode(), input.GetFlowId())
	data["browser_resend"] = browserAuthResendData(resp)
	if err != nil {
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if resp == nil {
		err := fmt.Errorf("browser %s OTP resend returned empty response", input.GetMode())
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if !resp.GetSuccess() {
		err := fmt.Errorf("browser %s OTP resend failed: %s", input.GetMode(), resp.GetErrorMessage())
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}

	output.FlowId = resp.GetFlowId()
	output.Email = resp.GetEmail()
	output.Success = true
	output.OtpIssuedAfterUnix = resp.GetOtpIssuedAfterUnix()
	output.OtpRequestStartedAtUnixMs = resp.GetOtpRequestStartedAtUnixMs()
	output.OtpTimeoutSeconds = s.registrationOtpTimeout()
	data["otp_issued_after_unix"] = output.GetOtpIssuedAfterUnix()
	data["otp_request_started_at_unix_ms"] = output.GetOtpRequestStartedAtUnixMs()
	output.Data = protoData(data)
	return output, step.complete(data, nil)
}

func (s *Server) FetchManualOTPActivity(ctx context.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	if strings.TrimSpace(input.GetOtpParam()) == "" {
		return OTPWaitOutput{}, nil
	}
	value, found, err := s.getJobParam(ctx, input.GetJobId(), input.GetOtpParam())
	if err != nil || !found {
		return OTPWaitOutput{}, err
	}
	if !manualinput.SubmittedAfter(ctx, s.jobStore, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetIssuedAfterUnix()) {
		return OTPWaitOutput{}, nil
	}
	code := normalizeOTP(value)
	return OTPWaitOutput{Found: code != "", Source: "manual", Code: code}, nil
}

func (s *Server) consumeStoredOTP(ctx context.Context, jobID, otpParam, submittedAtParam string, issuedAfterUnix int64) (string, error) {
	value, found, err := s.getJobParam(ctx, jobID, otpParam)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("otp not found")
	}
	if !manualinput.SubmittedAfter(ctx, s.jobStore, jobID, otpParam, submittedAtParam, issuedAfterUnix) {
		return "", fmt.Errorf("otp is stale")
	}
	code := normalizeOTP(value)
	if code == "" {
		_ = s.deleteJobParam(ctx, jobID, otpParam)
		_ = s.deleteJobParam(ctx, jobID, submittedAtParam)
		return "", fmt.Errorf("otp is empty")
	}
	if err := s.deleteJobParam(ctx, jobID, otpParam); err != nil {
		return "", err
	}
	_ = s.deleteJobParam(ctx, jobID, submittedAtParam)
	return code, nil
}

func browserAuthResendData(resp *pb.BrowserAuthResendOTPOutput) map[string]any {
	if resp == nil {
		return nil
	}
	data := map[string]any{
		"flow_id":                        resp.GetFlowId(),
		"email":                          resp.GetEmail(),
		"success":                        resp.GetSuccess(),
		"otp_issued_after_unix":          resp.GetOtpIssuedAfterUnix(),
		"otp_request_started_at_unix_ms": resp.GetOtpRequestStartedAtUnixMs(),
	}
	if resp.GetErrorMessage() != "" {
		data["error_message"] = resp.GetErrorMessage()
	}
	return data
}
