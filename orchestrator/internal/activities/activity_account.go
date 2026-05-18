package activities

import (
	"context"
	"fmt"
	"orchestrator/pb"
	"strings"
)

func boolPtr(value bool) *bool {
	return &value
}

func rejectUserAlreadyExistsAccount(account *pb.Account) error {
	if account != nil && isUserAlreadyExistsStatus(account.GetStatus()) {
		return fmt.Errorf("account user already exists; delete only")
	}
	return nil
}

func accountRef(account *pb.Account) AccountRef {
	if account == nil {
		return AccountRef{}
	}
	return AccountRef{
		AccountId:         account.GetAccountId(),
		PlusTrialKnown:    account.PlusTrialEligible != nil,
		PlusTrialEligible: account.GetPlusTrialEligible(),
		PlusActive:        account.GetPlusActive(),
		Tier:              normalizeTier(account.GetTier()),
	}
}

func isFreeTrialIneligibleError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "checkout amount") && strings.Contains(text, "not free-trial 0")
}

func accountEligibleForActivation(account *pb.Account) error {
	if account == nil {
		return fmt.Errorf("account is required")
	}
	if err := rejectUserAlreadyExistsAccount(account); err != nil {
		return err
	}
	tier := normalizeTier(account.GetTier())
	if tier != "free" {
		if tier == "" {
			return fmt.Errorf("account tier is unknown; probe tier before activation")
		}
		return fmt.Errorf("account tier %q cannot be activated; only free tier with trial eligibility is allowed", tier)
	}
	if account.PlusTrialEligible == nil {
		return fmt.Errorf("plus trial eligibility is unknown; probe trial eligibility before activation")
	}
	if !account.GetPlusTrialEligible() {
		return fmt.Errorf("account is not plus trial eligible")
	}
	return nil
}

func (s *Server) CreateJobActivity(ctx context.Context, input CreateJobInput) error {
	_, err := s.createJobWithID(ctx, input.GetJobId(), input.GetAccountId(), input.GetAction(), input.GetParams())
	return err
}

func (s *Server) EnsureAccountActivity(ctx context.Context, input EnsureAccountInput) (AccountRef, error) {
	spec := input.GetAccount()
	if spec.GetAccountId() == "" {
		return AccountRef{}, fmt.Errorf("account_id is required")
	}

	if account, err := s.getAccount(ctx, spec.GetAccountId()); err == nil {
		if err := rejectUserAlreadyExistsAccount(account); err != nil {
			return AccountRef{}, err
		}
		if strings.TrimSpace(account.GetEmail()) == "" {
			return AccountRef{}, fmt.Errorf("account email is required")
		}
		return accountRef(account), nil
	}

	email := spec.Email
	if strings.TrimSpace(email) == "" {
		var err error
		email, err = s.acquireEmail(ctx, spec.GetAccountId(), nil)
		if err != nil {
			return AccountRef{}, err
		}
	}

	resp, err := s.accountClient.CreateAccount(ctx, &pb.CreateAccountRequest{Account: &pb.Account{
		AccountId: spec.GetAccountId(),
		Email:     email,
		Password:  spec.GetPassword(),
		Status:    statusCreated,
	}})
	if err != nil {
		if account, getErr := s.getAccount(ctx, spec.GetAccountId()); getErr == nil {
			if err := rejectUserAlreadyExistsAccount(account); err != nil {
				return AccountRef{}, err
			}
			return accountRef(account), nil
		}
		return AccountRef{}, err
	}
	if resp.GetAccount() == nil || resp.GetAccount().GetAccountId() == "" {
		return AccountRef{}, fmt.Errorf("account-db returned empty account")
	}
	return accountRef(resp.GetAccount()), nil
}

func (s *Server) acquireEmail(ctx context.Context, accountID string, excludes []string) (string, error) {
	allocator := s.emailAllocator
	if allocator == nil {
		allocator = defaultAccountEmailAllocator(nil, s.accountClient)
	}
	email, err := allocator.Allocate(ctx, accountID, excludes)
	if err != nil {
		return "", err
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("email allocator returned empty email")
	}
	return email, nil
}

func (s *Server) ResolveAccountFromJobActivity(ctx context.Context, input ResolveAccountInput) (AccountRef, error) {
	if input.GetAccountId() != "" {
		account, err := s.getAccount(ctx, input.GetAccountId())
		if err != nil {
			return AccountRef{}, err
		}
		if err := rejectUserAlreadyExistsAccount(account); err != nil {
			return AccountRef{}, err
		}
		return accountRef(account), nil
	}
	job, err := s.getJob(ctx, input.GetSourceJobId())
	if err != nil {
		return AccountRef{}, err
	}
	account, err := s.getAccount(ctx, job.AccountID)
	if err != nil {
		return AccountRef{}, err
	}
	if err := rejectUserAlreadyExistsAccount(account); err != nil {
		return AccountRef{}, err
	}
	return accountRef(account), nil
}
