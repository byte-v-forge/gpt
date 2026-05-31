package activities

import (
	"context"
	"fmt"
	"orchestrator/db"
	"orchestrator/internal/gptaccount"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
	"strings"
)

func (s *Server) createAccount(ctx context.Context, account *pb.Account, credential *pb.AccountCredential) (*pb.Account, error) {
	resp, err := s.accountClient.CreateAccount(ctx, &pb.CreateAccountRequest{Account: account, Credential: credential})
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	if resp.GetAccount() == nil || gptaccount.ID(resp.GetAccount()) == "" {
		return nil, fmt.Errorf("gpt-account returned empty account")
	}
	return resp.GetAccount(), nil
}

func (s *Server) getAccount(ctx context.Context, accountID string) (*pb.Account, error) {
	resp, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		return nil, err
	}
	if resp.GetAccount() == nil {
		return nil, fmt.Errorf("account not found: %s", accountID)
	}
	return resp.GetAccount(), nil
}

func (s *Server) getAccountPassword(ctx context.Context, accountID string) (string, error) {
	resp, err := s.accountClient.GetAccountCredential(ctx, &pb.GetAccountCredentialRequest{AccountId: accountID})
	if err != nil {
		return "", err
	}
	password := strings.TrimSpace(resp.GetCredential().GetPassword())
	if password == "" {
		return "", fmt.Errorf("account password is required")
	}
	return password, nil
}

func (s *Server) updateAccount(ctx context.Context, account *pb.Account) error {
	_, err := s.accountClient.UpdateAccount(ctx, &pb.UpdateAccountRequest{Account: account})
	return err
}

func (s *Server) createJob(ctx context.Context, accountID, action string, params map[string]string) (*db.Job, error) {
	return s.jobStore.Create(ctx, accountID, action, params)
}

func (s *Server) setJobParams(ctx context.Context, jobID string, params map[string]string) error {
	return s.jobStore.SetParams(ctx, jobID, params)
}

func (s *Server) getJobParam(ctx context.Context, jobID, key string) (string, bool, error) {
	return s.jobStore.GetParam(ctx, jobID, key)
}

func (s *Server) deleteJobParam(ctx context.Context, jobID, key string) error {
	return s.jobStore.DeleteParam(ctx, jobID, key)
}

func (s *Server) updateJob(ctx context.Context, jobID, statusValue, errorMessage string, result activityStepResult) {
	s.jobStore.Update(ctx, jobID, statusValue, errorMessage, result)
}

func (s *Server) getJob(ctx context.Context, jobID string) (*db.Job, error) {
	return s.jobStore.Get(ctx, jobID)
}

func (s *Server) runAtomicStep(ctx context.Context, jobID, stepName string, recoverable bool, retryable bool, fn func() (activityStepResult, error)) (activityStepResult, error) {
	return s.jobStore.RunAtomicStep(ctx, jobID, stepName, recoverable, retryable, fn)
}

func (s *Server) updateRunningStepData(ctx context.Context, jobID, stepName string, result activityStepResult) {
	s.jobStore.UpdateRunningStepData(ctx, jobID, stepName, result)
}

func failedStatus(recoverable bool, retryable bool) string {
	return jobstatus.Failed(recoverable, retryable)
}
