package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/protojsonx"
	"github.com/google/uuid"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/emailotpwait"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	n8nRegisterResultSecretPrefix    = "n8n-register-result:"
	n8nRegisterResumeURLSecretPrefix = "n8n-register-resume-url:"

	registerEngineParam             = "engine"
	registerFlowIDParam             = "register_flow_id"
	registerEmailParam              = "register_email"
	registerOTPIssuedAfterParam     = "register_otp_issued_after_unix"
	registerOTPTimeoutParam         = "register_otp_timeout_seconds"
	registerOTPResumeSecretKeyParam = "register_otp_resume_url_secret_key"
)

func (s *Server) StartN8NRegister(ctx context.Context, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error) {
	jobID := uuid.NewString()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	if s.activities == nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: "GPT action API is not configured"}, "", fmt.Errorf("GPT action API is not configured")
	}
	account, err := s.activities.EnsureAccountActivity(ctx, pb.EnsureAccountInput{Account: &pb.AccountSpec{AccountId: accountID, Email: req.GetEmail(), Password: req.GetPassword(), EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()), CountryCode: req.GetCountryCode(), Region: req.GetRegion()}})
	if err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	accountID = account.GetAccountId()
	if err := s.generateAccountFingerprint(ctx, accountID, accountfingerprint.GenerateParams{CountryCode: req.GetCountryCode(), Region: req.GetRegion()}); err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	accountRecord, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	params := registerAccountJobParams(accountID, req.GetOtpOptions(), req.GetCountryCode(), req.GetRegion())
	params[registerEngineParam] = "n8n"
	params[registerEmailParam] = emailx.Normalize(accountRecord.GetAccount().GetEmail())
	params["target_connectivity_urls"] = "https://api.openai.com/v1/models"
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionRegister, params); err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.RegisterAccountResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) StartN8NRegisterBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.BrowserAuthStartActivity(ctx, pb.BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: protocolRegisterMode})
	result := s.n8nRegisterStartResult(ctx, jobID, accountID, n8nExecutionID, out)
	if err != nil {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: out.GetBrowserSessionId(), Mode: protocolRegisterMode})
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountStart, registerProtocolFailureStatus(err), registerProtocolRecoverable(err), registerProtocolRetryable(err), err, structMap(out.GetData()))
	}
	if errMessage := strings.TrimSpace(fmt.Sprint(result.Data["result_secret_error"])); errMessage != "" && errMessage != "<nil>" {
		return result, fmt.Errorf("failed to persist register result: %s", errMessage)
	}
	return result, nil
}

