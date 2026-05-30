package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"

	"orchestrator/internal/smsotpwait"
	"orchestrator/pb"
)

const (
	N8NSMSOTPResumeConsumerDurable = "gpt-sms-otp-resume"
	smsOTPResumeSecretPrefix       = "n8n:sms-otp-resume-url:"
	smsOTPResumedAtParam           = "sms_otp_resumed_at_unix"
)

type n8nSMSOTPResumeWorker struct {
	server *Server
	client *http.Client
}

type n8nSMSOTPWaitRequest struct {
	Action           string
	JobID            string
	AccountID        string
	N8NExecutionID   string
	Operation        string
	UserID           string
	ActivationID     string
	StepName         string
	TimeoutSeconds   int32
	OTPIssuedAfter   int64
	ResumeURL        string
	OTPParam         string
	SubmittedAtParam string
}

type n8nSMSOTPWaitResult struct {
	JobID              string         `json:"job_id"`
	AccountID          string         `json:"account_id,omitempty"`
	N8NExecutionID     string         `json:"n8n_execution_id,omitempty"`
	Action             string         `json:"action,omitempty"`
	Operation          string         `json:"operation,omitempty"`
	UserID             string         `json:"user_id,omitempty"`
	Step               string         `json:"step"`
	ActivationID       string         `json:"activation_id"`
	OTPRequired        bool           `json:"otp_required"`
	OTPFound           bool           `json:"otp_found,omitempty"`
	OTPSource          string         `json:"otp_source,omitempty"`
	OTPIssuedAfterUnix int64          `json:"otp_issued_after_unix,omitempty"`
	OTPTimeoutSeconds  int32          `json:"otp_timeout_seconds,omitempty"`
	Waiting            bool           `json:"waiting,omitempty"`
	Success            bool           `json:"success"`
	Data               map[string]any `json:"data,omitempty"`
}

func StartN8NSMSOTPResumeWorker(ctx context.Context, consumer eventbus.Consumer, server *Server) error {
	worker := &n8nSMSOTPResumeWorker{
		server: server,
		client: &http.Client{Timeout: 20 * time.Second},
	}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "GPT SMS OTP resume events",
		Consumer: consumer,
		Handler:  worker.handle,
		Logf:     logSMSOTPResumeWorker,
	})
}

func (w *n8nSMSOTPResumeWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	event := &smsv1.SmsCodeReceivedEvent{}
	if err := eventbus.UnmarshalPayload(message, event); err != nil {
		eventbus.TermMessage(ctx, message, "terminate malformed sms otp event", logSMSOTPResumeWorker)
		return
	}
	count, err := w.server.ResumeN8NSMSOTP(ctx, w.client, event)
	if err != nil {
		log.Printf("[orchestrator] resume SMS OTP failed event_id=%s: %v", eventbus.EventID(message), err)
		eventbus.NakMessageDelay(ctx, message, 15*time.Second, "retry SMS OTP resume", logSMSOTPResumeWorker)
		return
	}
	if count > 0 {
		log.Printf("[orchestrator] resumed SMS OTP wait count=%d event_id=%s", count, eventbus.EventID(message))
	}
	eventbus.AckMessage(ctx, message, "ack SMS OTP resume event", logSMSOTPResumeWorker)
}

