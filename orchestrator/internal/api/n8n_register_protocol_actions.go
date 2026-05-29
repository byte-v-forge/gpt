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
	"google.golang.org/protobuf/types/known/structpb"

	"orchestrator/internal/accountmail"
	"orchestrator/internal/emailotpwait"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	n8nRegisterProtocolResultSecretPrefix    = "n8n-register-protocol-result:"
	n8nRegisterProtocolResumeURLSecretPrefix = "n8n-register-protocol-resume-url:"

	registerProtocolEngineParam             = "engine"
	registerProtocolFlowIDParam             = "register_protocol_flow_id"
	registerProtocolEmailParam              = "register_protocol_email"
	registerProtocolOTPIssuedAfterParam     = "register_protocol_otp_issued_after_unix"
	registerProtocolOTPTimeoutParam         = "register_protocol_otp_timeout_seconds"
	registerProtocolOTPResumeSecretKeyParam = "register_protocol_otp_resume_url_secret_key"
)

type n8nRegisterProtocolStepResult struct {
	JobID              string         `json:"job_id"`
	AccountID          string         `json:"account_id"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Step               string         `json:"step"`
	FlowID             string         `json:"flow_id,omitempty"`
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

type n8nRegisterProtocolCompleteResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Action         string         `json:"action"`
	Started        bool           `json:"started"`
	Success        bool           `json:"success"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
}

type protocolAuthProgress interface {
	GetAccountId() string
	GetFlowId() string
	GetEmail() string
	GetOtpRequired() bool
	GetOtpIssuedAfterUnix() int64
	GetOtpTimeoutSeconds() int32
	GetResult() *pb.RegisterActivityOutput
	GetData() *structpb.Struct
}

func (s *Server) StartN8NRegisterProtocol(ctx context.Context, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, string, error) {
	jobID := uuid.NewString()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	if s.activities == nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: "GPT action API is not configured"}, "", fmt.Errorf("GPT action API is not configured")
	}
	account, err := s.activities.EnsureAccountActivity(ctx, pb.EnsureAccountInput{Account: &pb.AccountSpec{
		AccountId:     accountID,
		Email:         req.GetEmail(),
		Password:      req.GetPassword(),
		EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()),
		CountryCode:   req.GetCountryCode(),
		Region:        req.GetRegion(),
	}})
	if err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, "", err
	}
	accountID = account.GetAccountId()
	accountRecord, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	params := map[string]string{
		"account_id":                accountID,
		registerProtocolEngineParam: "n8n",
		registerProtocolEmailParam:  emailx.Normalize(accountRecord.GetAccount().GetEmail()),
	}
	putProtocolGeoParams(params, req.GetCountryCode(), req.GetRegion())
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionRegisterProtocol, params); err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, accountID, err
	}
	return &pb.RegisterAccountResponse{JobId: jobID, Started: true}, accountID, nil
}

func (s *Server) UseN8NRegisterProtocolProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolUseProxyActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, protocolRegisterMode))
	result := n8nProtocolStepResult(jobID, accountID, n8nExecutionID, stepProtocolUseProxy, out.GetData())
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepProtocolUseProxy, jobstatus.FailedRetryable, false, true, err, structMap(out.GetData()))
	}
	result.Success = true
	return result, nil
}