func (s *Server) AwaitN8NRegisterOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterIDs(jobID, accountID, n8nExecutionID)
	browserSessionID = strings.TrimSpace(browserSessionID)
	email = emailx.Normalize(email)
	resumeURL = strings.TrimSpace(resumeURL)
	if jobID == "" || accountID == "" || browserSessionID == "" || email == "" || resumeURL == "" {
		return nil, fmt.Errorf("job_id, account_id, browser_session_id, email and resume_url are required")
	}
	if s.emailOTPWaits == nil {
		return nil, fmt.Errorf("email otp wait store is required")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	if err := s.bindN8NRegisterExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	secretKey := n8nRegisterResumeURLSecretPrefix + jobID
	ttl := time.Duration(timeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, resumeURL, ttl); err != nil {
		return nil, err
	}
	entry := emailotpwait.Entry{JobID: jobID, AccountID: accountID, FlowID: browserSessionID, Email: email, StepName: stepRegisterAccountOTPWait, IssuedAfterUnix: otpIssuedAfterUnix, TimeoutSeconds: timeoutSeconds, ResumeSecretKey: secretKey, N8NExecutionID: n8nExecutionID}
	if err := s.emailOTPWaits.Register(ctx, entry, ttl); err != nil {
		return nil, err
	}
	detail := map[string]any{"account_id": accountID, "browser_session_id": browserSessionID, "flow_id": browserSessionID, "email": email, "timeout_seconds": timeoutSeconds, "otp_issued_after_unix": otpIssuedAfterUnix, "resume_url_registered": true, "mailbox_poll_requested": true, "n8n_execution_id": n8nExecutionID, "wait_index": "redis_ttl"}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: jobID, StepName: stepRegisterAccountOTPWait, Recoverable: false, Retryable: true, Detail: structData(detail)}); err != nil {
		return nil, err
	}
	params := map[string]string{registerFlowIDParam: browserSessionID, registerEmailParam: email, registerOTPIssuedAfterParam: fmt.Sprintf("%d", otpIssuedAfterUnix), registerOTPTimeoutParam: fmt.Sprintf("%d", timeoutSeconds), registerOTPResumeSecretKeyParam: secretKey}
	if n8nExecutionID != "" {
		params["n8n_execution_id"] = n8nExecutionID
	}
	if err := s.setJobParams(ctx, jobID, params); err != nil {
		return nil, err
	}
	if message, code, ok, err := s.latestMailboxEmailOTP(ctx, email, otpIssuedAfterUnix); err != nil {
		return nil, err
	} else if ok {
		receivedAt := message.GetReceivedAtUnix()
		if receivedAt <= 0 {
			receivedAt = time.Now().Unix()
		}
		otpData, err := s.completeMailboxEmailOTPWait(ctx, entry, message, code, receivedAt)
		if err != nil {
			return nil, err
		}
		if err := s.markEmailOTPResolved(ctx, jobID, time.Now().Unix()); err != nil {
			return nil, err
		}
		_ = s.emailOTPWaits.Delete(ctx, entry)
		for key, value := range otpData {
			detail[key] = value
		}
		return &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepRegisterAccountOTPWait, BrowserSessionID: browserSessionID, Email: email, OTPRequired: true, OTPIssuedAfterUnix: otpIssuedAfterUnix, OTPTimeoutSeconds: timeoutSeconds, OTPSource: "mailbox", MessageID: strings.TrimSpace(message.GetId()), Success: true, Data: detail}, nil
	}
	if s.mailboxPollRequester == nil {
		return nil, fmt.Errorf("mailbox poll requester is required")
	}
	if err := s.mailboxPollRequester.RequestMailboxEmailPoll(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED, otpIssuedAfterUnix, time.Duration(timeoutSeconds)*time.Second, "gpt_register_otp"); err != nil {
		return nil, err
	}
	return &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepRegisterAccountOTPWait, BrowserSessionID: browserSessionID, Email: email, OTPRequired: true, OTPIssuedAfterUnix: otpIssuedAfterUnix, OTPTimeoutSeconds: timeoutSeconds, Waiting: true, Success: true, Data: detail}, nil
}

func (s *Server) CompleteN8NRegisterBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, otpSource string, otpIssuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterIDs(jobID, accountID, n8nExecutionID)
	browserSessionID = strings.TrimSpace(browserSessionID)
	if err := s.bindN8NRegisterExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.BrowserAuthCompleteActivity(ctx, pb.BrowserAuthCompleteInput{JobId: jobID, AccountId: accountID, BrowserSessionId: browserSessionID, Mode: protocolRegisterMode, OtpParam: registrationOTPParam, SubmittedAtParam: registrationOTPSubmittedAtParam, OtpIssuedAfterUnix: otpIssuedAfterUnix, OtpSource: strings.TrimSpace(otpSource)})
	result, resultErr := s.n8nRegisterOutputResult(ctx, jobID, accountID, n8nExecutionID, stepRegisterAccountComplete, browserSessionID, &out)
	if err != nil {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: protocolRegisterMode})
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountComplete, registerProtocolFailureStatus(err), registerProtocolRecoverable(err), registerProtocolRetryable(err), err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) FinishN8NRegister(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nRegisterResultSecretPrefix + jobID
	}
	register, err := s.loadN8NRegisterResult(ctx, jobID, resultRef)
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepRegisterAccountComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	if register == nil || strings.TrimSpace(register.GetSessionToken()) == "" || strings.TrimSpace(register.GetAccessToken()) == "" {
		err := fmt.Errorf("browser register did not return both ChatGPT session and access tokens")
		return nil, s.markActionFailed(ctx, jobID, stepRegisterAccountComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	result := map[string]any{"account_id": accountID, "driver": "browser", "mode": protocolRegisterMode, "n8n_execution_id": n8nExecutionID, "session_token_present": true, "access_token_present": true, "plus_trial_checked": register.GetPlusTrialChecked(), "plus_trial_eligible": register.GetPlusTrialEligible()}
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{AccountId: accountID, SessionToken: register.GetSessionToken(), AccessToken: register.GetAccessToken(), PlusTrialEligible: register.GetPlusTrialEligible(), PlusTrialChecked: register.GetPlusTrialChecked()}); err != nil {
		return nil, s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, result)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteRuntimeSecretValue(ctx, resultRef)
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionRegister, Started: true, Success: true, Result: result}, nil
}

