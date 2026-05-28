package api

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"log"
	"orchestrator/db"
	"orchestrator/pb"
	"strconv"
	"strings"
	"time"
)

func (s *Server) SubmitOTP(ctx context.Context, req *pb.SubmitOTPRequest) (*pb.SubmitOTPResponse, error) {
	otp := normalizeOTP(req.GetOtp())
	if otp == "" {
		return &pb.SubmitOTPResponse{Success: false, ErrorMessage: "otp is required"}, nil
	}

	jobID, otpParam, submittedAtParam, otpKind, err := s.resolveManualOTPJob(ctx, strings.TrimSpace(req.GetJobId()), strings.TrimSpace(req.GetAccountId()))
	if err != nil {
		return &pb.SubmitOTPResponse{Success: false, ErrorMessage: err.Error()}, nil
	}

	if err := s.setJobParams(ctx, jobID, map[string]string{
		otpParam:         otp,
		submittedAtParam: strconv.FormatInt(time.Now().Unix(), 10),
	}); err != nil {
		return &pb.SubmitOTPResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if err := s.signalManualOTP(ctx, jobID, otpKind); err != nil {
		return &pb.SubmitOTPResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}

	log.Printf("[orchestrator] %s otp submitted job=%s source=manual", otpKind, jobID)
	return &pb.SubmitOTPResponse{Success: true, JobId: jobID}, nil
}

func (s *Server) ResendOTP(ctx context.Context, req *pb.ResendOTPRequest) (*pb.ResendOTPResponse, error) {
	jobID, _, _, otpKind, err := s.resolveManualOTPJob(ctx, strings.TrimSpace(req.GetJobId()), strings.TrimSpace(req.GetAccountId()))
	if err != nil {
		return &pb.ResendOTPResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	if otpKind != "registration" {
		return &pb.ResendOTPResponse{Success: false, JobId: jobID, ErrorMessage: "job is not waiting for browser registration otp"}, nil
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		return &pb.ResendOTPResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if job.Action != actionRegister && job.Action != actionRegisterAndActivate {
		return &pb.ResendOTPResponse{Success: false, JobId: jobID, ErrorMessage: "otp resend is not supported for job action: " + job.Action}, nil
	}
	if err := s.setJobParams(ctx, jobID, map[string]string{registrationOTPResendRequestedAtParam: strconv.FormatInt(time.Now().Unix(), 10)}); err != nil {
		return &pb.ResendOTPResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	log.Printf("[orchestrator] %s otp resend requested job=%s source=job-param", otpKind, jobID)
	return &pb.ResendOTPResponse{Success: true, JobId: jobID}, nil
}

func (s *Server) signalManualOTP(ctx context.Context, jobID, otpKind string) error {
	if otpKind != "registration" {
		return nil
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Action == actionRegisterProtocol && job.LastStep == stepRegisterAccountProtocolOTPWait {
		return s.ResumeN8NRegisterProtocolManualOTP(ctx, nil, jobID)
	}
	return nil
}

func (s *Server) resolveManualOTPJob(ctx context.Context, jobID, accountID string) (string, string, string, string, error) {
	if jobID != "" {
		var job db.Job
		err := s.db.WithContext(ctx).First(&job, "id = ?", jobID).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return "", "", "", "", fmt.Errorf("job not found: %s", jobID)
			}
			return "", "", "", "", err
		}
		if job.Status != statusRunning {
			return "", "", "", "", fmt.Errorf("job is not running: %s", job.Status)
		}
		otpParam, submittedAtParam, otpKind, err := s.manualOTPParamsForJob(ctx, &job)
		if err != nil {
			return "", "", "", "", err
		}
		return jobID, otpParam, submittedAtParam, otpKind, nil
	}
	if accountID == "" {
		return "", "", "", "", fmt.Errorf("job_id or account_id is required")
	}

	var job db.Job
	err := s.db.WithContext(ctx).
		Where("account_id = ? AND action IN ? AND status = ?", accountID, []string{actionRegister, actionRegisterProtocol, actionLoginSession, actionLoginSessionProtocol, actionActivate, actionAutopay, actionGoPayApp, actionGoPayPayment, actionGoPayQRISPaymentActivate, actionGoPayWAPayment, actionGoPayPaymentRebind, actionRegisterAndActivate}, statusRunning).
		Order("updated_at DESC").
		First(&job).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", "", "", fmt.Errorf("running otp-accepting job not found for account %s", accountID)
		}
		return "", "", "", "", err
	}
	otpParam, submittedAtParam, otpKind, err := s.manualOTPParamsForJob(ctx, &job)
	if err != nil {
		return "", "", "", "", err
	}
	return job.ID, otpParam, submittedAtParam, otpKind, nil
}

func (s *Server) manualOTPParamsForJob(ctx context.Context, job *db.Job) (string, string, string, error) {
	if job != nil && job.Action == actionRegisterAndActivate && job.LastStep == stepRegisterAccount && job.ID != "" && s != nil && s.db != nil {
		var step db.JobStep
		err := s.db.WithContext(ctx).First(&step, "job_id = ? AND step_name = ?", job.ID, stepRegisterAccount).Error
		if err == nil && step.Status == statusSucceeded {
			return paymentOTPParam, paymentOTPSubmittedAtParam, "payment", nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return "", "", "", err
		}
	}
	return manualOTPParamsForJobSnapshot(job)
}

func manualOTPParamsForJobSnapshot(job *db.Job) (string, string, string, error) {
	if job == nil {
		return "", "", "", fmt.Errorf("job is required")
	}
	switch job.Action {
	case actionRegister, actionRegisterProtocol, actionLoginSession, actionLoginSessionProtocol:
		return registrationOTPParam, registrationOTPSubmittedAtParam, "registration", nil
	case actionActivate, actionAutopay, actionGoPayApp, actionGoPayPayment, actionGoPayQRISPaymentActivate, actionGoPayWAPayment, actionGoPayPaymentRebind:
		return paymentOTPParam, paymentOTPSubmittedAtParam, "payment", nil
	case actionRegisterAndActivate:
		if job.LastStep == stepEnsureLogon || job.LastStep == stepGoPayPayment {
			return paymentOTPParam, paymentOTPSubmittedAtParam, "payment", nil
		}
		return registrationOTPParam, registrationOTPSubmittedAtParam, "registration", nil
	default:
		return "", "", "", fmt.Errorf("job does not accept otp: %s", job.Action)
	}
}
