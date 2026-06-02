package activities

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"orchestrator/internal/gptaccount"
	"strings"

	"orchestrator/db"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) PersistRegisteredActivity(ctx context.Context, input *PersistRegisteredInput) error {
	if err := s.saveChatGPTSessionToken(ctx, input.GetAccountId(), input.GetSessionToken()); err != nil {
		return err
	}
	if err := s.saveChatGPTAccessToken(ctx, input.GetAccountId(), input.GetAccessToken()); err != nil {
		return err
	}
	account := gptaccount.Patch(input.GetAccountId())
	gptaccount.SetStatus(account, gptplugin.AccountStatusRegistered, "")
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
	email := strings.TrimSpace(gptaccount.Email(registeredAccount))
	if email == "" {
		return nil
	}
	_, err = s.accountClient.MarkGPTEmailAllocationStatus(ctx, &pb.MarkGPTEmailAllocationStatusRequest{
		Email:  email,
		Status: gptplugin.EmailStatusRegistered,
	})
	return err
}

func (s *Server) MarkJobFailedActivity(ctx context.Context, input *JobFailureInput) error {
	if input.Status == "" {
		input.Status = failedStatus(input.Recoverable, input.Retryable)
	}
	s.updateJob(ctx, input.GetJobId(), input.GetStatus(), input.GetErrorMessage(), input.GetResult())
	if input.GetStepName() != "" {
		return s.markStepFailed(ctx, input)
	}
	return nil
}

func (s *Server) MarkJobSucceededActivity(ctx context.Context, input *JobSuccessInput) error {
	if err := s.ensureJobCanSucceed(ctx, input.GetJobId()); err != nil {
		return err
	}
	s.updateJob(ctx, input.GetJobId(), jobstatus.Succeeded, "", input.GetResult())
	return nil
}

func (s *Server) createJobWithID(ctx context.Context, jobID, accountID, action string, params map[string]string) (*db.Job, error) {
	return s.jobStore.CreateWithID(ctx, jobID, accountID, action, params)
}

func (s *Server) markStepFailed(ctx context.Context, input *JobFailureInput) error {
	return s.jobStore.MarkStepFailed(ctx, jobprojection.StepFailure{
		JobID:        input.GetJobId(),
		StepName:     input.GetStepName(),
		Status:       input.GetStatus(),
		Recoverable:  input.GetRecoverable(),
		Retryable:    input.GetRetryable(),
		ErrorMessage: input.GetErrorMessage(),
		Result:       input.GetResult(),
	})
}

func (s *Server) ensureJobCanSucceed(ctx context.Context, jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		return err
	}
	if terminalFailureStatus(job.Status) {
		return fmt.Errorf("job %s is already %s; refuse marking succeeded", jobID, job.Status)
	}
	steps, err := s.jobStore.Steps(ctx, jobID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if terminalFailureStatus(step.Status) {
			return fmt.Errorf("job %s has failed step %s; refuse marking succeeded", jobID, step.StepName)
		}
	}
	return nil
}

func terminalFailureStatus(status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	return status == jobstatus.Canceled || strings.HasPrefix(status, "FAILED_")
}