func (s *Server) StartN8NRegisterProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthStartActivity(ctx, s.protocolAuthStartInput(ctx, jobID, accountID, protocolRegisterMode))
	result, resultErr := s.n8nRegisterProtocolProgressResult(ctx, jobID, accountID, n8nExecutionID, stepRegisterAccountProtocolStart, &out)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountProtocolStart, registerProtocolFailureStatus(err), false, registerProtocolRetryable(err), err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) WaitN8NRegisterProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthWaitActivity(ctx, pb.ProtocolAuthWaitInput{
		JobId:     jobID,
		AccountId: accountID,
		FlowId:    strings.TrimSpace(flowID),
		Mode:      protocolRegisterMode,
		Email:     strings.TrimSpace(email),
		ProxyUrl:  s.protocolProxyURL(ctx, jobID),
	})
	result, resultErr := s.n8nRegisterProtocolProgressResult(ctx, jobID, accountID, n8nExecutionID, stepRegisterAccountProtocol, &out)
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountProtocol, registerProtocolFailureStatus(err), false, registerProtocolRetryable(err), err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) AwaitN8NRegisterProtocolOTP(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, email string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	flowID = strings.TrimSpace(flowID)
	email = emailx.Normalize(email)
	resumeURL = strings.TrimSpace(resumeURL)
	if jobID == "" || accountID == "" || flowID == "" || email == "" || resumeURL == "" {
		return nil, fmt.Errorf("job_id, account_id, flow_id, email and resume_url are required")
	}
	if s.emailOTPWaits == nil {
		return nil, fmt.Errorf("email otp wait store is required")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	secretKey := n8nRegisterProtocolResumeURLSecretPrefix + jobID
	ttl := time.Duration(timeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, resumeURL, ttl); err != nil {
		return nil, err
	}
	entry := emailotpwait.Entry{
		JobID:           jobID,
		AccountID:       accountID,
		FlowID:          flowID,
		Email:           email,
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
		StepName:    stepRegisterAccountProtocolOTPWait,
		Recoverable: false,
		Retryable:   true,
		Detail:      structData(detail),
	}); err != nil {
		return nil, err
	}
	params := map[string]string{
		registerProtocolFlowIDParam:             flowID,
		registerProtocolEmailParam:              email,
		registerProtocolOTPIssuedAfterParam:     fmt.Sprintf("%d", otpIssuedAfterUnix),
		registerProtocolOTPTimeoutParam:         fmt.Sprintf("%d", timeoutSeconds),
		registerProtocolOTPResumeSecretKeyParam: secretKey,
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
			Step:               stepRegisterAccountProtocolOTPWait,
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
	if err := s.mailboxPollRequester.RequestMailboxEmailPoll(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED, otpIssuedAfterUnix, time.Duration(timeoutSeconds)*time.Second, "gpt_register_protocol_otp"); err != nil {
		return nil, err
	}
	return &n8nRegisterProtocolStepResult{
		JobID:              jobID,
		AccountID:          accountID,
		N8NExecutionID:     n8nExecutionID,
		Step:               stepRegisterAccountProtocolOTPWait,
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

func (s *Server) latestMailboxEmailOTP(ctx context.Context, email string, issuedAfterUnix int64) (*mailboxv1.EmailInboxMessage, string, bool, error) {
	if s == nil || s.otpProjection == nil {
		return nil, "", false, nil
	}
	message, code, ok, err := s.otpProjection.LatestMailboxSignal(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP, issuedAfterUnix)
	if err != nil || !ok {
		return message, normalizeOTP(code), ok, err
	}
	message = accountmail.EnrichMessage(message)
	code = normalizeOTP(code)
	if code == "" {
		code = normalizeOTP(accountmail.OTPCode(message))
	}
	return message, code, code != "", nil
}

func (s *Server) CompleteN8NRegisterProtocolAuth(ctx context.Context, jobID string, accountID string, n8nExecutionID string, flowID string, otpSource string, otpIssuedAfterUnix int64) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	out, err := s.activities.ProtocolAuthCompleteActivity(ctx, pb.ProtocolAuthCompleteInput{
		JobId:              jobID,
		AccountId:          accountID,
		FlowId:             strings.TrimSpace(flowID),
		Mode:               protocolRegisterMode,
		OtpParam:           registrationOTPParam,
		SubmittedAtParam:   registrationOTPSubmittedAtParam,
		OtpIssuedAfterUnix: otpIssuedAfterUnix,
		OtpSource:          strings.TrimSpace(otpSource),
		ProxyUrl:           s.protocolProxyURL(ctx, jobID),
	})
	result, resultErr := s.n8nRegisterProtocolProgressResult(ctx, jobID, accountID, n8nExecutionID, stepRegisterAccountProtocolComplete, &registerOutputProgress{accountID: accountID, result: &out, data: out.GetData()})
	if err != nil {
		return result, s.markActionFailed(ctx, jobID, stepRegisterAccountProtocolComplete, registerProtocolFailureStatus(err), false, registerProtocolRetryable(err), err, structMap(out.GetData()))
	}
	if resultErr != nil {
		return result, resultErr
	}
	return result, nil
}

func (s *Server) FinishN8NRegisterProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, resultRef string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nRegisterProtocolResultSecretPrefix + jobID
	}
	register, err := s.loadN8NRegisterProtocolResult(ctx, jobID, resultRef)
	if err != nil {
		return nil, s.markActionFailed(ctx, jobID, stepRegisterAccountProtocolComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	if register == nil || strings.TrimSpace(register.GetAccessToken()) == "" {
		err := fmt.Errorf("protocol register did not return ChatGPT access token")
		return nil, s.markActionFailed(ctx, jobID, stepRegisterAccountProtocolComplete, jobstatus.FailedRetryable, false, true, err, nil)
	}
	result := map[string]any{
		"account_id":            accountID,
		"driver":                "protocol",
		"mode":                  protocolRegisterMode,
		"n8n_execution_id":      n8nExecutionID,
		"session_token_present": strings.TrimSpace(register.GetSessionToken()) != "",
		"access_token_present":  strings.TrimSpace(register.GetAccessToken()) != "",
		"plus_trial_checked":    register.GetPlusTrialChecked(),
		"plus_trial_eligible":   register.GetPlusTrialEligible(),
	}
	if err := s.activities.PersistRegisteredActivity(ctx, pb.PersistRegisteredInput{
		AccountId:         accountID,
		SessionToken:      register.GetSessionToken(),
		AccessToken:       register.GetAccessToken(),
		PlusTrialEligible: register.GetPlusTrialEligible(),
		PlusTrialChecked:  register.GetPlusTrialChecked(),
	}); err != nil {
		return nil, s.markActionFailed(ctx, jobID, "persist_registered", jobstatus.FailedRecoverable, true, false, err, result)
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result)}); err != nil {
		return nil, err
	}
	s.deleteRuntimeSecretValue(ctx, resultRef)
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionRegisterProtocol, Started: true, Success: true, Result: result}, nil
}