func (s *Server) AwaitN8NSMSOTP(ctx context.Context, req n8nSMSOTPWaitRequest) (*n8nSMSOTPWaitResult, error) {
	req = normalizeN8NSMSOTPWaitRequest(req)
	if req.JobID == "" || req.ActivationID == "" || req.ResumeURL == "" || req.StepName == "" {
		return nil, fmt.Errorf("job_id, activation_id, resume_url and step_name are required")
	}
	if s.smsOTPWaits == nil {
		return nil, fmt.Errorf("sms otp wait store is required")
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 300
	}
	if err := s.bindN8NGoPayExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	secretKey := smsOTPResumeSecretPrefix + req.JobID
	ttl := time.Duration(req.TimeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, req.ResumeURL, ttl); err != nil {
		return nil, err
	}
	entry := smsotpwait.Entry{
		JobID:            req.JobID,
		AccountID:        req.AccountID,
		Action:           req.Action,
		Operation:        req.Operation,
		UserID:           req.UserID,
		ActivationID:     req.ActivationID,
		StepName:         req.StepName,
		IssuedAfterUnix:  req.OTPIssuedAfter,
		TimeoutSeconds:   req.TimeoutSeconds,
		ResumeSecretKey:  secretKey,
		N8NExecutionID:   req.N8NExecutionID,
		OTPParam:         req.OTPParam,
		SubmittedAtParam: req.SubmittedAtParam,
	}
	if err := s.smsOTPWaits.Register(ctx, entry, ttl); err != nil {
		return nil, err
	}
	detail := map[string]any{
		"action":                req.Action,
		"operation":             req.Operation,
		"user_id":               req.UserID,
		"account_id":            req.AccountID,
		"activation_id":         req.ActivationID,
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
		"sms_activation_id":       req.ActivationID,
		"sms_otp_issued_after":    strconv.FormatInt(req.OTPIssuedAfter, 10),
		"sms_otp_timeout_seconds": strconv.FormatInt(int64(req.TimeoutSeconds), 10),
	}
	if req.N8NExecutionID != "" {
		params["n8n_execution_id"] = req.N8NExecutionID
	}
	if err := s.setJobParams(ctx, req.JobID, params); err != nil {
		return nil, err
	}
	if code, ok, err := s.latestSMSOTP(ctx, req.ActivationID, req.OTPIssuedAfter); err != nil {
		return nil, err
	} else if ok {
		otpData, err := s.completeSMSOTPWait(ctx, entry, code, time.Now().Unix())
		if err != nil {
			return nil, err
		}
		if err := s.markSMSOTPResolved(ctx, req.JobID, time.Now().Unix()); err != nil {
			return nil, err
		}
		_ = s.smsOTPWaits.Delete(ctx, entry)
		for key, value := range otpData {
			detail[key] = value
		}
		return s.smsOTPWaitResult(req, true, true, detail), nil
	}
	return s.smsOTPWaitResult(req, false, true, detail), nil
}

func (s *Server) ResumeN8NSMSOTP(ctx context.Context, client *http.Client, event *smsv1.SmsCodeReceivedEvent) (int, error) {
	if s == nil || event == nil || event.GetCode() == nil {
		return 0, nil
	}
	if s.smsOTPWaits == nil {
		return 0, fmt.Errorf("sms otp wait store is required")
	}
	activationID := strings.TrimSpace(event.GetOrderId())
	code := normalizeOTP(event.GetCode().GetValue())
	if activationID == "" || code == "" {
		return 0, nil
	}
	receivedAt := smsOTPReceivedAt(event)
	pending, err := s.smsOTPWaits.PendingForActivation(ctx, activationID, receivedAt)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	resumed := 0
	for _, job := range pending {
		claimed, err := s.smsOTPWaits.Claim(ctx, job.JobID, smsOTPClaimTTL(job.TimeoutSeconds))
		if err != nil {
			return resumed, err
		}
		if !claimed {
			continue
		}
		if err := s.resumeSMSOTPJob(ctx, client, job, code, receivedAt); err != nil {
			_ = s.smsOTPWaits.ReleaseClaim(ctx, job.JobID)
			return resumed, err
		}
		resumed++
	}
	return resumed, nil
}

func (s *Server) resumeSMSOTPJob(ctx context.Context, client *http.Client, job smsotpwait.Entry, code string, receivedAt int64) error {
	resumeURL, err := s.runtimeSecretValue(ctx, job.ResumeSecretKey)
	if err != nil {
		return err
	}
	if _, err := s.completeSMSOTPWait(ctx, job, code, receivedAt); err != nil {
		return err
	}
	if err := postN8NResume(ctx, client, resumeURL, map[string]any{
		"job_id":                job.JobID,
		"account_id":            job.AccountID,
		"operation":             job.Operation,
		"user_id":               job.UserID,
		"activation_id":         job.ActivationID,
		"otp_source":            "sms",
		"otp_issued_after_unix": job.IssuedAfterUnix,
		"n8n_execution_id":      strings.TrimSpace(job.N8NExecutionID),
	}); err != nil {
		return err
	}
	if err := s.markSMSOTPResolved(ctx, job.JobID, time.Now().Unix()); err != nil {
		return err
	}
	return s.smsOTPWaits.Delete(ctx, job)
}

