package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/protojsonx"
	"google.golang.org/protobuf/proto"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type channelOTPResumeRequest struct {
	Job            channelotpwait.Entry
	Code           string
	ReceivedAtUnix int64
	Channel        string
	Source         string
	Metadata       *pb.N8NChannelOTPMetadata
	MessageID      string
}

type channelOTPEvent struct {
	Channel        string
	Targets        []string
	Code           string
	Source         string
	ReceivedAtUnix int64
	Metadata       *pb.N8NChannelOTPMetadata
	MessageID      string
}

func (s *Server) resumeChannelOTPJob(ctx context.Context, client *http.Client, req channelOTPResumeRequest) error {
	resumeURL, err := s.runtimeSecretValue(ctx, req.Job.ResumeSecretKey)
	if err != nil {
		return err
	}
	source := firstNonEmpty(req.Source, req.Channel)
	if _, err := s.completeChannelOTPWait(ctx, channelOTPCompleteRequest{
		Job:            req.Job,
		Code:           req.Code,
		ReceivedAtUnix: req.ReceivedAtUnix,
		Channel:        req.Channel,
		Source:         source,
		Metadata:       req.Metadata,
	}); err != nil {
		return err
	}
	payload := channelOTPResumePayload(req, source)
	if err := postN8NResume(ctx, client, resumeURL, payload); err != nil {
		return err
	}
	if err := s.markChannelOTPResolved(ctx, req.Job.JobID, contracts.JobParamChannelOTPResumedAtUnix, time.Now().Unix()); err != nil {
		return err
	}
	return s.channelOTPWaits.Delete(ctx, req.Job)
}

func (s *Server) resumeManualChannelOTP(ctx context.Context, client *http.Client, channel string, target string, otp string) (string, bool, error) {
	if s == nil || s.channelOTPWaits == nil {
		return "", false, nil
	}
	channel = channelotpwait.NormalizeChannel(channel)
	target = channelotpwait.NormalizeTarget(channel, target)
	code := channelotpwait.NormalizeCode(otp)
	if channel == "" {
		return "", false, fmt.Errorf("otp channel is required")
	}
	if target == "" {
		return "", false, fmt.Errorf("otp target is required")
	}
	if code == "" {
		return "", false, fmt.Errorf("%s otp code is required", channel)
	}
	receivedAt := time.Now().Unix()
	event := channelOTPEvent{
		Channel:        channel,
		Targets:        []string{target},
		Code:           code,
		ReceivedAtUnix: receivedAt,
		Source:         "manual",
		Metadata:       &pb.N8NChannelOTPMetadata{ManualOtpSubmitted: true},
	}
	pending, err := s.pendingChannelOTPJobs(ctx, channel, []string{target}, receivedAt)
	if err != nil {
		return "", false, err
	}
	if len(pending) == 0 {
		return "", false, nil
	}
	jobIDs, err := s.resumeChannelOTPJobs(ctx, client, event, pending)
	if err != nil {
		return "", true, err
	}
	if len(jobIDs) == 0 {
		return pending[0].JobID, true, nil
	}
	return jobIDs[0], true, nil
}

func channelOTPResumePayload(req channelOTPResumeRequest, source string) *pb.N8NChannelOTPResumePayload {
	return &pb.N8NChannelOTPResumePayload{
		JobId:              strings.TrimSpace(req.Job.JobID),
		AccountId:          strings.TrimSpace(req.Job.AccountID),
		Operation:          strings.TrimSpace(req.Job.Operation),
		FlowId:             strings.TrimSpace(req.Job.FlowID),
		Channel:            strings.TrimSpace(req.Channel),
		Target:             strings.TrimSpace(req.Job.Target),
		OtpSource:          strings.TrimSpace(source),
		OtpIssuedAfterUnix: req.Job.IssuedAfterUnix,
		N8NExecutionId:     strings.TrimSpace(req.Job.N8NExecutionID),
		MessageId:          strings.TrimSpace(req.MessageID),
	}
}

func postN8NResume(ctx context.Context, client *http.Client, resumeURL string, payload proto.Message) error {
	resumeURL = strings.TrimSpace(resumeURL)
	if resumeURL == "" {
		return errors.New("n8n resume url is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	body, err := protojsonx.Marshal(payload)
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

func (s *Server) resumePendingChannelOTP(ctx context.Context, client *http.Client, event channelOTPEvent) (int, error) {
	jobIDs, err := s.resumeChannelOTPEvent(ctx, client, event)
	return len(jobIDs), err
}

func (s *Server) resumeChannelOTPEvent(ctx context.Context, client *http.Client, event channelOTPEvent) ([]string, error) {
	if s == nil || s.channelOTPWaits == nil {
		return nil, fmt.Errorf("channel otp wait store is required")
	}
	channel := channelotpwait.NormalizeChannel(event.Channel)
	code := channelotpwait.NormalizeCode(event.Code)
	if channel == "" || code == "" {
		return nil, nil
	}
	targets := []string{}
	for _, target := range event.Targets {
		target = channelotpwait.NormalizeTarget(channel, target)
		if target != "" {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}
	pending, err := s.pendingChannelOTPJobs(ctx, channel, targets, event.ReceivedAtUnix)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	return s.resumeChannelOTPJobs(ctx, client, event, pending)
}

func (s *Server) resumeChannelOTPJobs(ctx context.Context, client *http.Client, event channelOTPEvent, pending []channelotpwait.Entry) ([]string, error) {
	channel := channelotpwait.NormalizeChannel(event.Channel)
	code := channelotpwait.NormalizeCode(event.Code)
	if channel == "" || code == "" || len(pending) == 0 {
		return nil, nil
	}
	jobIDs := []string{}
	for _, job := range pending {
		claimed, err := s.channelOTPWaits.Claim(ctx, job.JobID, channelOTPClaimTTL(job.TimeoutSeconds, channel))
		if err != nil {
			return jobIDs, err
		}
		if !claimed {
			continue
		}
		if err := s.resumeChannelOTPJob(ctx, client, channelOTPResumeRequest{
			Job:            job,
			Code:           code,
			ReceivedAtUnix: event.ReceivedAtUnix,
			Channel:        channel,
			Source:         event.Source,
			Metadata:       event.Metadata,
			MessageID:      event.MessageID,
		}); err != nil {
			_ = s.channelOTPWaits.ReleaseClaim(ctx, job.JobID)
			return jobIDs, err
		}
		jobIDs = append(jobIDs, job.JobID)
	}
	return jobIDs, nil
}

func (s *Server) pendingChannelOTPJobs(ctx context.Context, channel string, targets []string, receivedAt int64) ([]channelotpwait.Entry, error) {
	seen := map[string]struct{}{}
	out := []channelotpwait.Entry{}
	for _, target := range targets {
		pending, err := s.channelOTPWaits.Pending(ctx, channel, target, receivedAt)
		if err != nil {
			return nil, err
		}
		for _, job := range pending {
			if _, ok := seen[job.JobID]; ok {
				continue
			}
			seen[job.JobID] = struct{}{}
			out = append(out, job)
		}
	}
	return out, nil
}

func channelOTPClaimTTL(timeoutSeconds int32, channel string) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = channelotpwait.DefaultTimeoutSeconds(channel)
	}
	return time.Duration(timeoutSeconds)*time.Second + time.Hour
}
