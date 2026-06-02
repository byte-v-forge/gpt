package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/manualinput"
	"orchestrator/pb"
)

func (s *Server) BrowserAuthResendOTPActivity(ctx context.Context, input *BrowserAuthResendOTPInput) (*BrowserAuthResendOTPOutput, error) {
	output := &BrowserAuthResendOTPOutput{
		BrowserSessionId:  input.GetBrowserSessionId(),
		OtpTimeoutSeconds: s.registrationOtpTimeout(ctx),
	}
	stepName, err := browserAuthOTPRequestStepName(input.GetMode())
	if err != nil {
		return output, err
	}
	if strings.TrimSpace(input.GetBrowserSessionId()) == "" {
		return output, fmt.Errorf("browser_session_id is required")
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}

	data := &pb.ActivityBrowserAuthOTPRequestStepData{
		AccountId:        input.GetAccountId(),
		BrowserSessionId: input.GetBrowserSessionId(),
		Mode:             input.GetMode(),
	}
	step.progress("resending email OTP", data)
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "resending email OTP", data)
	defer stopHeartbeat()

	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	otpKind, _, err := s.getJobParam(ctx, input.GetJobId(), browserAuthOTPKindParam)
	if err != nil {
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if otpKind != "" {
		data.OtpKind = otpKind
	}
	resp, err := s.browserAuthResendOTP(ctx, input.GetMode(), input.GetJobId(), account, input.GetBrowserSessionId(), otpKind)
	data.BrowserResend = browserAuthResendData(resp, input.GetMode())
	if err != nil {
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if resp == nil {
		err := fmt.Errorf("browser %s OTP resend returned empty response", input.GetMode())
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}
	if !resp.GetSuccess() {
		err := fmt.Errorf("browser %s OTP resend failed: %s", input.GetMode(), resp.GetErrorMessage())
		output.Data = data
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, input.GetAccountId(), data, err)
	}

	output.BrowserSessionId = resp.GetBrowserSessionId()
	output.Email = resp.GetEmail()
	output.Success = true
	output.OtpIssuedAfterUnix = resp.GetOtpIssuedAfterUnix()
	output.OtpRequestStartedAtUnixMs = resp.GetOtpRequestStartedAtUnixMs()
	output.OtpTimeoutSeconds = s.registrationOtpTimeout(ctx)
	data.OtpIssuedAfterUnix = output.GetOtpIssuedAfterUnix()
	data.OtpRequestStartedAtUnixMs = output.GetOtpRequestStartedAtUnixMs()
	output.Data = data
	return output, step.complete(data, nil)
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
	code := channelotpwait.NormalizeCode(value)
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

func browserAuthResendData(resp *pb.BrowserAuthResendOTPOutput, mode string) *pb.ActivityBrowserResendOTPData {
	if resp == nil {
		return &pb.ActivityBrowserResendOTPData{
			ResponsePresent: boolPtr(false),
			Mode:            mode,
		}
	}
	return &pb.ActivityBrowserResendOTPData{
		ResponsePresent:           boolPtr(true),
		Success:                   boolPtr(resp.GetSuccess()),
		ErrorMessage:              resp.GetErrorMessage(),
		BrowserSessionId:          resp.GetBrowserSessionId(),
		Email:                     resp.GetEmail(),
		OtpIssuedAfterUnix:        resp.GetOtpIssuedAfterUnix(),
		OtpRequestStartedAtUnixMs: resp.GetOtpRequestStartedAtUnixMs(),
		Mode:                      mode,
	}
}
