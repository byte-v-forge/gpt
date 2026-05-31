package api

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/contracts"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) SubmitOTP(ctx context.Context, req *pb.SubmitOTPRequest) (*pb.SubmitOTPResponse, error) {
	otp := channelotpwait.NormalizeCode(req.GetOtp())
	if otp == "" {
		return &pb.SubmitOTPResponse{Success: false, ErrorMessage: "otp is required"}, nil
	}
	jobID, resumed, err := s.resumeManualChannelOTP(ctx, nil, req.GetChannel(), req.GetTarget(), otp)
	if err != nil {
		return &pb.SubmitOTPResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if !resumed {
		return &pb.SubmitOTPResponse{Success: false, ErrorMessage: "pending channel otp job not found"}, nil
	}

	log.Printf("[orchestrator] channel otp submitted job=%s source=manual", jobID)
	return &pb.SubmitOTPResponse{Success: true, JobId: jobID}, nil
}

func (s *Server) ResendOTP(ctx context.Context, req *pb.ResendOTPRequest) (*pb.ResendOTPResponse, error) {
	jobID, err := s.resolveRegisterOTPResendJob(ctx, strings.TrimSpace(req.GetJobId()))
	if err != nil {
		return &pb.ResendOTPResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	if err := s.setJobParams(ctx, jobID, map[string]string{contracts.JobParamRegistrationOTPResendRequestedAtUnix: strconv.FormatInt(time.Now().Unix(), 10)}); err != nil {
		return &pb.ResendOTPResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	log.Printf("[orchestrator] registration otp resend requested job=%s source=job-param", jobID)
	return &pb.ResendOTPResponse{Success: true, JobId: jobID}, nil
}

func (s *Server) resolveRegisterOTPResendJob(ctx context.Context, jobID string) (string, error) {
	if jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		if jobprojection.IsNotFound(err) {
			return "", fmt.Errorf("job not found: %s", jobID)
		}
		return "", err
	}
	if job.Status != jobstatus.Running {
		return "", fmt.Errorf("job is not running: %s", job.Status)
	}
	if strings.TrimSpace(job.Action) != contracts.ActionRegister {
		return "", fmt.Errorf("otp resend is not supported for job action: %s", job.Action)
	}
	return jobID, nil
}
