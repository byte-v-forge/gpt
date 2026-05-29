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
	n8nLoginProtocolResultSecretPrefix    = "n8n-login-session-protocol-result:"
	n8nLoginProtocolResumeURLSecretPrefix = "n8n-login-session-protocol-resume-url:"

	loginProtocolEngineParam             = "engine"
	loginProtocolFlowIDParam             = "login_protocol_flow_id"
	loginProtocolEmailParam              = "login_protocol_email"
	loginProtocolOTPIssuedAfterParam     = "login_protocol_otp_issued_after_unix"
	loginProtocolOTPTimeoutParam         = "login_protocol_otp_timeout_seconds"
	loginProtocolOTPResumeSecretKeyParam = "login_protocol_otp_resume_url_secret_key"
)

func (s *Server) StartN8NLoginSessionProtocol(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, string, error) {
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
		loginProtocolEngineParam:   "n8n",
		loginProtocolEmailParam:    emailx.Normalize(account.GetAccount().GetEmail()),
		"target_connectivity_urls": "https://api.openai.com/v1/models",
	}
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionLoginSessionProtocol, params); err != nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.LoginAccountResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) N8NLoginSessionProtocolDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	return s.n8nDynamicProxySettings(ctx, jobID, accountID, n8nExecutionID, actionLoginSessionProtocol, s.bindN8NLoginProtocolExecution)
}

func (s *Server) RecordN8NLoginSessionProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error) {
	return s.recordN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, proxyURL, data, actionLoginSessionProtocol, s.bindN8NLoginProtocolExecution)
}

func (s *Server) FailN8NLoginSessionProtocolDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	return s.failN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, errorMessage, data, s.bindN8NLoginProtocolExecution)
}

func (s *Server) UseN8NLoginSessionProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolUseProxyActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, protocolLoginMode))
	result := n8nProtocolStepResult(jobID, accountID, n8nExecutionID, stepProtocolUseProxy, out.GetData())
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProtocolUseProxy, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	result.Success = true
	return result, nil
}