func (s *Server) FailN8NRegisterProtocol(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NRegisterProtocolExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "register protocol failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	data["account_id"] = accountID
	data["n8n_execution_id"] = n8nExecutionID
	step := s.registerProtocolFailureStep(ctx, jobID)
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, step, jobstatus.FailedRetryable, false, true, err, data); markErr != nil {
		return nil, markErr
	}
	_ = s.cancelRegisterProtocolState(ctx, jobID, data)
	return &n8nRegisterProtocolCompleteResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Action: actionRegisterProtocol, Started: true, Success: false, ErrorMessage: errorMessage, Result: data}, nil
}

func (s *Server) n8nRegisterProtocolProgressResult(ctx context.Context, jobID string, accountID string, n8nExecutionID string, step string, progress protocolAuthProgress) (*n8nRegisterProtocolStepResult, error) {
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
	if register := progress.GetResult(); register != nil {
		ref, err := s.saveN8NRegisterProtocolResult(ctx, jobID, register)
		if err != nil {
			return result, err
		}
		result.ResultReady = true
		result.ResultRef = ref
	}
	return result, nil
}

func n8nProtocolStepResult(jobID string, accountID string, n8nExecutionID string, step string, data *structpb.Struct) *n8nRegisterProtocolStepResult {
	return &n8nRegisterProtocolStepResult{
		JobID:          strings.TrimSpace(jobID),
		AccountID:      strings.TrimSpace(accountID),
		N8NExecutionID: strings.TrimSpace(n8nExecutionID),
		Step:           step,
		Data:           structMap(data),
	}
}

