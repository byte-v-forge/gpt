package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/manualinput"
	"orchestrator/internal/paymentotpwait"
	"orchestrator/pb"
)

const (
	paymentOTPResumeSecretPrefix = "n8n:payment-otp-resume-url:"
	paymentOTPResumedAtParam     = "payment_otp_resumed_at_unix"
	paymentOTPDefaultPurpose     = "gopay"
)

type n8nPaymentOTPWaitRequest struct {
	Action           string
	JobID            string
	AccountID        string
	N8NExecutionID   string
	Operation        string
	UserID           string
	Source           string
	Purpose          string
	StepName         string
	TimeoutSeconds   int32
	OTPIssuedAfter   int64
	ResumeURL        string
	OTPParam         string
	SubmittedAtParam string
}

type n8nPaymentOTPReceiveRequest struct {
	Source         string
	Purpose        string
	OTP            string
	OTPSource      string
	ReceivedAtUnix int64
}

type n8nPaymentOTPWaitResult struct {
	JobID              string         `json:"job_id,omitempty"`
	AccountID          string         `json:"account_id,omitempty"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Action             string         `json:"action,omitempty"`
	Operation          string         `json:"operation,omitempty"`
	UserID             string         `json:"user_id,omitempty"`
	Step               string         `json:"step,omitempty"`
	Source             string         `json:"source,omitempty"`
	Purpose            string         `json:"purpose,omitempty"`
	OTPRequired        bool           `json:"otp_required"`
	OTPFound           bool           `json:"otp_found,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	Waiting            bool           `json:"waiting,omitempty"`
	Success            bool           `json:"success"`
	ResumedCount       int            `json:"resumed_count,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

func (s *Server) AwaitN8NPaymentOTP(ctx context.Context, req n8nPaymentOTPWaitRequest) (*n8nPaymentOTPWaitResult, error) {
	req = normalizeN8NPaymentOTPWaitRequest(req)
	if req.JobID == "" || req.Source == "" || req.Purpose == "" || req.ResumeURL == "" || req.StepName == "" {
		return nil, fmt.Errorf("job_id, source, purpose, resume_url and step_name are required")
	}
	if s.paymentOTPWaits == nil {
		return nil, fmt.Errorf("payment otp wait store is required")
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 300
	}
	queueKey, err := paymentotpwait.QueueKey(req.Source, req.Purpose)
	if err != nil {
		return nil, err
	}
	if err := s.bindN8NGoPayExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	secretKey := paymentOTPResumeSecretPrefix + req.JobID
	ttl := time.Duration(req.TimeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, req.ResumeURL, ttl); err != nil {
		return nil, err
	}
	entry := paymentotpwait.Entry{
		JobID:            req.JobID,
		AccountID:        req.AccountID,
		Action:           req.Action,
		Operation:        req.Operation,
		UserID:           req.UserID,
		Source:           req.Source,
		Purpose:          req.Purpose,
		QueueKey:         queueKey,
		StepName:         req.StepName,
		IssuedAfterUnix:  req.OTPIssuedAfter,
		TimeoutSeconds:   req.TimeoutSeconds,
		ResumeSecretKey:  secretKey,
		N8NExecutionID:   req.N8NExecutionID,
		OTPParam:         req.OTPParam,
		SubmittedAtParam: req.SubmittedAtParam,
	}
	if err := s.paymentOTPWaits.Register(ctx, entry, ttl); err != nil {
		return nil, err
	}
	detail := map[string]any{
		"action":                req.Action,
		"operation":             req.Operation,
		"user_id":               req.UserID,
		"account_id":            req.AccountID,
		"source":                req.Source,
		"purpose":               req.Purpose,
		"queue_key":             queueKey,
		"timeout_seconds":       req.TimeoutSeconds,
		"otp_issued_after_unix": req.OTPIssuedAfter,
		"resume_url_registered": true,
		"n8n_execution_id":      req.N8NExecutionID,
		"wait_index":            "redis_ttl",
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: req.JobID, StepName: req.StepName, Recoverable: false, Retryable: true, Detail: structData(detail)}); err != nil {
		return nil, err
	}
	params := map[string]string{
		"payment_otp_source":       req.Source,
		"payment_otp_purpose":      req.Purpose,
		"payment_otp_issued_after": strconv.FormatInt(req.OTPIssuedAfter, 10),
		"payment_otp_timeout":      strconv.FormatInt(int64(req.TimeoutSeconds), 10),
	}
	if req.N8NExecutionID != "" {
		params["n8n_execution_id"] = req.N8NExecutionID
	}
	if err := s.setJobParams(ctx, req.JobID, params); err != nil {
		return nil, err
	}
	if record, ok, err := s.paymentOTPWaits.LatestCode(ctx, queueKey, req.OTPIssuedAfter); err != nil {
		return nil, err
	} else if ok {
		otpData, err := s.completePaymentOTPWait(ctx, entry, record.Code, record.ReceivedAtUnix, record.OTPSource)
		if err != nil {
			return nil, err
		}
		if err := s.markPaymentOTPResolved(ctx, req.JobID, time.Now().Unix()); err != nil {
			return nil, err
		}
		_ = s.paymentOTPWaits.Delete(ctx, entry)
		for key, value := range otpData {
			detail[key] = value
		}
		return s.paymentOTPWaitResult(req, true, true, detail), nil
	}
	return s.paymentOTPWaitResult(req, false, true, detail), nil
}

func (s *Server) ReceiveN8NPaymentOTP(ctx context.Context, req n8nPaymentOTPReceiveRequest) (*n8nPaymentOTPWaitResult, error) {
	if s.paymentOTPWaits == nil {
		return nil, fmt.Errorf("payment otp wait store is required")
	}
	req = normalizeN8NPaymentOTPReceiveRequest(req)
	if req.Source == "" || req.Purpose == "" || normalizeOTP(req.OTP) == "" {
		return nil, fmt.Errorf("source, purpose and otp are required")
	}
	queueKey, err := paymentotpwait.QueueKey(req.Source, req.Purpose)
	if err != nil {
		return nil, err
	}
	if req.ReceivedAtUnix <= 0 {
		req.ReceivedAtUnix = time.Now().Unix()
	}
	if err := s.paymentOTPWaits.RecordCode(ctx, paymentotpwait.CodeRecord{Source: req.Source, OTPSource: req.OTPSource, Purpose: req.Purpose, QueueKey: queueKey, Code: req.OTP, ReceivedAtUnix: req.ReceivedAtUnix}, 5*time.Minute); err != nil {
		return nil, err
	}
	pending, err := s.paymentOTPWaits.PendingForQueue(ctx, queueKey, req.ReceivedAtUnix)
	if err != nil || len(pending) == 0 {
		return &n8nPaymentOTPWaitResult{Source: req.Source, Purpose: req.Purpose, Success: err == nil, ResumedCount: 0}, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resumed := 0
	for _, job := range pending {
		claimed, err := s.paymentOTPWaits.Claim(ctx, job.JobID, paymentOTPClaimTTL(job.TimeoutSeconds))
		if err != nil {
			return nil, err
		}
		if !claimed {
			continue
		}
		if err := s.resumePaymentOTPJob(ctx, client, job, req.OTP, req.ReceivedAtUnix, req.OTPSource); err != nil {
			_ = s.paymentOTPWaits.ReleaseClaim(ctx, job.JobID)
			return nil, err
		}
		resumed++
	}
	return &n8nPaymentOTPWaitResult{Source: req.Source, Purpose: req.Purpose, Success: true, ResumedCount: resumed}, nil
}

func (s *Server) ResumeN8NPaymentManualOTP(ctx context.Context, client *http.Client, jobID string) error {
	if s.paymentOTPWaits == nil {
		return nil
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}
	job, ok, err := s.paymentOTPWaits.Get(ctx, jobID)
	if err != nil || !ok {
		return err
	}
	otpParam := firstNonEmpty(job.OTPParam, paymentOTPParam)
	submittedAtParam := firstNonEmpty(job.SubmittedAtParam, paymentOTPSubmittedAtParam)
	if !manualinput.SubmittedAfter(ctx, s.jobStore, job.JobID, otpParam, submittedAtParam, job.IssuedAfterUnix) {
		return fmt.Errorf("payment otp is stale")
	}
	params, err := s.jobStore.Params(ctx, job.JobID)
	if err != nil {
		return err
	}
	code := normalizeOTP(params[otpParam])
	if code == "" {
		return fmt.Errorf("payment otp is empty")
	}
	claimed, err := s.paymentOTPWaits.Claim(ctx, job.JobID, paymentOTPClaimTTL(job.TimeoutSeconds))
	if err != nil || !claimed {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if err := s.resumePaymentOTPJob(ctx, client, job, code, time.Now().Unix(), "manual"); err != nil {
		_ = s.paymentOTPWaits.ReleaseClaim(ctx, job.JobID)
		return err
	}
	return nil
}

func (s *Server) resumePaymentOTPJob(ctx context.Context, client *http.Client, job paymentotpwait.Entry, code string, receivedAt int64, source string) error {
	resumeURL, err := s.runtimeSecretValue(ctx, job.ResumeSecretKey)
	if err != nil {
		return err
	}
	if _, err := s.completePaymentOTPWait(ctx, job, code, receivedAt, source); err != nil {
		return err
	}
	if err := postN8NResume(ctx, client, resumeURL, map[string]any{
		"job_id":                job.JobID,
		"account_id":            job.AccountID,
		"operation":             job.Operation,
		"user_id":               job.UserID,
		"source":                job.Source,
		"purpose":               job.Purpose,
		"otp_source":            firstNonEmpty(source, "whatsapp"),
		"otp_issued_after_unix": job.IssuedAfterUnix,
		"n8n_execution_id":      strings.TrimSpace(job.N8NExecutionID),
	}); err != nil {
		return err
	}
	if err := s.markPaymentOTPResolved(ctx, job.JobID, time.Now().Unix()); err != nil {
		return err
	}
	return s.paymentOTPWaits.Delete(ctx, job)
}

func (s *Server) completePaymentOTPWait(ctx context.Context, job paymentotpwait.Entry, code string, receivedAt int64, source string) (map[string]any, error) {
	code = normalizeOTP(code)
	if code == "" {
		return nil, fmt.Errorf("payment otp code is required")
	}
	now := time.Now().Unix()
	timeoutSeconds := job.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	otpParam := firstNonEmpty(job.OTPParam, paymentOTPParam)
	submittedAtParam := firstNonEmpty(job.SubmittedAtParam, paymentOTPSubmittedAtParam)
	otpSource := firstNonEmpty(source, "whatsapp")
	otpData := map[string]any{
		"channel":               "payment",
		"found":                 true,
		"source":                otpSource,
		"purpose":               job.Purpose,
		"queue_key":             job.QueueKey,
		"timeout_seconds":       timeoutSeconds,
		"issued_after_unix":     job.IssuedAfterUnix,
		"received_at_unix":      receivedAt,
		"n8n_execution_id":      strings.TrimSpace(job.N8NExecutionID),
		"resume_event_received": true,
	}
	if err := s.setJobParams(ctx, job.JobID, map[string]string{otpParam: code, submittedAtParam: strconv.FormatInt(now, 10)}); err != nil {
		return nil, err
	}
	if err := s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{JobId: job.JobID, StepName: job.StepName, Recoverable: false, Retryable: true, Result: structData(otpData)}); err != nil {
		return nil, err
	}
	return otpData, nil
}

func (s *Server) markPaymentOTPResolved(ctx context.Context, jobID string, atUnix int64) error {
	if atUnix <= 0 {
		atUnix = time.Now().Unix()
	}
	return s.setJobParams(ctx, jobID, map[string]string{paymentOTPResumedAtParam: strconv.FormatInt(atUnix, 10)})
}

func (s *Server) paymentOTPWaitResult(req n8nPaymentOTPWaitRequest, found bool, success bool, data map[string]any) *n8nPaymentOTPWaitResult {
	return &n8nPaymentOTPWaitResult{JobID: req.JobID, AccountID: req.AccountID, N8NExecutionID: req.N8NExecutionID, Action: req.Action, Operation: req.Operation, UserID: req.UserID, Step: req.StepName, Source: req.Source, Purpose: req.Purpose, OTPRequired: true, OTPFound: found, OTPSource: paymentOTPSource(found), OTPIssuedAfterUnix: req.OTPIssuedAfter, OTPTimeoutSeconds: req.TimeoutSeconds, Waiting: !found, Success: success, Data: data}
}

func paymentOTPSource(found bool) string {
	if found {
		return "whatsapp"
	}
	return ""
}

func normalizeN8NPaymentOTPWaitRequest(req n8nPaymentOTPWaitRequest) n8nPaymentOTPWaitRequest {
	req.Action = strings.ToUpper(strings.TrimSpace(req.Action))
	req.JobID = strings.TrimSpace(req.JobID)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.N8NExecutionID = strings.TrimSpace(req.N8NExecutionID)
	req.Operation = strings.TrimSpace(req.Operation)
	req.UserID = strings.TrimSpace(req.UserID)
	req.Source = firstNonEmpty(req.Source, req.UserID, "local")
	if source, err := paymentotpwait.NormalizeSource(req.Source); err == nil {
		req.Source = source
	}
	req.Purpose = firstNonEmpty(req.Purpose, paymentOTPDefaultPurpose)
	req.StepName = strings.TrimSpace(req.StepName)
	req.ResumeURL = strings.TrimSpace(req.ResumeURL)
	req.OTPParam = strings.TrimSpace(req.OTPParam)
	req.SubmittedAtParam = strings.TrimSpace(req.SubmittedAtParam)
	return req
}

func normalizeN8NPaymentOTPReceiveRequest(req n8nPaymentOTPReceiveRequest) n8nPaymentOTPReceiveRequest {
	if source, err := paymentotpwait.NormalizeSource(req.Source); err == nil {
		req.Source = source
	} else {
		req.Source = strings.TrimSpace(req.Source)
	}
	req.Purpose = firstNonEmpty(req.Purpose, paymentOTPDefaultPurpose)
	req.OTP = normalizeOTP(req.OTP)
	req.OTPSource = firstNonEmpty(req.OTPSource, "whatsapp")
	return req
}

func paymentOTPClaimTTL(timeoutSeconds int32) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return time.Duration(timeoutSeconds)*time.Second + time.Hour
}
