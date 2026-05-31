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

type channelOTPCompleteRequest struct {
	Job            channelotpwait.Entry
	Code           string
	ReceivedAtUnix int64
	Channel        string
	Source         string
	Metadata       *pb.N8NChannelOTPMetadata
	ExtraParams    map[string]string
}

func (s *Server) completeChannelOTPWait(ctx context.Context, req channelOTPCompleteRequest) (*pb.N8NChannelOTPCompleteData, error) {
	code := channelotpwait.NormalizeCode(req.Code)
	if code == "" {
		return nil, fmt.Errorf("%s otp code is required", req.Channel)
	}
	now := time.Now().Unix()
	timeoutSeconds := req.Job.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = channelotpwait.DefaultTimeoutSeconds(firstNonEmpty(req.Channel, req.Job.Channel))
	}
	otpParam := firstNonEmpty(req.Job.OTPParam, contracts.JobParamChannelOTP)
	submittedAtParam := firstNonEmpty(req.Job.SubmittedAtParam, contracts.JobParamChannelOTPSubmittedAtUnix)
	otpData := channelOTPCompleteData(req, timeoutSeconds)
	params := map[string]string{otpParam: code, submittedAtParam: strconv.FormatInt(now, 10)}
	for key, value := range req.ExtraParams {
		key = strings.TrimSpace(key)
		if key != "" {
			params[key] = value
		}
	}
	if err := s.setJobParams(ctx, req.Job.JobID, params); err != nil {
		return nil, err
	}
	if err := s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{JobId: req.Job.JobID, StepName: req.Job.StepName, Recoverable: false, Retryable: true, Result: jobDataMessage(otpData)}); err != nil {
		return nil, err
	}
	return otpData, nil
}

func channelOTPCompleteData(req channelOTPCompleteRequest, timeoutSeconds int32) *pb.N8NChannelOTPCompleteData {
	data := &pb.N8NChannelOTPCompleteData{
		Channel:             req.Channel,
		Target:              req.Job.Target,
		Found:               true,
		Source:              req.Source,
		TimeoutSeconds:      timeoutSeconds,
		IssuedAfterUnix:     req.Job.IssuedAfterUnix,
		ReceivedAtUnix:      req.ReceivedAtUnix,
		N8NExecutionId:      strings.TrimSpace(req.Job.N8NExecutionID),
		ResumeEventReceived: true,
		AccountId:           req.Job.AccountID,
		FlowId:              req.Job.FlowID,
	}
	if req.Metadata != nil {
		data.MessageId = strings.TrimSpace(req.Metadata.GetMessageId())
		data.ProviderKey = strings.TrimSpace(req.Metadata.GetProviderKey())
		data.ManualOtpSubmitted = req.Metadata.GetManualOtpSubmitted()
	}
	return data
}

func (s *Server) markChannelOTPResolved(ctx context.Context, jobID string, param string, atUnix int64) error {
	if atUnix <= 0 {
		atUnix = time.Now().Unix()
	}
	return s.setJobParams(ctx, jobID, map[string]string{param: strconv.FormatInt(atUnix, 10)})
}
