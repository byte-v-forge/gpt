package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	browserRegisterMode                    = "register"
	registrationOTPDefaultFirstWaitSeconds = int32(30)
	registrationOTPDefaultWaitSeconds      = int32(180)
	registrationOTPResendRequestedAtParam  = "registration_otp_resend_requested_at_unix"
	registrationOTPResendHandledAtParam    = "registration_otp_resend_handled_at_unix"
)

type registerOTPPolicy struct {
	ManualOnly       bool
	AutoResend       bool
	FirstWaitSeconds int32
	TimeoutSeconds   int32
	Mode             string
}

type browserRegisterActionResult struct {
	AccountID string
	Register  pb.RegisterActivityOutput
	Data      map[string]any
}

func (s *Server) runRegisterAccountAction(ctx context.Context, jobID string, accountID string, params map[string]string) error {
	registered, err := s.runBrowserRegisterAccountAction(ctx, jobID, accountID, params)
	if err != nil {
		return err
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(registered.Data)}); err != nil {
		return err
	}
	_, _ = s.jobStore.CreateWithID(ctx, jobID+"-probe", registered.AccountID, actionProbeAccount, map[string]string{"account_id": registered.AccountID, "source_job_id": jobID})
	return nil
}

func (s *Server) runBrowserRegisterAccountAction(ctx context.Context, jobID string, accountID string, params map[string]string) (browserRegisterActionResult, error) {
	result := browserRegisterActionResult{}
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: strings.TrimSpace(accountID)})
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, nil)
	}
	accountID = account.GetAccountId()
	result.AccountID = accountID
	combined := map[string]any{"account_id": accountID, "driver": "browser", "mode": browserRegisterMode}
	result.Data = combined

	start, err := s.activities.BrowserAuthStartActivity(ctx, pb.BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: browserRegisterMode})
	mergeActionData(combined, stepRegisterAccountStart, structMap(start.GetData()))
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountStart, registerProtocolFailureStatus(err), registerProtocolRecoverable(err), registerProtocolRetryable(err), err, combined)
	}
	register := start.GetResult()
	browserSessionID := start.GetBrowserSessionId()
	email := start.GetEmail()

	otpRequired := false
	otpIssuedAfter := int64(0)
	otpTimeoutSeconds := int32(0)
	if register == nil {
		if start.GetOtpRequired() {
			otpRequired = true
			otpIssuedAfter = start.GetOtpIssuedAfterUnix()
			otpTimeoutSeconds = start.GetOtpTimeoutSeconds()
			mergeActionData(combined, stepRegisterAccountBrowser, structMap(start.GetData()))
		} else {
			err := fmt.Errorf("browser register did not reach terminal or OTP state")
			_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: browserRegisterMode})
			return result, s.markActionFailed(ctx, jobID, stepRegisterAccountBrowser, registerProtocolFailureStatus(err), registerProtocolRecoverable(err), registerProtocolRetryable(err), err, combined)
		}
	}

	if otpRequired {
		policy := registerOTPPolicyFromParams(params, otpTimeoutSeconds)
		otp, issuedAfter, err := s.waitBrowserRegistrationOTP(ctx, registerOTPWaitRequest{JobID: jobID, AccountID: accountID, BrowserSessionID: browserSessionID, Email: email, IssuedAfterUnix: otpIssuedAfter, TimeoutSeconds: otpTimeoutSeconds, Policy: policy})
		mergeActionData(combined, stepRegisterAccountOTPWait, otpWaitData(email, policy.TimeoutSeconds, issuedAfter, otp))
		if err != nil {
			_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: browserRegisterMode})
			return result, s.markActionFailed(ctx, jobID, stepRegisterAccountOTPWait, jobstatus.FailedRetryable, false, true, err, combined)
		}
		completed, err := s.activities.BrowserAuthCompleteActivity(ctx, pb.BrowserAuthCompleteInput{JobId: jobID, AccountId: accountID, BrowserSessionId: browserSessionID, Mode: browserRegisterMode, OtpParam: registrationOTPParam, SubmittedAtParam: registrationOTPSubmittedAtParam, OtpIssuedAfterUnix: issuedAfter, OtpSource: otp.GetSource()})
		mergeActionData(combined, stepRegisterAccountComplete, structMap(completed.GetData()))
		if err != nil {
			return result, s.markActionFailed(ctx, jobID, stepRegisterAccountComplete, registerProtocolFailureStatus(err), registerProtocolRecoverable(err), registerProtocolRetryable(err), err, combined)
		}
		register = &completed
	}

	if register == nil || strings.TrimSpace(register.GetSessionToken()) == "" || strings.TrimSpace(register.GetAccessToken()) == "" {
		err := fmt.Errorf("browser register did not return session_token and access_token")
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountComplete, jobstatus.FailedRetryable, false, true, err, combined)
	}
	mergeActionData(combined, "register_account", structMap(register.GetData()))
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{AccountId: accountID, SessionToken: register.GetSessionToken(), AccessToken: register.GetAccessToken(), PlusTrialEligible: register.GetPlusTrialEligible(), PlusTrialChecked: register.GetPlusTrialChecked()}); err != nil {
		return result, s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, combined)
	}
	result.Register = *register
	return result, nil
}

