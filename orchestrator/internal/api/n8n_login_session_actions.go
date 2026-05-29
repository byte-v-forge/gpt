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

	"orchestrator/internal/emailotpwait"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	n8nLoginSessionResultSecretPrefix    = "n8n-login-session-result:"
	n8nLoginSessionResumeURLSecretPrefix = "n8n-login-session-resume-url:"

	loginSessionEngineParam             = "engine"
	loginSessionFlowIDParam             = "login_session_flow_id"
	loginSessionEmailParam              = "login_session_email"
	loginSessionOTPIssuedAfterParam     = "login_session_otp_issued_after_unix"
	loginSessionOTPTimeoutParam         = "login_session_otp_timeout_seconds"
	loginSessionOTPResumeSecretKeyParam = "login_session_otp_resume_url_secret_key"
)

type n8nBrowserAuthStepResult struct {
	JobID              string         `json:"job_id"`
	AccountID          string         `json:"account_id"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Step               string         `json:"step"`
	BrowserSessionID   string         `json:"browser_session_id,omitempty"`
	Email              string         `json:"email,omitempty"`
	OTPRequired        bool           `json:"otp_required"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	MessageID          string         `json:"message_id,omitempty"`
	ResultReady        bool           `json:"result_ready"`
	ResultRef          string         `json:"result_ref,omitempty"`
	Waiting            bool           `json:"waiting,omitempty"`
	Success            bool           `json:"success"`
	Data               map[string]any `json:"data,omitempty"`
}

func (s *Server) StartN8NLoginSession(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error) {
	jobID := uuid.NewString()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: "account_id is required"}, "", fmt.Errorf("account_id is required")
	}
	account, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	if account.GetAccount() == nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: "account not found"}, accountID, fmt.Errorf("account not found")
	}
	params := map[string]string{
		"account_id":               accountID,
		loginSessionEngineParam:    "n8n",
		loginSessionEmailParam:     emailx.Normalize(account.GetAccount().GetEmail()),
		"target_connectivity_urls": "https://api.openai.com/v1/models",
	}
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionLoginSession, params); err != nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.LoginAccountResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) StartN8NLoginSessionBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginSessionIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginSessionExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.BrowserAuthStartActivity(ctx, pb.BrowserAuthStartInput{JobId: jobID, AccountId: accountID, Mode: browserLoginMode})
	result := s.n8nLoginSessionStartResult(ctx, jobID, accountID, n8nExecutionID, out)
	if err != nil {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: out.GetBrowserSessionId(), Mode: browserLoginMode})
		return result, s.markActionFailed(ctx, jobID, stepLoginSessionStart, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	if result.ResultReady && result.ResultRef == "" {
		return result, fmt.Errorf("failed to persist login result")
	}
	if errMessage := strings.TrimSpace(fmt.Sprint(result.Data["result_secret_error"])); errMessage != "" && errMessage != "<nil>" {
		return result, fmt.Errorf("failed to persist login result: %s", errMessage)
	}
	return result, nil
}

func (s *Server) AwaitN8NLoginSessionOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginSessionIDs(jobID, accountID, n8nExecutionID)
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
	if err := s.bindN8NLoginSessionExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	secretKey := n8nLoginSessionResumeURLSecretPrefix + jobID
	ttl := time.Duration(timeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, resumeURL, ttl); err != nil {
		return nil, err
	}
	entry := emailotpwait.Entry{JobID: jobID, AccountID: accountID, FlowID: browserSessionID, Email: email, StepName: stepLoginSessionOTPWait, IssuedAfterUnix: otpIssuedAfterUnix, TimeoutSeconds: timeoutSeconds, ResumeSecretKey: secretKey, N8NExecutionID: n8nExecutionID}
	if err := s.emailOTPWaits.Register(ctx, entry, ttl); err != nil {
		return nil, err
	}
	detail := map[string]any{
		"account_id":             accountID,
		"browser_session_id":     browserSessionID,
		"flow_id":                browserSessionID,
		"email":                  email,
		"timeout_seconds":        timeoutSeconds,
		"otp_issued_after_unix":  otpIssuedAfterUnix,
		"resume_url_registered":  true,
		"mailbox_poll_requested": true,
		"n8n_execution_id":       n8nExecutionID,
		"wait_index":             "redis_ttl",
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: jobID, StepName: stepLoginSessionOTPWait, Recoverable: false, Retryable: true, Detail: structData(detail)}); err != nil {
		return nil, err
	}
	params := map[string]string{
		loginSessionFlowIDParam:             browserSessionID,
		loginSessionEmailParam:              email,
		loginSessionOTPIssuedAfterParam:     fmt.Sprintf("%d", otpIssuedAfterUnix),
		loginSessionOTPTimeoutParam:         fmt.Sprintf("%d", timeoutSeconds),
		loginSessionOTPResumeSecretKeyParam: secretKey,
	}
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
		return &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepLoginSessionOTPWait, BrowserSessionID: browserSessionID, Email: email, OTPRequired: true, OTPIssuedAfterUnix: otpIssuedAfterUnix, OTPTimeoutSeconds: timeoutSeconds, OTPSource: "mailbox", MessageID: strings.TrimSpace(message.GetId()), Success: true, Data: detail}, nil
	}
	if s.mailboxPollRequester == nil {
		return nil, fmt.Errorf("mailbox poll requester is required")
	}
	if err := s.mailboxPollRequester.RequestMailboxEmailPoll(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED, otpIssuedAfterUnix, time.Duration(timeoutSeconds)*time.Second, "gpt_login_session_otp"); err != nil {
		return nil, err
	}
	return &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepLoginSessionOTPWait, BrowserSessionID: browserSessionID, Email: email, OTPRequired: true, OTPIssuedAfterUnix: otpIssuedAfterUnix, OTPTimeoutSeconds: timeoutSeconds, Waiting: true, Success: true, Data: detail}, nil
}

