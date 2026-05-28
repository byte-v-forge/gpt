package activities

import (
	"context"
	"orchestrator/db"
	"orchestrator/internal/jobprojection"
	"orchestrator/pb"
	"strings"
)

func (s *Server) PersistRegisteredActivity(ctx context.Context, input PersistRegisteredInput) error {
	if err := s.saveChatGPTSessionToken(ctx, input.GetAccountId(), input.GetSessionToken()); err != nil {
		return err
	}
	if err := s.saveChatGPTAccessToken(ctx, input.GetAccountId(), input.GetAccessToken()); err != nil {
		return err
	}
	account := &pb.Account{
		AccountId: input.GetAccountId(),
		Status:    "REGISTERED",
	}
	if input.GetPlusTrialChecked() {
		account.PlusTrialEligible = boolPtr(input.GetPlusTrialEligible())
	}
	if err := s.updateAccount(ctx, account); err != nil {
		return err
	}
	registeredAccount, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return err
	}
	email := strings.TrimSpace(registeredAccount.GetEmail())
	if email == "" {
		return nil
	}
	_, err = s.accountClient.MarkGPTEmailAllocationStatus(ctx, &pb.MarkGPTEmailAllocationStatusRequest{
		Email:  email,
		Status: emailStatusRegistered,
	})
	return err
}

func (s *Server) PersistActivatedActivity(ctx context.Context, input PersistActivatedInput) error {
	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return err
	}
	sessionToken := input.GetSessionToken()
	if sessionToken == "" {
		sessionToken = s.cachedChatGPTSessionToken(ctx, account.GetAccountId())
	}
	accessToken := input.GetAccessToken()
	if accessToken == "" {
		accessToken = s.cachedChatGPTAccessToken(ctx, account.GetAccountId())
	}
	if err := s.saveChatGPTSessionToken(ctx, input.GetAccountId(), sessionToken); err != nil {
		return err
	}
	if err := s.saveChatGPTAccessToken(ctx, input.GetAccountId(), accessToken); err != nil {
		return err
	}
	update := &pb.Account{
		AccountId:  input.GetAccountId(),
		Status:     accountStatusActivated,
		ChargeRef:  input.GetChargeRef(),
		PlusActive: boolPtr(true),
		Tier:       "plus",
	}
	if input.GetPlusTrialChecked() {
		update.PlusTrialEligible = boolPtr(input.GetPlusTrialEligible())
	}
	return s.updateAccount(ctx, update)
}

func (s *Server) MarkJobFailedActivity(ctx context.Context, input JobFailureInput) error {
	if input.Status == "" {
		input.Status = failedStatus(input.Recoverable, input.Retryable)
	}
	s.updateJob(ctx, input.GetJobId(), input.GetStatus(), input.GetErrorMessage(), protoDataMap(input.GetResult()))
	if input.GetStepName() != "" {
		return s.markStepFailed(ctx, input)
	}
	return nil
}

func (s *Server) MarkJobSucceededActivity(ctx context.Context, input JobSuccessInput) error {
	s.updateJob(ctx, input.GetJobId(), statusSucceeded, "", protoDataMap(input.GetResult()))
	return nil
}

func (s *Server) createJobWithID(ctx context.Context, jobID, accountID, action string, params map[string]string) (*db.Job, error) {
	return s.jobStore.CreateWithID(ctx, jobID, accountID, action, params)
}

func (s *Server) markStepFailed(ctx context.Context, input JobFailureInput) error {
	return s.jobStore.MarkStepFailed(ctx, jobprojection.StepFailure{
		JobID:        input.GetJobId(),
		StepName:     input.GetStepName(),
		Status:       input.GetStatus(),
		Recoverable:  input.GetRecoverable(),
		Retryable:    input.GetRetryable(),
		ErrorMessage: input.GetErrorMessage(),
		Result:       protoDataMap(input.GetResult()),
	})
}