type registerOTPWaitRequest struct {
	JobID            string
	AccountID        string
	BrowserSessionID string
	Email            string
	IssuedAfterUnix  int64
	TimeoutSeconds   int32
	Policy           registerOTPPolicy
}

func (s *Server) waitBrowserRegistrationOTP(ctx context.Context, req registerOTPWaitRequest) (pb.OTPWaitOutput, int64, error) {
	policy := req.Policy
	if policy.TimeoutSeconds <= 0 {
		policy = registerOTPPolicyFromParams(nil, req.TimeoutSeconds)
	}
	issuedAfter := req.IssuedAfterUnix
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: req.JobID, StepName: stepRegisterAccountOTPWait, Recoverable: false, Retryable: true, Detail: structData(registrationOTPWaitStartData(req.Email, issuedAfter, policy, 0))}); err != nil {
		return pb.OTPWaitOutput{}, issuedAfter, err
	}
	resendCount := 0
	autoResent := false
	for {
		waitSeconds := policy.TimeoutSeconds
		if policy.AutoResend && !autoResent && !policy.ManualOnly {
			waitSeconds = policy.FirstWaitSeconds
		}
		otp, err := s.waitRegistrationOTPRound(ctx, req, policy, issuedAfter, waitSeconds)
		if err == nil && otp.GetFound() {
			return otp, issuedAfter, s.completeRegistrationOTPWait(ctx, req.JobID, req.Email, issuedAfter, policy, resendCount, otp)
		}
		if s.registrationOTPResendRequested(ctx, req.JobID, issuedAfter) || (err != nil && policy.AutoResend && !autoResent && !policy.ManualOnly) {
			if policy.AutoResend && !autoResent && !policy.ManualOnly {
				autoResent = true
			}
			resend, resendErr := s.activities.BrowserAuthResendOTPActivity(ctx, pb.BrowserAuthResendOTPInput{JobId: req.JobID, AccountId: req.AccountID, BrowserSessionId: req.BrowserSessionID, Mode: browserRegisterMode})
			mergeData := structMap(resend.GetData())
			if resendErr != nil {
				return pb.OTPWaitOutput{Data: structData(mergeData)}, issuedAfter, resendErr
			}
			if !resend.GetSuccess() || resend.GetOtpIssuedAfterUnix() <= 0 {
				return pb.OTPWaitOutput{Data: structData(mergeData)}, issuedAfter, fmt.Errorf("browser register OTP resend failed: %s", resend.GetErrorMessage())
			}
			issuedAfter = resend.GetOtpIssuedAfterUnix()
			resendCount++
			_ = s.setJobParams(ctx, req.JobID, map[string]string{registrationOTPResendHandledAtParam: strconv.FormatInt(time.Now().Unix(), 10)})
			continue
		}
		if err != nil {
			return otp, issuedAfter, err
		}
		return otp, issuedAfter, fmt.Errorf("otp wait ended unexpectedly")
	}
}