func (s *Server) completeSMSOTPWait(ctx context.Context, job smsotpwait.Entry, code string, receivedAt int64) (map[string]any, error) {
	code = normalizeOTP(code)
	if code == "" {
		return nil, fmt.Errorf("sms otp code is required")
	}
	now := time.Now().Unix()
	timeoutSeconds := job.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	otpParam := firstNonEmpty(job.OTPParam, paymentOTPParam)
	submittedAtParam := firstNonEmpty(job.SubmittedAtParam, paymentOTPSubmittedAtParam)
	otpData := map[string]any{
		"channel":               "sms",
		"found":                 true,
		"source":                "sms",
		"activation_id":         job.ActivationID,
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

func (s *Server) latestSMSOTP(ctx context.Context, activationID string, issuedAfterUnix int64) (string, bool, error) {
	if s == nil || s.otpProjection == nil {
		return "", false, nil
	}
	code, ok, err := s.otpProjection.LatestSMSCode(ctx, activationID, issuedAfterUnix)
	return normalizeOTP(code), ok && normalizeOTP(code) != "", err
}

func (s *Server) markSMSOTPResolved(ctx context.Context, jobID string, atUnix int64) error {
	if atUnix <= 0 {
		atUnix = time.Now().Unix()
	}
	return s.setJobParams(ctx, jobID, map[string]string{smsOTPResumedAtParam: strconv.FormatInt(atUnix, 10)})
}

func (s *Server) smsOTPWaitResult(req n8nSMSOTPWaitRequest, found bool, success bool, data map[string]any) *n8nSMSOTPWaitResult {
	return &n8nSMSOTPWaitResult{JobID: req.JobID, AccountID: req.AccountID, N8NExecutionID: req.N8NExecutionID, Action: req.Action, Operation: req.Operation, UserID: req.UserID, Step: req.StepName, ActivationID: req.ActivationID, OTPRequired: true, OTPFound: found, OTPSource: smsOTPSource(found), OTPIssuedAfterUnix: req.OTPIssuedAfter, OTPTimeoutSeconds: req.TimeoutSeconds, Waiting: !found, Success: success, Data: data}
}

func smsOTPSource(found bool) string {
	if found {
		return "sms"
	}
	return ""
}

func normalizeN8NSMSOTPWaitRequest(req n8nSMSOTPWaitRequest) n8nSMSOTPWaitRequest {
	req.Action = strings.ToUpper(strings.TrimSpace(req.Action))
	req.JobID = strings.TrimSpace(req.JobID)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.N8NExecutionID = strings.TrimSpace(req.N8NExecutionID)
	req.Operation = strings.TrimSpace(req.Operation)
	req.UserID = strings.TrimSpace(req.UserID)
	req.ActivationID = strings.TrimSpace(req.ActivationID)
	req.StepName = strings.TrimSpace(req.StepName)
	req.ResumeURL = strings.TrimSpace(req.ResumeURL)
	req.OTPParam = strings.TrimSpace(req.OTPParam)
	req.SubmittedAtParam = strings.TrimSpace(req.SubmittedAtParam)
	return req
}

func smsOTPReceivedAt(event *smsv1.SmsCodeReceivedEvent) int64 {
	if event.GetCode() != nil && event.GetCode().GetReceivedAt() != nil {
		return event.GetCode().GetReceivedAt().AsTime().Unix()
	}
	if event.GetContext() != nil && event.GetContext().GetOccurredAt() != nil {
		return event.GetContext().GetOccurredAt().AsTime().Unix()
	}
	return time.Now().Unix()
}

func smsOTPClaimTTL(timeoutSeconds int32) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return time.Duration(timeoutSeconds)*time.Second + time.Hour
}

func logSMSOTPResumeWorker(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