func (s *Server) StartN8NLoginSessionProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthStartActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, protocolLoginMode))
	result, resultErr := s.n8nLoginProtocolProgressResult(ctx, jobID, accountID, n8nExecutionID, stepLoginSessionProtocolStart, &out)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepLoginSessionProtocolStart, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) WaitN8NLoginSessionProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthWaitActivity(ctx, pb.ProtocolAuthWaitInput{
		JobId:     jobID,
		AccountId: accountID,
		FlowId:    strings.TrimSpace(flowID),
		Mode:      protocolLoginMode,
		Email:     strings.TrimSpace(email),
		ProxyUrl:  s.protocolProxyURL(ctx, jobID),
	})
	result, resultErr := s.n8nLoginProtocolProgressResult(ctx, jobID, accountID, n8nExecutionID, stepLoginSessionProtocol, &out)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepLoginSessionProtocol, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) AwaitN8NLoginSessionProtocolOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	flowID = strings.TrimSpace(flowID)
	email = emailx.Normalize(email)
	resumeURL = strings.TrimSpace(resumeURL)
	if jobID == "" || accountID == "" || flowID == "" || email == "" || resumeURL == "" {
		return nil, fmt.Errorf("job_id, account_id, flow_id, email and resume_url are required")
	}
	if s.emailOTPWaits == nil {
		return nil, fmt.Errorf("protocol otp wait store is required")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	secretKey := n8nLoginProtocolResumeURLSecretPrefix + jobID
	ttl := time.Duration(timeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, resumeURL, ttl); err != nil {
		return nil, err
	}
	entry := emailotpwait.Entry{
		JobID:           jobID,
		AccountID:       accountID,
		FlowID:          flowID,
		Email:           email,
		StepName:        stepLoginSessionProtocolOTPWait,
		IssuedAfterUnix: otpIssuedAfterUnix,
		TimeoutSeconds:  timeoutSeconds,
		ResumeSecretKey: secretKey,
		N8NExecutionID:  n8nExecutionID,
	}
	if err := s.emailOTPWaits.Register(ctx, entry, ttl); err != nil {
		return nil, err
	}
	detail := map[string]any{
		"account_id":             accountID,
		"flow_id":                flowID,
		"email":                  email,
		"timeout_seconds":        timeoutSeconds,
		"otp_issued_after_unix":  otpIssuedAfterUnix,
		"resume_url_registered":  true,
		"mailbox_poll_requested": true,
		"n8n_execution_id":       n8nExecutionID,
		"wait_index":             "redis_ttl",
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{
		JobId:       jobID,
		StepName:    stepLoginSessionProtocolOTPWait,
		Recoverable: false,
		Retryable:   true,
		Detail:      structData(detail),
	}); err != nil {
		return nil, err
	}
	params := map[string]string{
		loginProtocolFlowIDParam:             flowID,
		loginProtocolEmailParam:              email,
		loginProtocolOTPIssuedAfterParam:     fmt.Sprintf("%d", otpIssuedAfterUnix),
		loginProtocolOTPTimeoutParam:         fmt.Sprintf("%d", timeoutSeconds),
		loginProtocolOTPResumeSecretKeyParam: secretKey,
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
		return &n8nRegisterProtocolStepResult{
			JobID:              jobID,
			AccountID:          accountID,
			N8NExecutionID:     n8nExecutionID,
			Step:               stepLoginSessionProtocolOTPWait,
			FlowID:             flowID,
			Email:              email,
			OTPRequired:        true,
			OTPIssuedAfterUnix: otpIssuedAfterUnix,
			OTPTimeoutSeconds:  timeoutSeconds,
			OTPSource:          "mailbox",
			MessageID:          strings.TrimSpace(message.GetId()),
			Success:            true,
			Data:               detail,
		}, nil
	}
	if s.mailboxPollRequester == nil {
		return nil, fmt.Errorf("mailbox poll requester is required")
	}
	if err := s.mailboxPollRequester.RequestMailboxEmailPoll(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED, otpIssuedAfterUnix, time.Duration(timeoutSeconds)*time.Second, "gpt_login_session_protocol_otp"); err != nil {
		return nil, err
	}
	return &n8nRegisterProtocolStepResult{
		JobID:              jobID,
		AccountID:          accountID,
		N8NExecutionID:     n8nExecutionID,
		Step:               stepLoginSessionProtocolOTPWait,
		FlowID:             flowID,
		Email:              email,
		OTPRequired:        true,
		OTPIssuedAfterUnix: otpIssuedAfterUnix,
		OTPTimeoutSeconds:  timeoutSeconds,
		Waiting:            true,
		Success:            true,
		Data:               detail,
	}, nil
}

func (s *Server) CompleteN8NLoginSessionProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, otpSource string, otpIssuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthCompleteActivity(ctx, pb.ProtocolAuthCompleteInput{
		JobId:              jobID,
		AccountId:          accountID,
		FlowId:             strings.TrimSpace(flowID),
		Mode:               protocolLoginMode,
		OtpParam:           registrationOTPParam,
		SubmittedAtParam:   registrationOTPSubmittedAtParam,
		OtpIssuedAfterUnix: otpIssuedAfterUnix,
		OtpSource:          strings.TrimSpace(otpSource),
		ProxyUrl:           s.protocolProxyURL(ctx, jobID),
	})
	result, resultErr := s.n8nLoginProtocolProgressResult(ctx, jobID, accountID, n8nExecutionID, stepLoginSessionProtocolComplete, &registerOutputProgress{accountID: accountID, result: &out, data: out.GetData()})
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepLoginSessionProtocolComplete, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) FinishN8NLoginSessionProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nLoginProtocolResultSecretPrefix + jobID
	}
	login, err := s.loadN8NLoginProtocolResult(ctx, jobID, resultRef)
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepLoginSessionProtocolComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	if login == nil || strings.TrimSpace(login.GetAccessToken()) == "" {
		err := fmt.Errorf("protocol login did not return ChatGPT access token")
		return nil, s.markActionFailed(ctx, jobID, stepLoginSessionProtocolComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	result := map[string]any{
		"account_id":            accountID,
		"driver":                "protocol",
		"mode":                  protocolLoginMode,
		"n8n_execution_id":      n8nExecutionID,
		"session_token_present": strings.TrimSpace(login.GetSessionToken()) != "",
		"access_token_present":  strings.TrimSpace(login.GetAccessToken()) != "",
	}
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{AccountId: accountID, SessionToken: login.GetSessionToken(), AccessToken: login.GetAccessToken()}); err != nil {
		return nil, s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, result)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteRuntimeSecretValue(ctx, resultRef)
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionLoginSessionProtocol, Started: true, Success: true, Result: result}, nil
}

