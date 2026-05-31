package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nChannelOTPWaitRequest struct {
	Action           string
	JobID            string
	AccountID        string
	N8NExecutionID   string
	FlowID           string
	Operation        string
	Channel          string
	Target           string
	StepName         string
	TimeoutSeconds   int32
	OTPIssuedAfter   int64
	ResumeURL        string
	OTPParam         string
	SubmittedAtParam string
}

type n8nChannelOTPWaitConfig struct {
	Channel              string
	StepName             string
	ResumeSecretPrefix   string
	FlowIDParam          string
	IssuedAfterParam     string
	TimeoutParam         string
	ResumeSecretKeyParam string
	PollReason           string
	Latest               func(context.Context, string, int64) (n8nChannelOTPLatestResult, error)
	Poll                 func(context.Context, n8nChannelOTPWaitRequest, n8nChannelOTPWaitConfig, time.Duration) error
}

type n8nChannelOTPLatestResult struct {
	Code           string
	Source         string
	Found          bool
	ReceivedAtUnix int64
	MessageID      string
	Metadata       *pb.N8NChannelOTPMetadata
}

func (s *Server) awaitN8NChannelOTP(ctx context.Context, req n8nChannelOTPWaitRequest, cfg n8nChannelOTPWaitConfig) (*pb.N8NChannelOTPWaitResult, error) {
	req = normalizeN8NChannelOTPWaitRequest(req, cfg.Channel)
	if req.JobID == "" || req.Target == "" || req.ResumeURL == "" || req.StepName == "" {
		return nil, fmt.Errorf("job_id, target, resume_url and step_name are required")
	}
	if strings.TrimSpace(cfg.ResumeSecretPrefix) == "" {
		return nil, fmt.Errorf("resume secret prefix is required")
	}
	if strings.TrimSpace(cfg.FlowIDParam) != "" && req.FlowID == "" {
		return nil, fmt.Errorf("flow_id is required")
	}
	if s.channelOTPWaits == nil {
		return nil, fmt.Errorf("channel otp wait store is required")
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = channelotpwait.DefaultTimeoutSeconds(req.Channel)
	}
	if err := s.bindN8NExecution(ctx, req.JobID, req.N8NExecutionID); err != nil {
		return nil, err
	}
	secretKey := cfg.ResumeSecretPrefix + req.JobID
	ttl := time.Duration(req.TimeoutSeconds)*time.Second + time.Hour
	if err := s.saveRuntimeSecretValueTTL(ctx, secretKey, req.ResumeURL, ttl); err != nil {
		return nil, err
	}
	entry := channelotpwait.Entry{
		JobID:            req.JobID,
		AccountID:        req.AccountID,
		FlowID:           req.FlowID,
		Action:           req.Action,
		Operation:        req.Operation,
		Channel:          req.Channel,
		Target:           req.Target,
		StepName:         req.StepName,
		IssuedAfterUnix:  req.OTPIssuedAfter,
		TimeoutSeconds:   req.TimeoutSeconds,
		ResumeSecretKey:  secretKey,
		N8NExecutionID:   req.N8NExecutionID,
		OTPParam:         req.OTPParam,
		SubmittedAtParam: req.SubmittedAtParam,
	}
	if err := s.channelOTPWaits.Register(ctx, entry, ttl); err != nil {
		return nil, err
	}
	detail := channelOTPWaitDetail(req)
	if cfg.Poll != nil {
		detail.MailboxPollRequested = true
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: req.JobID, StepName: req.StepName, Recoverable: false, Retryable: true, Detail: jobDataMessage(detail)}); err != nil {
		return nil, err
	}
	params := map[string]string{
		contracts.JobParamChannelOTPChannel:        req.Channel,
		contracts.JobParamChannelOTPTarget:         req.Target,
		contracts.JobParamChannelOTPIssuedAfter:    strconv.FormatInt(req.OTPIssuedAfter, 10),
		contracts.JobParamChannelOTPTimeoutSeconds: strconv.FormatInt(int64(req.TimeoutSeconds), 10),
	}
	putN8NChannelOTPParam(params, cfg.FlowIDParam, req.FlowID)
	putN8NChannelOTPParam(params, cfg.IssuedAfterParam, strconv.FormatInt(req.OTPIssuedAfter, 10))
	putN8NChannelOTPParam(params, cfg.TimeoutParam, strconv.FormatInt(int64(req.TimeoutSeconds), 10))
	putN8NChannelOTPParam(params, cfg.ResumeSecretKeyParam, secretKey)
	if req.N8NExecutionID != "" {
		params["n8n_execution_id"] = req.N8NExecutionID
	}
	if err := s.setJobParams(ctx, req.JobID, params); err != nil {
		return nil, err
	}
	if cfg.Latest != nil {
		latest, err := cfg.Latest(ctx, req.Target, req.OTPIssuedAfter)
		if err != nil {
			return nil, err
		}
		if latest.Found {
			receivedAt := latest.ReceivedAtUnix
			if receivedAt <= 0 {
				receivedAt = time.Now().Unix()
			}
			otpData, err := s.completeChannelOTPWait(ctx, channelOTPCompleteRequest{
				Job:            entry,
				Code:           latest.Code,
				ReceivedAtUnix: receivedAt,
				Channel:        req.Channel,
				Source:         firstNonEmpty(latest.Source, req.Channel),
				Metadata:       latest.Metadata,
			})
			if err != nil {
				return nil, err
			}
			if err := s.markChannelOTPResolved(ctx, req.JobID, contracts.JobParamChannelOTPResumedAtUnix, time.Now().Unix()); err != nil {
				return nil, err
			}
			_ = s.channelOTPWaits.Delete(ctx, entry)
			return channelOTPWaitResult(req, true, true, firstNonEmpty(latest.Source, req.Channel), latest.MessageID, channelOTPWaitData(detail, otpData)), nil
		}
	}
	if cfg.Poll != nil {
		if err := cfg.Poll(ctx, req, cfg, time.Duration(req.TimeoutSeconds)*time.Second); err != nil {
			return nil, err
		}
	}
	return channelOTPWaitResult(req, false, true, "", "", channelOTPWaitData(detail, nil)), nil
}

