package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"orchestrator/pb"
)

const accountEmailAllocatorPageLimit int32 = 100

type AccountEmailAllocator interface {
	Allocate(ctx context.Context, accountID string, excludes []string, strategy pb.AccountEmailStrategy) (string, error)
}

type accountDBEmailAllocator struct {
	accountClient pb.GPTAccountServiceClient
}

func defaultAccountEmailAllocator(allocator AccountEmailAllocator, accountClient pb.GPTAccountServiceClient) AccountEmailAllocator {
	if allocator != nil {
		return allocator
	}
	if accountClient == nil {
		return nil
	}
	return &accountDBEmailAllocator{accountClient: accountClient}
}

func NewAccountEmailAllocator(accountClient pb.GPTAccountServiceClient) AccountEmailAllocator {
	return defaultAccountEmailAllocator(nil, accountClient)
}

func (a *accountDBEmailAllocator) Allocate(ctx context.Context, accountID string, excludes []string, strategy pb.AccountEmailStrategy) (string, error) {
	if a == nil || a.accountClient == nil {
		return "", fmt.Errorf("email allocator is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return "", fmt.Errorf("account_id is required for email allocation")
	}
	strategy = normalizeEmailStrategy(strategy)
	if strategy == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_EXPLICIT {
		return "", fmt.Errorf("explicit email strategy requires an email")
	}

	excludeSet := normalizedSet(excludes)
	email, found, err := a.claimAvailablePrimary(ctx, accountID, excludeSet)
	if err != nil {
		return "", err
	}
	if found {
		return email, nil
	}

	if strategyAllowsAlias(strategy) {
		email, found, err := a.claimAvailableAlias(ctx, accountID, excludeSet)
		if err != nil {
			return "", err
		}
		if found {
			return email, nil
		}

		email, found, err = a.createRegisteredPrimaryAlias(ctx, accountID, excludeSet)
		if err != nil {
			return "", err
		}
		if found {
			return email, nil
		}
	}

	return "", fmt.Errorf("no available GPT email allocation")
}

func normalizeEmailStrategy(strategy pb.AccountEmailStrategy) pb.AccountEmailStrategy {
	if strategy == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED {
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS
	}
	return strategy
}

func strategyAllowsAlias(strategy pb.AccountEmailStrategy) bool {
	return normalizeEmailStrategy(strategy) == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS
}

func (a *accountDBEmailAllocator) claimAvailablePrimary(
	ctx context.Context,
	accountID string,
	excludes map[string]struct{},
) (string, bool, error) {
	options := allocationPageOptions{status: gptplugin.EmailStatusAvailable, isPrimary: boolPtr(true)}
	return a.findAndClaimAllocation(ctx, options, func(allocation *pb.GPTEmailAllocation) (string, bool, error) {
		if eligibleAvailablePrimaryAllocation(allocation, excludes) {
			return a.claim(ctx, allocation.GetEmail(), accountID, false)
		}
		return "", false, nil
	})
}

func (a *accountDBEmailAllocator) claimAvailableAlias(
	ctx context.Context,
	accountID string,
	excludes map[string]struct{},
) (string, bool, error) {
	options := allocationPageOptions{status: gptplugin.EmailStatusAvailable, isPrimary: boolPtr(false)}
	return a.findAndClaimAllocation(ctx, options, func(allocation *pb.GPTEmailAllocation) (string, bool, error) {
		if eligibleAvailableAliasAllocation(allocation, excludes) {
			return a.claim(ctx, allocation.GetEmail(), accountID, true)
		}
		return "", false, nil
	})
}

func (a *accountDBEmailAllocator) createRegisteredPrimaryAlias(
	ctx context.Context,
	accountID string,
	excludes map[string]struct{},
) (string, bool, error) {
	options := allocationPageOptions{status: gptplugin.EmailStatusRegistered, splittableOnly: true}
	return a.findAndClaimAllocation(ctx, options, func(allocation *pb.GPTEmailAllocation) (string, bool, error) {
		if eligibleRegisteredPrimaryAllocation(allocation, excludes) {
			return a.createAlias(ctx, allocation.GetEmail(), accountID)
		}
		return "", false, nil
	})
}

type allocationPageOptions struct {
	status         string
	splittableOnly bool
	isPrimary      *bool
}

func (a *accountDBEmailAllocator) findAndClaimAllocation(
	ctx context.Context,
	options allocationPageOptions,
	claim func(*pb.GPTEmailAllocation) (string, bool, error),
) (string, bool, error) {
	cursor := ""
	for {
		resp, err := a.accountClient.ListGPTEmailAllocations(ctx, &pb.ListGPTEmailAllocationsRequest{
			Status:         options.status,
			Limit:          accountEmailAllocatorPageLimit,
			SplittableOnly: options.splittableOnly,
			IsPrimary:      options.isPrimary,
			Cursor:         cursor,
		})
		if err != nil {
			return "", false, err
		}
		for _, allocation := range resp.GetAllocations() {
			email, claimed, err := claim(allocation)
			if err != nil || claimed {
				return email, claimed, err
			}
		}
		cursor = strings.TrimSpace(resp.GetNextCursor())
		if cursor == "" {
			return "", false, nil
		}
	}
}

func (a *accountDBEmailAllocator) claim(ctx context.Context, email string, accountID string, requirePrimarySplittable bool) (string, bool, error) {
	req := &pb.ClaimGPTEmailAllocationRequest{
		Email:                    strings.TrimSpace(email),
		AccountId:                strings.TrimSpace(accountID),
		ExpectedStatus:           gptplugin.EmailStatusAvailable,
		Status:                   gptplugin.EmailStatusAssigned,
		RequirePrimarySplittable: requirePrimarySplittable,
		ExpectedPrimaryStatus:    "",
	}
	if requirePrimarySplittable {
		req.ExpectedPrimaryStatus = gptplugin.EmailStatusRegistered
	}
	resp, err := a.accountClient.ClaimGPTEmailAllocation(ctx, req)
	if err != nil {
		return "", false, err
	}
	if resp == nil || !resp.GetClaimed() || strings.TrimSpace(resp.GetAllocation().GetEmail()) == "" {
		return "", false, nil
	}
	return strings.TrimSpace(resp.GetAllocation().GetEmail()), true, nil
}

func (a *accountDBEmailAllocator) createAlias(ctx context.Context, primaryEmail string, accountID string) (string, bool, error) {
	resp, err := a.accountClient.CreateGPTEmailAliasAllocation(ctx, &pb.CreateGPTEmailAliasAllocationRequest{
		PrimaryEmail: strings.TrimSpace(primaryEmail),
		AccountId:    strings.TrimSpace(accountID),
	})
	if err != nil {
		return "", false, err
	}
	if resp == nil || !resp.GetCreated() || strings.TrimSpace(resp.GetAllocation().GetEmail()) == "" {
		return "", false, nil
	}
	return strings.TrimSpace(resp.GetAllocation().GetEmail()), true, nil
}

func eligibleAvailablePrimaryAllocation(allocation *pb.GPTEmailAllocation, excludes map[string]struct{}) bool {
	email := normalizedEmail(allocation.GetEmail())
	if email == "" {
		return false
	}
	if _, ok := excludes[email]; ok {
		return false
	}
	return allocation.GetIsPrimary() && strings.TrimSpace(allocation.GetStatus()) == gptplugin.EmailStatusAvailable
}

func eligibleAvailableAliasAllocation(allocation *pb.GPTEmailAllocation, excludes map[string]struct{}) bool {
	email := normalizedEmail(allocation.GetEmail())
	if email == "" {
		return false
	}
	if _, ok := excludes[email]; ok {
		return false
	}
	return !allocation.GetIsPrimary() &&
		strings.TrimSpace(allocation.GetStatus()) == gptplugin.EmailStatusAvailable &&
		normalizedEmail(allocation.GetPrimaryEmail()) != ""
}

func eligibleRegisteredPrimaryAllocation(allocation *pb.GPTEmailAllocation, excludes map[string]struct{}) bool {
	email := normalizedEmail(allocation.GetEmail())
	if email == "" {
		return false
	}
	if _, ok := excludes[email]; ok {
		return false
	}
	return allocation.GetIsPrimary() &&
		allocation.GetSplittable() &&
		strings.TrimSpace(allocation.GetStatus()) == gptplugin.EmailStatusRegistered
}

func normalizedSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		if normalized := normalizedEmail(value); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func normalizedEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