func (s *Server) FailN8NRegister(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterIDs(jobID, accountID, n8nExecutionID)
	if data == nil {
		data = map[string]any{}
	}
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage == "" {
		errorMessage = "register failed"
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	if err := s.bindN8NRegisterExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	step := s.registerFailureStep(ctx, jobID)
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, step, registerProtocolFailureStatus(err), registerProtocolRecoverable(err), registerProtocolRetryable(err), err, data); markErr != nil {
		return nil, markErr
	}
	_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: strings.TrimSpace(browserSessionID), Mode: protocolRegisterMode})
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionRegister, Started: true, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) n8nRegisterStartResult(ctx context.Context, jobID string, accountID string, n8nExecutionID string, out pb.BrowserAuthStartOutput) *n8nBrowserAuthStepResult {
	data := structMap(out.GetData())
	if data == nil {
		data = map[string]any{}
	}
	result := &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepRegisterAccountStart, BrowserSessionID: strings.TrimSpace(out.GetBrowserSessionId()), Email: emailx.Normalize(out.GetEmail()), OTPRequired: out.GetOtpRequired(), OTPIssuedAfterUnix: out.GetOtpIssuedAfterUnix(), OTPTimeoutSeconds: out.GetOtpTimeoutSeconds(), Success: true, Data: data}
	if register := out.GetResult(); register != nil {
		if ref, err := s.saveN8NRegisterResult(ctx, jobID, register); err == nil {
			result.ResultReady = true
			result.ResultRef = ref
		} else {
			result.Data["result_secret_error"] = err.Error()
		}
	}
	return result
}

func (s *Server) n8nRegisterOutputResult(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, browserSessionID string, out *pb.RegisterActivityOutput) (*n8nBrowserAuthStepResult, error) {
	data := map[string]any{}
	if out != nil {
		data = structMap(out.GetData())
		if data == nil {
			data = map[string]any{}
		}
	}
	result := &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: step, BrowserSessionID: strings.TrimSpace(browserSessionID), Success: true, Data: data}
	if out != nil {
		ref, err := s.saveN8NRegisterResult(ctx, jobID, out)
		if err != nil {
			return result, err
		}
		result.ResultReady = true
		result.ResultRef = ref
	}
	return result, nil
}

func (s *Server) saveN8NRegisterResult(ctx context.Context, jobID string, result *pb.RegisterActivityOutput) (string, error) {
	if result == nil {
		return "", nil
	}
	key := n8nRegisterResultSecretPrefix + strings.TrimSpace(jobID)
	data, err := protojsonx.Marshal(result)
	if err != nil {
		return "", err
	}
	if err := s.saveRuntimeSecretValueTTL(ctx, key, string(data), s.runtimeSecretTTL()); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) loadN8NRegisterResult(ctx context.Context, jobID string, resultRef string) (*pb.RegisterActivityOutput, error) {
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nRegisterResultSecretPrefix + strings.TrimSpace(jobID)
	}
	raw, err := s.runtimeSecretValue(ctx, resultRef)
	if err != nil {
		return nil, err
	}
	out := &pb.RegisterActivityOutput{}
	if err := protojsonx.Unmarshal([]byte(raw), out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) bindN8NRegisterExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) registerFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepRegisterAccountStart
}

func normalizeN8NRegisterIDs(jobID string, accountID string, n8nExecutionID string) (string, string, string) {
	return strings.TrimSpace(jobID), strings.TrimSpace(accountID), strings.TrimSpace(n8nExecutionID)
}
