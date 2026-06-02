package activities

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
	"strings"
)

func boolPtr(value bool) *bool {
	return &value
}

func rejectUserAlreadyExistsAccount(account *pb.Account) error {
	if account != nil && isUserAlreadyExistsStatus(gptaccount.Status(account)) {
		return fmt.Errorf("account user already exists; delete only")
	}
	return nil
}

func accountRef(account *pb.Account) *AccountRef {
	if account == nil {
		return nil
	}
	return &AccountRef{
		AccountId:         gptaccount.ID(account),
		PlusTrialKnown:    account.PlusTrialEligible != nil,
		PlusTrialEligible: account.GetPlusTrialEligible(),
		PlusActive:        account.GetPlusActive(),
		Tier:              normalizeTier(account.GetTier()),
	}
}

func (s *Server) CreateJobActivity(ctx context.Context, input *CreateJobInput) error {
	_, err := s.createJobWithID(ctx, input.GetJobId(), input.GetAccountId(), input.GetAction(), input.GetParams())
	return err
}

func (s *Server) EnsureAccountActivity(ctx context.Context, input *EnsureAccountInput) (*AccountRef, error) {
	spec := input.GetAccount()
	if spec.GetAccountId() == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	if account, err := s.getAccount(ctx, spec.GetAccountId()); err == nil {
		if err := rejectUserAlreadyExistsAccount(account); err != nil {
			return nil, err
		}
		if strings.TrimSpace(gptaccount.Email(account)) == "" {
			return nil, fmt.Errorf("account email is required")
		}
		if err := s.generateAccountFingerprint(ctx, gptaccount.ID(account), accountfingerprint.GenerateParams{
			CountryCode: spec.GetCountryCode(),
			Region:      spec.GetRegion(),
		}); err != nil {
			return nil, err
		}
		return accountRef(account), nil
	}

	email := spec.Email
	if strings.TrimSpace(email) == "" {
		var err error
		email, err = s.acquireEmail(ctx, spec.GetAccountId(), nil, accountEmailStrategy(spec))
		if err != nil {
			return nil, err
		}
	}

	resp, err := s.accountClient.CreateAccount(ctx, &pb.CreateAccountRequest{
		Account:    gptaccount.New(spec.GetAccountId(), email, gptplugin.AccountStatusUnregistered),
		Credential: &pb.AccountCredential{Password: spec.GetPassword()},
	})
	if err != nil {
		if account, getErr := s.getAccount(ctx, spec.GetAccountId()); getErr == nil {
			if err := rejectUserAlreadyExistsAccount(account); err != nil {
				return nil, err
			}
			return accountRef(account), nil
		}
		return nil, err
	}
	if resp.GetAccount() == nil || gptaccount.ID(resp.GetAccount()) == "" {
		return nil, fmt.Errorf("gpt-account returned empty account")
	}
	if err := s.generateAccountFingerprint(ctx, gptaccount.ID(resp.GetAccount()), accountfingerprint.GenerateParams{
		CountryCode: spec.GetCountryCode(),
		Region:      spec.GetRegion(),
	}); err != nil {
		return nil, err
	}
	return accountRef(resp.GetAccount()), nil
}

func (s *Server) acquireEmail(ctx context.Context, accountID string, excludes []string, strategy pb.AccountEmailStrategy) (string, error) {
	allocator := s.emailAllocator
	if allocator == nil {
		allocator = defaultAccountEmailAllocator(nil, s.accountClient)
	}
	email, err := allocator.Allocate(ctx, accountID, excludes, strategy)
	if err != nil {
		return "", err
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("email allocator returned empty email")
	}
	return email, nil
}

func accountEmailStrategy(spec *pb.AccountSpec) pb.AccountEmailStrategy {
	if spec == nil || spec.GetEmailStrategy() == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED {
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS
	}
	return spec.GetEmailStrategy()
}

func (s *Server) ResolveAccountFromJobActivity(ctx context.Context, input *ResolveAccountInput) (*AccountRef, error) {
	if input.GetAccountId() != "" {
		account, err := s.getAccount(ctx, input.GetAccountId())
		if err != nil {
			return nil, err
		}
		if err := rejectUserAlreadyExistsAccount(account); err != nil {
			return nil, err
		}
		return accountRef(account), nil
	}
	job, err := s.getJob(ctx, input.GetSourceJobId())
	if err != nil {
		return nil, err
	}
	account, err := s.getAccount(ctx, job.AccountID)
	if err != nil {
		return nil, err
	}
	if err := rejectUserAlreadyExistsAccount(account); err != nil {
		return nil, err
	}
	return accountRef(account), nil
}