func (s *Server) waitRegistrationOTPRound(ctx context.Context, req registerOTPWaitRequest, policy registerOTPPolicy, issuedAfter int64, waitSeconds int32) (pb.OTPWaitOutput, error) {
	if waitSeconds <= 0 {
		waitSeconds = policy.TimeoutSeconds
	}
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	lastErr := ""
	for {
		input := pb.OTPWaitInput{JobId: req.JobID, StepName: stepRegisterAccountOTPWait, Target: &pb.OTPWaitInput_Email{Email: &pb.OTPWaitEmailTarget{Email: req.Email}}, TimeoutSeconds: minOTPWaitChunkSeconds(int32(time.Until(deadline).Seconds())), IssuedAfterUnix: issuedAfter, OtpParam: registrationOTPParam, SubmittedAtParam: registrationOTPSubmittedAtParam}
		manual, err := s.activities.FetchManualOTPActivity(ctx, input)
		if err != nil {
			lastErr = err.Error()
		} else if manual.GetFound() {
			return manual, nil
		}
		if s.registrationOTPResendRequested(ctx, req.JobID, issuedAfter) {
			return pb.OTPWaitOutput{}, fmt.Errorf("registration otp resend requested")
		}
		if time.Now().After(deadline) {
			break
		}
		if !policy.ManualOnly {
			chunkSeconds := minOTPWaitChunkSeconds(int32(time.Until(deadline).Seconds()))
			if chunkSeconds > 0 {
				input.TimeoutSeconds = chunkSeconds
				out, err := s.activities.OTPWaitActivity(ctx, input)
				if err != nil {
					lastErr = err.Error()
				} else if out.GetFound() {
					return out, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return pb.OTPWaitOutput{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if lastErr != "" {
		return pb.OTPWaitOutput{ErrorMessage: lastErr}, fmt.Errorf("otp not received after %ds: %s", waitSeconds, lastErr)
	}
	return pb.OTPWaitOutput{}, fmt.Errorf("otp not received after %ds", waitSeconds)
}

func (s *Server) registrationOTPResendRequested(ctx context.Context, jobID string, issuedAfter int64) bool {
	requestedRaw, found, err := s.jobStore.GetParam(ctx, jobID, registrationOTPResendRequestedAtParam)
	if err != nil || !found {
		return false
	}
	requestedAt, _ := strconv.ParseInt(strings.TrimSpace(requestedRaw), 10, 64)
	handledRaw, _, _ := s.jobStore.GetParam(ctx, jobID, registrationOTPResendHandledAtParam)
	handledAt, _ := strconv.ParseInt(strings.TrimSpace(handledRaw), 10, 64)
	return requestedAt > handledAt && requestedAt >= issuedAfter
}

func (s *Server) completeRegistrationOTPWait(ctx context.Context, jobID string, email string, issuedAfter int64, policy registerOTPPolicy, resendCount int, output pb.OTPWaitOutput) error {
	result := otpWaitData(email, policy.TimeoutSeconds, issuedAfter, output)
	for key, value := range registrationOTPWaitStartData(email, issuedAfter, policy, resendCount) {
		result[key] = value
	}
	return s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{JobId: jobID, StepName: stepRegisterAccountOTPWait, Recoverable: false, Retryable: true, Result: structData(result)})
}

func registrationOTPWaitStartData(email string, issuedAfter int64, policy registerOTPPolicy, resendCount int) map[string]any {
	data := otpWaitStartData(email, policy.TimeoutSeconds, issuedAfter)
	data["otp_mode"] = policy.Mode
	data["manual_only"] = policy.ManualOnly
	data["auto_resend"] = policy.AutoResend
	data["first_wait_seconds"] = policy.FirstWaitSeconds
	data["resend_count"] = resendCount
	return data
}

func registerOTPPolicyFromParams(params map[string]string, fallbackTimeout int32) registerOTPPolicy {
	mode := strings.TrimSpace(params["registration_otp_mode"])
	manualOnly := strings.EqualFold(mode, pb.RegisterOTPMode_REGISTER_OTP_MODE_MANUAL.String()) || strings.EqualFold(mode, "manual")
	if mode == "" {
		mode = "auto"
	}
	timeoutSeconds := int32Param(params, "registration_otp_timeout_seconds", fallbackTimeout)
	if timeoutSeconds <= 0 {
		timeoutSeconds = registrationOTPDefaultWaitSeconds
	}
	firstWaitSeconds := int32Param(params, "registration_otp_first_wait_seconds", registrationOTPDefaultFirstWaitSeconds)
	autoResend := !manualOnly
	if value := strings.TrimSpace(params["registration_otp_auto_resend"]); value != "" {
		autoResend = strings.EqualFold(value, "true")
	}
	return registerOTPPolicy{ManualOnly: manualOnly, AutoResend: autoResend, FirstWaitSeconds: firstWaitSeconds, TimeoutSeconds: timeoutSeconds, Mode: mode}
}

func int32Param(params map[string]string, key string, fallback int32) int32 {
	value, err := strconv.ParseInt(strings.TrimSpace(params[key]), 10, 32)
	if err != nil || value <= 0 {
		return fallback
	}
	return int32(value)
}
