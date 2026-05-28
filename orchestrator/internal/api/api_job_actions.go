package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobstatus"
	"orchestrator/internal/resultdata"
	"orchestrator/pb"
)

var jobActionRunnableActions = []string{
	actionActivate,
	actionAutopay,
	actionProbeAccount,
	actionRegister,
	actionRegisterAndActivate,
	actionLoginSession,
	actionLoginSessionProtocol,
	actionCodexOAuth,
	actionCodexOAuthProtocol,
	actionCodexOAuthAddPhone,
	actionCodexOAuthBatchAddPhone,
	actionGoPayApp,
	actionGoPayPayment,
	actionGoPayQRISPaymentActivate,
	actionGoPayWAPayment,
	actionGoPayPaymentRebind,
}
var jobActionStaleSteps = []string{
	"claimed",
	stepProbePlusTrial,
	stepProbeTier,
	stepGoPayPaymentPrepare,
	stepGoPayPaymentPrepareCheckout,
	stepGoPayPaymentPrepareRefresh,
	stepGoPayPaymentPrepareLink,
	stepGoPayPayment,
	stepGoPayResolveWAPhone,
	stepGoPayAppEnsureBalance,
	stepGoPayAppEnsureBalanceConfirm,
	stepRegisterAccountStart,
	stepRegisterAccountBrowser,
	stepRegisterAccountOTPWait,
	stepRegisterAccountComplete,
	stepProtocolUseProxy,
	stepRegisterAccountProtocolStart,
	stepRegisterAccountProtocol,
	stepRegisterAccountProtocolOTPWait,
	stepRegisterAccountProtocolComplete,
	stepLoginSessionStart,
	stepLoginSessionBrowser,
	stepLoginSessionOTPWait,
	stepLoginSessionComplete,
	stepLoginSessionProtocolStart,
	stepLoginSessionProtocol,
	stepLoginSessionProtocolOTPWait,
	stepLoginSessionProtocolComplete,
	stepCodexOAuthAcquirePhone,
	stepCodexOAuthProtocolStart,
	stepCodexOAuthProtocolDetect,
	stepCodexOAuthProtocolEmail,
	stepCodexOAuthProtocolPassword,
	stepCodexOAuthProtocolEmailOTP,
	stepCodexOAuthProtocolAddPhone,
	stepCodexOAuthProtocolComplete,
	stepCodexOAuthBrowserStart,
	stepCodexOAuthBrowserDetect,
	stepCodexOAuthBrowserEmail,
	stepCodexOAuthBrowserPassword,
	stepCodexOAuthBrowserEmailOTP,
	stepCodexOAuthBrowserAddPhone,
	stepCodexOAuthBrowserComplete,
	stepCodexOAuthReleasePhone,
	stepGoPayAppLogin,
	stepGoPayAppEnsurePINSetup,
	stepGoPayAppChangePhone,
	stepGoPayAppChangePhoneGetNumber,
	stepGoPayAppChangePhoneStart,
	stepGoPayAppChangePhoneSMSWait,
	stepGoPayAppChangePhoneRetry,
	stepGoPayAppChangePhoneCancel,
	stepGoPayAppChangePhoneComplete,
	stepGoPayAppSMSFinish,
	stepGoPayAppSMSRequestMore,
	stepGoPayAppSignup,
	stepGoPayAppSignupRetry,
	stepGoPayAppSignupPhoneCancel,
	stepGoPayAppSignupPhone,
	stepGoPayAppGenerateDeviceProxy,
	stepGoPayAppCheckPhone,
	stepGoPayAppDeactivate,
	stepGoPayAppDeactivateStart,
	stepGoPayAppDeactivateSMSWait,
	stepGoPayAppDeactivateComplete,
	stepGoPayAppStatus,
}