func (s *Server) FailN8NLoginSessionProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NLoginProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NLoginProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "login session protocol failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	step := s.loginProtocolFailureStep(ctx, jobID)
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, err, data); markErr != nil {
		return nil, markErr
	}
	_ = s.cancelLoginProtocolState(ctx, jobID, data)
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionLoginSessionProtocol, Started: true, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) n8nLoginProtocolProgressResult(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, progress protocolAuthProgress) (*n8nRegisterProtocolStepResult, error) {
	result := &n8nRegisterProtocolStepResult{
		JobID:              jobID,
		AccountID:          accountID,
		N8NExecutionID:     n8nExecutionID,
		Step:               step,
		FlowID:             strings.TrimSpace(progress.GetFlowId()),
		Email:              emailx.Normalize(progress.GetEmail()),
		OTPRequired:        progress.GetOtpRequired(),
		OTPIssuedAfterUnix: progress.GetOtpIssuedAfterUnix(),
		OTPTimeoutSeconds:  progress.GetOtpTimeoutSeconds(),
		Success:            true,
		Data:               structMap(progress.GetData()),
	}
	if result.AccountID == "" {
		result.AccountID = strings.TrimSpace(progress.GetAccountId())
	}
	if login := progress.GetResult(); login != nil {
		ref, err := s.saveN8NLoginProtocolResult(ctx, jobID, login)
		if err != nil {
			return result, err
		}
		result.ResultReady = true
		result.ResultRef = ref
	}
	return result, nil
}

func (s *Server) saveN8NLoginProtocolResult(ctx context.Context, jobID string, result *pb.RegisterActivityOutput) (string, error) {
	if result == nil {
		return "", nil
	}
	key := n8nLoginProtocolResultSecretPrefix + strings.TrimSpace(jobID)
	data, err := protojsonx.Marshal(result)
	if err != nil {
		return "", err
	}
	if err := s.saveRuntimeSecretValueTTL(ctx, key, string(data), s.runtimeSecretTTL()); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) loadN8NLoginProtocolResult(ctx context.Context, jobID string, resultRef string) (*pb.RegisterActivityOutput, error) {
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nLoginProtocolResultSecretPrefix + strings.TrimSpace(jobID)
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

func (s *Server) bindN8NLoginProtocolExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) loginProtocolFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepLoginSessionProtocolStart
}

func (s *Server) cancelLoginProtocolState(ctx context.Context, jobID string, data map[string]any) error {
	flowID := registerProtocolFlowID(data)
	if flowID == "" {
		params, err := s.jobStore.Params(ctx, jobID)
		if err == nil {
			flowID = strings.TrimSpace(params[loginProtocolFlowIDParam])
		}
	}
	if flowID == "" {
		return nil
	}
	return s.activities.ProtocolAuthCancelActivity(ctx, pb.ProtocolAuthCancelInput{JobId: jobID, FlowId: flowID, Mode: protocolLoginMode})
}

func normalizeN8NLoginProtocolIDs(jobID string, accountID string, n8nExecutionID string) (string, string, string) {
	return strings.TrimSpace(jobID), strings.TrimSpace(accountID), strings.TrimSpace(n8nExecutionID)
}
