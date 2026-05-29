package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/eventbus"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"

	"orchestrator/internal/accountmail"
	"orchestrator/internal/emailotpwait"
	"orchestrator/pb"
)

const (
	N8NEmailOTPResumeConsumerDurable = "gpt-email-otp-resume"
	emailOTPResumedAtParam           = "email_otp_resumed_at_unix"
	emailOTPSourceParam              = "email_otp_source"
	emailOTPMessageIDParam           = "email_otp_message_id"
)

type n8nEmailOTPResumeWorker struct {
	server *Server
	client *http.Client
}

func StartN8NEmailOTPResumeWorker(ctx context.Context, consumer eventbus.Consumer, server *Server) error {
	worker := &n8nEmailOTPResumeWorker{
		server: server,
		client: &http.Client{Timeout: 20 * time.Second},
	}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "GPT email OTP resume events",
		Consumer: consumer,
		Handler:  worker.handle,
		Logf:     logEmailOTPResumeWorker,
	})
}

func (w *n8nEmailOTPResumeWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	event := &mailboxv1.MailboxEmailReceivedEvent{}
	if err := eventbus.UnmarshalPayload(message, event); err != nil {
		eventbus.TermMessage(ctx, message, "terminate malformed email event", logEmailOTPResumeWorker)
		return
	}
	count, err := w.server.ResumeN8NEmailOTP(ctx, w.client, event)
	if err != nil {
		log.Printf("[orchestrator] resume email OTP failed event_id=%s: %v", eventbus.EventID(message), err)
		eventbus.NakMessageDelay(ctx, message, 15*time.Second, "retry email OTP resume", logEmailOTPResumeWorker)
		return
	}
	if count > 0 {
		log.Printf("[orchestrator] resumed email OTP wait count=%d event_id=%s", count, eventbus.EventID(message))
	}
	eventbus.AckMessage(ctx, message, "ack email OTP resume event", logEmailOTPResumeWorker)
}

func (s *Server) ResumeN8NEmailOTP(ctx context.Context, client *http.Client, event *mailboxv1.MailboxEmailReceivedEvent) (int, error) {
	if s == nil || event == nil || event.GetMessage() == nil {
		return 0, nil
	}
	if s.emailOTPWaits == nil {
		return 0, fmt.Errorf("email otp wait store is required")
	}
	message := accountmail.EnrichMessage(event.GetMessage())
	if !accountmail.IsOpenAIMessage(message) {
		return 0, nil
	}
	code := normalizeOTP(accountmail.OTPCode(message))
	if code == "" {
		return 0, nil
	}
	emails := emailOTPMessageEmails(message)
	if len(emails) == 0 {
		return 0, nil
	}
	receivedAt := message.GetReceivedAtUnix()
	if receivedAt <= 0 {
		receivedAt = time.Now().Unix()
	}
	pending, err := s.emailOTPWaits.PendingForEmails(ctx, emails, receivedAt)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	resumed := 0
	for _, job := range pending {
		claimed, err := s.emailOTPWaits.Claim(ctx, job.JobID, emailOTPClaimTTL(job.TimeoutSeconds))
		if err != nil {
			return resumed, err
		}
		if !claimed {
			continue
		}
		if err := s.resumeEmailOTPJob(ctx, client, job, message, code, receivedAt); err != nil {
			_ = s.emailOTPWaits.ReleaseClaim(ctx, job.JobID)
			return resumed, err
		}
		resumed++
	}
	return resumed, nil
}

func (s *Server) pendingEmailOTPJob(ctx context.Context, jobID string) (*emailotpwait.Entry, error) {
	if s == nil || s.emailOTPWaits == nil {
		return nil, fmt.Errorf("email otp wait store is required")
	}
	job, ok, err := s.emailOTPWaits.Get(ctx, jobID)
	if err != nil || !ok {
		return nil, err
	}
	return &job, nil
}

func (s *Server) resumeEmailOTPJob(ctx context.Context, client *http.Client, job emailotpwait.Entry, message *mailboxv1.EmailInboxMessage, code string, receivedAt int64) error {
	resumeURL, err := s.runtimeSecretValue(ctx, job.ResumeSecretKey)
	if err != nil {
		return err
	}
	if _, err := s.completeMailboxEmailOTPWait(ctx, job, message, code, receivedAt); err != nil {
		return err
	}
	if err := postN8NResume(ctx, client, resumeURL, map[string]any{
		"job_id":                job.JobID,
		"account_id":            job.AccountID,
		"flow_id":               job.FlowID,
		"email":                 emailx.Normalize(job.Email),
		"otp_source":            "mailbox",
		"otp_issued_after_unix": job.IssuedAfterUnix,
		"message_id":            strings.TrimSpace(message.GetId()),
		"n8n_execution_id":      strings.TrimSpace(job.N8NExecutionID),
	}); err != nil {
		return err
	}
	if err := s.markEmailOTPResolved(ctx, job.JobID, time.Now().Unix()); err != nil {
		return err
	}
	return s.emailOTPWaits.Delete(ctx, job)
}