func (s *Server) saveN8NRegisterProtocolResult(ctx context.Context, jobID string, result *pb.RegisterActivityOutput) (string, error) {
	if result == nil {
		return "", nil
	}
	key := n8nRegisterProtocolResultSecretPrefix + strings.TrimSpace(jobID)
	data, err := protojsonx.Marshal(result)
	if err != nil {
		return "", err
	}
	ttl := s.runtimeSecretTTL()
	if err := s.saveRuntimeSecretValueTTL(ctx, key, string(data), ttl); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) loadN8NRegisterProtocolResult(ctx context.Context, jobID string, resultRef string) (*pb.RegisterActivityOutput, error) {
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = n8nRegisterProtocolResultSecretPrefix + strings.TrimSpace(jobID)
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

func (s *Server) runtimeSecretTTL() time.Duration {
	if s == nil || s.runtimeSecrets == nil || s.runtimeSecrets.DefaultTTL() <= 0 {
		return time.Hour
	}
	return s.runtimeSecrets.DefaultTTL()
}

func (s *Server) bindN8NRegisterProtocolExecution(ctx context.Context, jobID string, executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil
	}
	return s.jobStore.BindN8NExecution(ctx, jobID, executionID)
}

func (s *Server) registerProtocolFailureStep(ctx context.Context, jobID string) string {
	job, err := s.getJob(ctx, jobID)
	if err == nil && strings.TrimSpace(job.LastStep) != "" {
		return strings.TrimSpace(job.LastStep)
	}
	return stepRegisterAccountProtocolStart
}

func (s *Server) cancelRegisterProtocolState(ctx context.Context, jobID string, data map[string]any) error {
	flowID := registerProtocolFlowID(data)
	if flowID == "" {
		params, err := s.jobStore.Params(ctx, jobID)
		if err == nil {
			flowID = strings.TrimSpace(params[registerProtocolFlowIDParam])
		}
	}
	if flowID == "" {
		return nil
	}
	return s.activities.ProtocolAuthCancelActivity(ctx, pb.ProtocolAuthCancelInput{
		JobId:  jobID,
		FlowId: flowID,
		Mode:   protocolRegisterMode,
	})
}

func registerProtocolFlowID(data map[string]any) string {
	if data == nil {
		return ""
	}
	if value := strings.TrimSpace(fmt.Sprint(data["flow_id"])); value != "" && value != "<nil>" {
		return value
	}
	if nested, ok := data["data"].(map[string]any); ok {
		return registerProtocolFlowID(nested)
	}
	if body, ok := data["body"].(map[string]any); ok {
		return registerProtocolFlowID(body)
	}
	return ""
}

func normalizeN8NRegisterProtocolIDs(jobID string, accountID string, n8nExecutionID string) (string, string, string) {
	return strings.TrimSpace(jobID), strings.TrimSpace(accountID), strings.TrimSpace(n8nExecutionID)
}

type registerOutputProgress struct {
	accountID string
	result    *pb.RegisterActivityOutput
	data      *structpb.Struct
}

func (p *registerOutputProgress) GetAccountId() string                  { return strings.TrimSpace(p.accountID) }
func (p *registerOutputProgress) GetFlowId() string                     { return "" }
func (p *registerOutputProgress) GetEmail() string                      { return "" }
func (p *registerOutputProgress) GetOtpRequired() bool                  { return false }
func (p *registerOutputProgress) GetOtpIssuedAfterUnix() int64          { return 0 }
func (p *registerOutputProgress) GetOtpTimeoutSeconds() int32           { return 0 }
func (p *registerOutputProgress) GetResult() *pb.RegisterActivityOutput { return p.result }
func (p *registerOutputProgress) GetData() *structpb.Struct             { return p.data }
