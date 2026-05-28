package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const browserLoginMode = "login"

func (s *Server) runLoginSessionAction(ctx context.Context, jobID string, accountID string) error {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: strings.TrimSpace(accountID)})
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, nil)
	}
	accountID = account.GetAccountId()
	combined := map[string]any{"account_id": accountID, "driver": "browser", "mode": browserLoginMode}

	start, err := s.activities.BrowserAuthStartActivity(ctx, pb.BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: browserLoginMode})
	mergeActionData(combined, stepLoginSessionStart, structMap(start.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepLoginSessionStart, jobstatus.FailedRetryable, false, true, err, combined)
	}
	login := start.GetResult()
	browserSessionID := start.GetBrowserSessionId()
	email := start.GetEmail()

	otpRequired := false
	otpIssuedAfter := int64(0)
	otpTimeoutSeconds := int32(0)
	if login == nil {
		if start.GetOtpRequired() {
			otpRequired = true
			otpIssuedAfter = start.GetOtpIssuedAfterUnix()
			otpTimeoutSeconds = start.GetOtpTimeoutSeconds()
		} else {
			err := fmt.Errorf("browser login did not reach terminal or OTP state")
			_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: browserLoginMode})
			return s.markActionFailed(ctx, jobID, stepLoginSessionBrowser, jobstatus.FailedRetryable, false, true, err, combined)
		}
	}

	if otpRequired {
		otp, err := s.waitProtocolEmailOTP(ctx, jobID, stepLoginSessionOTPWait, email, otpTimeoutSeconds, otpIssuedAfter)
		mergeActionData(combined, stepLoginSessionOTPWait, otpWaitData(email, otpTimeoutSeconds, otpIssuedAfter, otp))
		if err != nil {
			_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: browserLoginMode})
			return s.markActionFailed(ctx, jobID, stepLoginSessionOTPWait, jobstatus.FailedRetryable, false, true, err, combined)
		}
		completed, err := s.activities.BrowserAuthCompleteActivity(ctx, pb.BrowserAuthCompleteInput{
			JobId:              jobID,
			AccountId:          accountID,
			BrowserSessionId:   browserSessionID,
			Mode:               browserLoginMode,
			OtpParam:           registrationOTPParam,
			SubmittedAtParam:   registrationOTPSubmittedAtParam,
			OtpIssuedAfterUnix: otpIssuedAfter,
			OtpSource:          otp.GetSource(),
		})
		mergeActionData(combined, stepLoginSessionComplete, structMap(completed.GetData()))
		if err != nil {
			_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: browserLoginMode})
			return s.markActionFailed(ctx, jobID, stepLoginSessionComplete, jobstatus.FailedRetryable, false, true, err, combined)
		}
		login = &completed
	}

	if login == nil || strings.TrimSpace(login.GetSessionToken()) == "" || strings.TrimSpace(login.GetAccessToken()) == "" {
		err := fmt.Errorf("browser login did not return session_token and access_token")
		return s.markActionFailed(ctx, jobID, stepLoginSessionComplete, jobstatus.FailedRetryable, false, true, err, combined)
	}
	mergeActionData(combined, "login_session", structMap(login.GetData()))
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{AccountId: accountID, SessionToken: login.GetSessionToken(), AccessToken: login.GetAccessToken()}); err != nil {
		return s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, combined)
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}