func (s *Server) completeMailboxEmailOTPWait(ctx context.Context, job emailotpwait.Entry, message *mailboxv1.EmailInboxMessage, code string, receivedAt int64) (map[string]any, error) {
	stepName := emailOTPWaitStep(job, stepRegisterAccountProtocolOTPWait)
	message = accountmail.EnrichMessage(message)
	code = normalizeOTP(code)
	if code == "" {
		code = normalizeOTP(accountmail.OTPCode(message))
	}
	if code == "" {
		return nil, fmt.Errorf("mailbox otp code is required")
	}
	now := time.Now().Unix()
	timeoutSeconds := job.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	messageID := strings.TrimSpace(message.GetId())
	otpData := map[string]any{
		"channel":               "email",
		"email":                 emailx.Normalize(job.Email),
		"found":                 true,
		"source":                "mailbox",
		"message_id":            messageID,
		"provider_key":          strings.TrimSpace(message.GetProviderKey()),
		"timeout_seconds":       timeoutSeconds,
		"issued_after_unix":     job.IssuedAfterUnix,
		"received_at_unix":      receivedAt,
		"n8n_execution_id":      strings.TrimSpace(job.N8NExecutionID),
		"resume_event_received": true,
	}
	params := map[string]string{
		registrationOTPParam:            code,
		registrationOTPSubmittedAtParam: strconv.FormatInt(now, 10),
		emailOTPSourceParam:             "mailbox",
		emailOTPMessageIDParam:          messageID,
	}
	if err := s.setJobParams(ctx, job.JobID, params); err != nil {
		return nil, err
	}
	if err := s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{
		JobId:       job.JobID,
		StepName:    stepName,
		Recoverable: false,
		Retryable:   true,
		Result:      structData(otpData),
	}); err != nil {
		return nil, err
	}
	return otpData, nil
}

func emailOTPWaitStep(job emailotpwait.Entry, fallback string) string {
	if stepName := strings.TrimSpace(job.StepName); stepName != "" {
		return stepName
	}
	return fallback
}

func (s *Server) ResumeN8NEmailManualOTP(ctx context.Context, client *http.Client, jobID string) error {
	job, err := s.pendingEmailOTPJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("email otp wait not found for job %s", strings.TrimSpace(jobID))
	}
	claimed, err := s.emailOTPWaits.Claim(ctx, job.JobID, emailOTPClaimTTL(job.TimeoutSeconds))
	if err != nil || !claimed {
		return err
	}
	if err := s.resumeEmailManualOTP(ctx, client, *job); err != nil {
		_ = s.emailOTPWaits.ReleaseClaim(ctx, job.JobID)
		return err
	}
	return nil
}

func (s *Server) resumeEmailManualOTP(ctx context.Context, client *http.Client, job emailotpwait.Entry) error {
	stepName := emailOTPWaitStep(job, stepRegisterAccountProtocolOTPWait)
	resumeURL, err := s.runtimeSecretValue(ctx, job.ResumeSecretKey)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	timeoutSeconds := job.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	otpData := map[string]any{
		"channel":              "manual",
		"email":                emailx.Normalize(job.Email),
		"found":                true,
		"source":               "manual",
		"timeout_seconds":      timeoutSeconds,
		"issued_after_unix":    job.IssuedAfterUnix,
		"received_at_unix":     now,
		"n8n_execution_id":     strings.TrimSpace(job.N8NExecutionID),
		"manual_otp_submitted": true,
	}
	if err := s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{
		JobId:       job.JobID,
		StepName:    stepName,
		Recoverable: false,
		Retryable:   true,
		Result:      structData(otpData),
	}); err != nil {
		return err
	}
	if err := postN8NResume(ctx, client, resumeURL, map[string]any{
		"job_id":                job.JobID,
		"account_id":            job.AccountID,
		"flow_id":               job.FlowID,
		"email":                 emailx.Normalize(job.Email),
		"otp_source":            "manual",
		"otp_issued_after_unix": job.IssuedAfterUnix,
		"n8n_execution_id":      strings.TrimSpace(job.N8NExecutionID),
	}); err != nil {
		return err
	}
	if err := s.markEmailOTPResolved(ctx, job.JobID, now); err != nil {
		return err
	}
	return s.emailOTPWaits.Delete(ctx, job)
}

func (s *Server) markEmailOTPResolved(ctx context.Context, jobID string, atUnix int64) error {
	if atUnix <= 0 {
		atUnix = time.Now().Unix()
	}
	return s.setJobParams(ctx, jobID, map[string]string{emailOTPResumedAtParam: strconv.FormatInt(atUnix, 10)})
}

func emailOTPMessageEmails(message *mailboxv1.EmailInboxMessage) []string {
	if message == nil {
		return nil
	}
	emails := []string{message.GetMailboxEmail(), message.GetSourceMailboxEmail()}
	emails = append(emails, message.GetRecipients()...)
	return emailotpwait.EmailCandidates(emails...)
}

func emailOTPClaimTTL(timeoutSeconds int32) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	return time.Duration(timeoutSeconds)*time.Second + time.Hour
}

func postN8NResume(ctx context.Context, client *http.Client, resumeURL string, payload map[string]any) error {
	resumeURL = strings.TrimSpace(resumeURL)
	if resumeURL == "" {
		return errors.New("n8n resume url is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resumeURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post n8n resume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("n8n resume returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func logEmailOTPResumeWorker(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