func (s *Server) CompleteN8NLoginSessionBrowser(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, otpSource string, otpIssuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginSessionIDs(jobID, accountID, n8nExecutionID)
	browserSessionID = strings.TrimSpace(browserSessionID)
	if err := s.bindN8NLoginSessionExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.BrowserAuthCompleteActivity(ctx, pb.BrowserAuthCompleteInput{JobId: jobID, AccountId: accountID, BrowserSessionId: browserSessionID, Mode: browserLoginMode, OtpParam: registrationOTPParam, SubmittedAtParam: registrationOTPSubmittedAtParam, OtpIssuedAfterUnix: otpIssuedAfterUnix, OtpSource: strings.TrimSpace(otpSource)})
	result, resultErr := s.n8nLoginSessionOutputResult(ctx, jobID, accountID, n8nExecutionID, stepLoginSessionComplete, browserSessionID, &out)
	if err != nil {
		_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: browserSessionID, Mode: browserLoginMode})
		return result, s.markActionFailed(ctx, jobID, stepLoginSessionComplete, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) FinishN8NLoginSession(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginSessionIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginSessionExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nLoginSessionResultSecretPrefix + jobID
	}
	login, err := s.loadN8NLoginSessionResult(ctx, jobID, resultRef)
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepLoginSessionComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	if login == nil || strings.TrimSpace(login.GetSessionToken()) == "" || strings.TrimSpace(login.GetAccessToken()) == "" {
		err := fmt.Errorf("browser login did not return both ChatGPT session and access tokens")
		return nil, s.markActionFailed(ctx, jobID, stepLoginSessionComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	result := map[string]any{"account_id": accountID, "driver": "browser", "mode": browserLoginMode, "n8n_execution_id": n8nExecutionID, "session_token_present": true, "access_token_present": true}
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{AccountId: accountID, SessionToken: login.GetSessionToken(), AccessToken: login.GetAccessToken()}); err != nil {
		return nil, s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, result)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteRuntimeSecretValue(ctx, resultRef)
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionLoginSession, Started: true, Success: true, Result: result}, nil
}

func (s *Server) FailN8NLoginSession(ctx context.Context, jobID string, accountID string, n8nExecutionID string, browserSessionID string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginSessionIDs(jobID, accountID, n8nExecutionID)
	if data == nil {
		data = map[string]any{}
	}
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage == "" {
		errorMessage = "login session failed"
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	if err := s.bindN8NLoginSessionExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	step := s.loginSessionFailureStep(ctx, jobID)
	if markErr := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, fmt.Errorf("%s", errorMessage), data); markErr != nil {
		return nil, markErr
	}
	_ = s.activities.BrowserAuthCancelActivity(ctx, pb.BrowserAuthCancelInput{JobId: jobID, BrowserSessionId: strings.TrimSpace(browserSessionID), Mode: browserLoginMode})
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionLoginSession, Started: true, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) n8nLoginSessionStartResult(ctx context.Context, jobID string, accountID string, n8nExecutionID string, out pb.BrowserAuthStartOutput) *n8nBrowserAuthStepResult {
	data := structMap(out.GetData())
	if data == nil {
		data = map[string]any{}
	}
	result := &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepLoginSessionStart, BrowserSessionID: strings.TrimSpace(out.GetBrowserSessionId()), Email: emailx.Normalize(out.GetEmail()), OTPRequired: out.GetOtpRequired(), OTPIssuedAfterUnix: out.GetOtpIssuedAfterUnix(), OTPTimeoutSeconds: out.GetOtpTimeoutSeconds(), Success: true, Data: data}
	if login := out.GetResult(); login != nil {
		if ref, err := s.saveN8NLoginSessionResult(ctx, jobID, login); err == nil {
			result.ResultReady = true
			result.ResultRef = ref
		} else {
			result.Data["result_secret_error"] = err.Error()
		}
	}
	return result
}

func (s *Server) n8nLoginSessionOutputResult(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, browserSessionID string, out *pb.RegisterActivityOutput) (*n8nBrowserAuthStepResult, error) {
	data := map[string]any{}
	if out != nil {
		data = structMap(out.GetData())
		if data == nil {
			data = map[string]any{}
		}
	}
	result := &n8nBrowserAuthStepResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: step, BrowserSessionID: strings.TrimSpace(browserSessionID), Success: true, Data: data}
	if out != nil {
		ref, err := s.saveN8NLoginSessionResult(ctx, jobID, out)
		if err != nil {
			return result, err
		}
		result.ResultReady = true
		result.ResultRef = ref
	}
	return result, nil
}

func (s *Server) saveN8NLoginSessionResult(ctx context.Context, jobID string, result *pb.RegisterActivityOutput) (string, error) {
	if result == nil {
		return "", nil
	}
	key := n8nLoginSessionResultSecretPrefix + strings.TrimSpace(jobID)
	data, err := protojsonx.Marshal(result)
	if err != nil {
		return "", err
	}
	if err := s.saveRuntimeSecretValueTTL(ctx, key, string(data), s.runtimeSecretTTL()); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) loadN8NLoginSessionResult(ctx context.Context, jobID string, resultRef string) (*pb.RegisterActivityOutput, error) {
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nLoginSessionResultSecretPrefix + strings.TrimSpace(jobID)
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

func (s *Server) bindN8NLoginSessionExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) loginSessionFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepLoginSessionStart
}

func normalizeN8NLoginSessionIDs(jobID string, accountID string, n8nExecutionID string) (string, string, string) {
	return strings.TrimSpace(jobID), strings.TrimSpace(accountID), strings.TrimSpace(n8nExecutionID)
}