func (s *Server) awaitN8NAuthChannelOTP(ctx context.Context, cfg n8nChannelOTPWaitConfig, jobID string, accountID string, n8nExecutionID string, flowID string, channel string, target string, timeoutSeconds int32, otpIssuedAfterUnix int64, resumeURL string) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	cfg.Channel = firstNonEmpty(cfg.Channel, channelotpwait.ChannelEmail)
	if cfg.Channel == channelotpwait.ChannelEmail {
		cfg.Latest = s.latestMailboxEmailChannelOTP
		cfg.Poll = s.requestMailboxEmailChannelOTPPoll
	}
	out, err := s.awaitN8NChannelOTP(ctx, n8nChannelOTPWaitRequest{
		JobID:            jobID,
		AccountID:        accountID,
		N8NExecutionID:   n8nExecutionID,
		FlowID:           flowID,
		Channel:          channel,
		Target:           target,
		StepName:         cfg.StepName,
		TimeoutSeconds:   timeoutSeconds,
		OTPIssuedAfter:   otpIssuedAfterUnix,
		ResumeURL:        resumeURL,
		OTPParam:         contracts.JobParamRegistrationOTP,
		SubmittedAtParam: contracts.JobParamRegistrationOTPSubmittedAtUnix,
	}, cfg)
	if err != nil {
		return nil, err
	}
	return n8nAuthStepResultFromChannelOTP(out), nil
}

func normalizeN8NChannelOTPWaitRequest(req n8nChannelOTPWaitRequest, channel string) n8nChannelOTPWaitRequest {
	req.Action = contracts.NormalizeActionID(req.Action)
	req.JobID = strings.TrimSpace(req.JobID)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.N8NExecutionID = strings.TrimSpace(req.N8NExecutionID)
	req.FlowID = strings.TrimSpace(req.FlowID)
	req.Operation = strings.TrimSpace(req.Operation)
	req.Channel = channelotpwait.NormalizeChannel(firstNonEmpty(channel, req.Channel))
	req.Target = channelotpwait.NormalizeTarget(req.Channel, req.Target)
	req.StepName = strings.TrimSpace(req.StepName)
	req.ResumeURL = strings.TrimSpace(req.ResumeURL)
	req.OTPParam = strings.TrimSpace(req.OTPParam)
	req.SubmittedAtParam = strings.TrimSpace(req.SubmittedAtParam)
	return req
}

func channelOTPWaitDetail(req n8nChannelOTPWaitRequest) *pb.N8NChannelOTPWaitDetail {
	return &pb.N8NChannelOTPWaitDetail{
		Action:              req.Action,
		Operation:           req.Operation,
		AccountId:           req.AccountID,
		FlowId:              req.FlowID,
		Channel:             req.Channel,
		Target:              req.Target,
		TimeoutSeconds:      req.TimeoutSeconds,
		OtpIssuedAfterUnix:  req.OTPIssuedAfter,
		ResumeUrlRegistered: true,
		N8NExecutionId:      req.N8NExecutionID,
		WaitIndex:           "redis_ttl",
	}
}

func channelOTPWaitData(detail *pb.N8NChannelOTPWaitDetail, complete *pb.N8NChannelOTPCompleteData) *pb.N8NChannelOTPWaitData {
	return &pb.N8NChannelOTPWaitData{Detail: detail, Complete: complete}
}

func channelOTPWaitResult(req n8nChannelOTPWaitRequest, found bool, success bool, source string, messageID string, data *pb.N8NChannelOTPWaitData) *pb.N8NChannelOTPWaitResult {
	return &pb.N8NChannelOTPWaitResult{
		JobId:              req.JobID,
		AccountId:          req.AccountID,
		N8NExecutionId:     req.N8NExecutionID,
		FlowId:             req.FlowID,
		Action:             req.Action,
		Operation:          req.Operation,
		Step:               req.StepName,
		Channel:            req.Channel,
		Target:             req.Target,
		OtpRequired:        true,
		OtpFound:           found,
		OtpSource:          source,
		OtpIssuedAfterUnix: req.OTPIssuedAfter,
		OtpTimeoutSeconds:  req.TimeoutSeconds,
		MessageId:          strings.TrimSpace(messageID),
		Waiting:            !found,
		Success:            success,
		Data:               data,
	}
}

func putN8NChannelOTPParam(params map[string]string, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key != "" && value != "" {
		params[key] = value
	}
}
