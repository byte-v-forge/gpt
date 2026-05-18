package activities

import (
	"context"
	"fmt"
	"orchestrator/pb"
	"sort"
	"strings"
)

const accountEmailAllocatorLimit int32 = 500

type AccountEmailAllocator interface {
	Allocate(ctx context.Context, accountID string, excludes []string) (string, error)
}

type accountDBEmailAllocator struct {
	accountClient pb.AccountDatabaseServiceClient
}

func defaultAccountEmailAllocator(allocator AccountEmailAllocator, accountClient pb.AccountDatabaseServiceClient) AccountEmailAllocator {
	if allocator != nil {
		return allocator
	}
	if accountClient == nil {
		return nil
	}
	return &accountDBEmailAllocator{accountClient: accountClient}
}

func NewAccountEmailAllocator(accountClient pb.AccountDatabaseServiceClient) AccountEmailAllocator {
	return defaultAccountEmailAllocator(nil, accountClient)
}

func (a *accountDBEmailAllocator) Allocate(ctx context.Context, accountID string, excludes []string) (string, error) {
	if a == nil || a.accountClient == nil {
		return "", fmt.Errorf("email allocator is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return "", fmt.Errorf("account_id is required for email allocation")
	}

	excludeSet := normalizedSet(excludes)
	available, err := a.listAllocations(ctx, emailStatusAvailable, false)
	if err != nil {
		return "", err
	}
	sortAllocationsOldestFirst(available)

	for _, allocation := range available {
		if !eligibleAvailablePrimaryAllocation(allocation, excludeSet) {
			continue
		}
		email, claimed, err := a.claim(ctx, allocation.GetEmail(), accountID, false)
		if err != nil {
			return "", err
		}
		if claimed {
			return email, nil
		}
	}

	for _, allocation := range available {
		if !eligibleAvailableAliasAllocation(allocation, excludeSet) {
			continue
		}
		email, claimed, err := a.claim(ctx, allocation.GetEmail(), accountID, true)
		if err != nil {
			return "", err
		}
		if claimed {
			return email, nil
		}
	}

	registered, err := a.listAllocations(ctx, emailStatusRegistered, true)
	if err != nil {
		return "", err
	}
	sortAllocationsOldestFirst(registered)

	for _, allocation := range registered {
		if !eligibleRegisteredPrimaryAllocation(allocation, excludeSet) {
			continue
		}
		email, created, err := a.createAlias(ctx, allocation.GetEmail(), accountID)
		if err != nil {
			return "", err
		}
		if created {
			return email, nil
		}
	}

	return "", fmt.Errorf("no available GPT email allocation")
}

func (a *accountDBEmailAllocator) listAllocations(ctx context.Context, status string, splittableOnly bool) ([]*pb.GPTEmailAllocation, error) {
	resp, err := a.accountClient.ListGPTEmailAllocations(ctx, &pb.ListGPTEmailAllocationsRequest{
		Status:         status,
		Limit:          accountEmailAllocatorLimit,
		SplittableOnly: splittableOnly,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetAllocations(), nil
}

func (a *accountDBEmailAllocator) claim(ctx context.Context, email string, accountID string, requirePrimarySplittable bool) (string, bool, error) {
	req := &pb.ClaimGPTEmailAllocationRequest{
		Email:                    strings.TrimSpace(email),
		AccountId:                strings.TrimSpace(accountID),
		ExpectedStatus:           emailStatusAvailable,
		Status:                   emailStatusAssigned,
		RequirePrimarySplittable: requirePrimarySplittable,
		ExpectedPrimaryStatus:    "",
	}
	if requirePrimarySplittable {
		req.ExpectedPrimaryStatus = emailStatusRegistered
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
	return allocation.GetIsPrimary() && strings.TrimSpace(allocation.GetStatus()) == emailStatusAvailable
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
		strings.TrimSpace(allocation.GetStatus()) == emailStatusAvailable &&
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
		strings.TrimSpace(allocation.GetStatus()) == emailStatusRegistered
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

func sortAllocationsOldestFirst(allocations []*pb.GPTEmailAllocation) {
	sort.SliceStable(allocations, func(i, j int) bool {
		left := allocations[i]
		right := allocations[j]
		if left.GetUpdatedAt() != right.GetUpdatedAt() {
			return left.GetUpdatedAt() < right.GetUpdatedAt()
		}
		if left.GetCreatedAt() != right.GetCreatedAt() {
			return left.GetCreatedAt() < right.GetCreatedAt()
		}
		return strings.TrimSpace(left.GetEmail()) < strings.TrimSpace(right.GetEmail())
	})
}