func (s *Server) RunGPTJobAction(ctx context.Context, req *pb.GPTJobActionRequest) (*pb.GPTJobActionResponse, error) {
	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}
	if s.jobStore == nil {
		return nil, status.Error(codes.Unavailable, "job store is not configured")
	}
	if s.activities == nil {
		return nil, status.Error(codes.Unavailable, "GPT action runner is not configured")
	}
	job, terminal, err := s.jobStore.StartClaimedRun(ctx, jobID, req.GetIdempotencyKey())
	if err != nil {
		return nil, jobActionStartError(err)
	}
	if terminal {
		return s.jobActionResponse(ctx, jobID, job.Action, ""), nil
	}
	action := strings.ToUpper(strings.TrimSpace(req.GetAction()))
	if action == "" && job != nil {
		action = strings.ToUpper(strings.TrimSpace(job.Action))
	}
	switch action {
	case actionActivate:
		err = s.runActivateAccountAction(ctx, jobID, req.GetParams())
	case actionAutopay:
		err = s.runAutopayAccountAction(ctx, jobID, req.GetParams())
	case actionProbeAccount:
		err = s.runProbeAccountAction(ctx, jobID, req.GetAccountId())
	case actionRegister:
		err = s.runRegisterAccountAction(ctx, jobID, req.GetAccountId(), req.GetParams())
	case actionRegisterAndActivate:
		err = s.runRegisterAndActivateAction(ctx, jobID, req.GetParams())
	case actionLoginSession:
		err = s.runLoginSessionAction(ctx, jobID, req.GetAccountId())
	case actionLoginSessionProtocol:
		err = s.runLoginSessionProtocolAction(ctx, jobID, req.GetAccountId())
	case actionCodexOAuth:
		err = s.runCodexOAuthAction(ctx, jobID, req.GetAccountId(), req.GetParams())
	case actionCodexOAuthProtocol:
		err = s.runCodexOAuthProtocolAction(ctx, jobID, req.GetAccountId(), req.GetParams())
	case actionCodexOAuthAddPhone:
		err = s.runCodexOAuthAddPhoneAction(ctx, jobID, req.GetAccountId(), req.GetParams())
	case actionCodexOAuthBatchAddPhone:
		err = s.runCodexOAuthBatchAddPhoneAction(ctx, jobID, req.GetParams())
	case actionGoPayApp:
		err = s.runGoPayAppAction(ctx, jobID, req.GetParams())
	case actionGoPayPayment:
		err = s.runGoPayPaymentAction(ctx, jobID, req.GetParams())
	case actionGoPayQRISPaymentActivate:
		err = s.runGoPayQRISPaymentActivateAction(ctx, jobID, req.GetParams())
	case actionGoPayWAPayment:
		err = s.runGoPayWAPaymentAction(ctx, jobID, req.GetParams())
	case actionGoPayPaymentRebind:
		err = s.runGoPayPaymentRebindAction(ctx, jobID, req.GetParams())
	default:
		err = jobprojection.ErrJobUnsupported
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	return s.jobActionResponse(ctx, jobID, action, message), nil
}

func (s *Server) runProbeAccountAction(ctx context.Context, jobID string, accountID string) error {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: strings.TrimSpace(accountID)})
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, nil)
	}
	combined := map[string]any{
		"account_id":               account.GetAccountId(),
		"plus_trial_already_known": account.GetPlusTrialKnown(),
		"tier":                     account.GetTier(),
		"plus_active":              account.GetPlusActive(),
	}
	plusTrial, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: account.GetAccountId()})
	combined["probe_plus_trial"] = structMap(plusTrial.GetData())
	if err != nil {
		return err
	}

	tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: account.GetAccountId()})
	combined["probe_tier"] = structMap(tier.GetData())
	if err != nil {
		return err
	}
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)}); err != nil {
		return err
	}
	if !plusTrial.GetSuccess() || !tier.GetSuccess() {
		return fmt.Errorf("probe completed with unsuccessful result")
	}
	return nil
}

func (s *Server) markActionFailed(ctx context.Context, jobID string, step string, statusValue string, recoverable bool, retryable bool, err error, data map[string]any) error {
	if err == nil {
		return nil
	}
	markErr := s.activities.MarkJobFailedActivity(ctx, pb.JobFailureInput{
		JobId:        jobID,
		StepName:     step,
		Status:       statusValue,
		Recoverable:  recoverable,
		Retryable:    retryable,
		ErrorMessage: err.Error(),
		Result:       structData(data),
	})
	if markErr != nil {
		return errors.Join(err, markErr)
	}
	return err
}

func (s *Server) jobActionResponse(ctx context.Context, jobID string, action string, fallbackError string) *pb.GPTJobActionResponse {
	snapshot, err := s.jobStore.GetSnapshot(ctx, jobID)
	if err != nil {
		return &pb.GPTJobActionResponse{JobId: jobID, Action: action, ErrorMessage: err.Error()}
	}
	statusValue := ""
	message := strings.TrimSpace(fallbackError)
	if snapshot.GetJob() != nil {
		statusValue = snapshot.GetJob().GetStatus()
		if message == "" {
			message = snapshot.GetJob().GetErrorMessage()
		}
	}
	return &pb.GPTJobActionResponse{
		JobId:        jobID,
		Action:       action,
		Success:      strings.EqualFold(statusValue, jobstatus.Succeeded),
		Status:       statusValue,
		ErrorMessage: message,
		Snapshot:     snapshot,
	}
}

func jobActionStartError(err error) error {
	switch {
	case errors.Is(err, jobprojection.ErrJobNotClaimed):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, jobprojection.ErrJobAlreadyRunning):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, jobprojection.ErrJobStaleClaim):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, jobprojection.ErrJobUnsupported):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "start GPT job action: %v", err)
	}
}

func structMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return nil
	}
	return value.AsMap()
}

func structData(value map[string]any) *structpb.Struct {
	return resultdata.Struct(value)
}
