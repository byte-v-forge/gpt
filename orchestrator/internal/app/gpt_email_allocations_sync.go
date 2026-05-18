package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"orchestrator/pb"
)

const (
	gptEmailStatusAvailable         = "AVAILABLE"
	gptEmailStatusAssigned          = "ASSIGNED"
	gptEmailStatusRegistered        = "REGISTERED"
	gptEmailStatusOAuthPending      = "OAUTH_PENDING"
	gptEmailStatusAuthFailed        = "AUTH_FAILED"
	gptEmailStatusNeedsManualVerify = "NEEDS_MANUAL_VERIFICATION"
	gptEmailStatusUserAlreadyExists = "USER_ALREADY_EXISTS"
	gptEmailStatusRegistrationFail  = "REGISTRATION_FAILED"
	gptEmailStatusBlocked           = "BLOCKED"
	emailAuthStatusAuthorized       = "AUTHORIZED"
	emailAuthStatusOAuthPending     = "OAUTH_PENDING"
	emailAuthStatusAuthFailed       = "AUTH_FAILED"
	emailAuthStatusNeedsManual      = "NEEDS_MANUAL_VERIFICATION"
)

func syncGPTEmailAllocationsFromMailboxes(deps *orchestratorDependencies) error {
	if deps == nil || deps.emailClient == nil || deps.accountClient == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := deps.emailClient.ListMailboxes(ctx, &pb.ListEmailMailboxesRequest{Limit: 500})
	if err != nil {
		return fmt.Errorf("list mailboxes: %w", err)
	}
	var synced int
	for _, mailbox := range resp.GetMailboxes() {
		email := strings.ToLower(strings.TrimSpace(mailbox.GetEmailAddress()))
		if email == "" {
			continue
		}
		primaryEmail := strings.ToLower(strings.TrimSpace(mailbox.GetPrimaryEmail()))
		if primaryEmail == "" {
			if mailbox.GetIsPrimary() {
				primaryEmail = email
			} else {
				primaryEmail = canonicalEmail(email)
			}
		}
		isPrimary := mailbox.GetIsPrimary() || primaryEmail == email
		status := allocationStatusFromMailbox(mailbox)
		splittable := isPrimary && status == gptEmailStatusRegistered
		if _, err := deps.accountClient.UpsertGPTEmailAllocation(ctx, &pb.UpsertGPTEmailAllocationRequest{
			Allocation: &pb.GPTEmailAllocation{
				Email:        email,
				PrimaryEmail: primaryEmail,
				IsPrimary:    isPrimary,
				Status:       status,
				Splittable:   splittable,
				LastError:    strings.TrimSpace(mailbox.GetLastError()),
			},
		}); err != nil {
			return fmt.Errorf("upsert GPT email allocation %s: %w", redactEmail(email), err)
		}
		synced++
	}
	log.Printf("synced %d GPT email allocation rows from mailbox service", synced)
	return nil
}

func allocationStatusFromMailbox(mailbox *pb.EmailMailbox) string {
	status := strings.TrimSpace(mailbox.GetStatus())
	switch status {
	case gptEmailStatusAssigned, gptEmailStatusRegistered, gptEmailStatusUserAlreadyExists, gptEmailStatusRegistrationFail, gptEmailStatusBlocked:
		return status
	}
	authStatus := mailboxAuthStatus(mailbox)
	switch authStatus {
	case emailAuthStatusAuthorized:
		return gptEmailStatusAvailable
	case emailAuthStatusAuthFailed:
		return gptEmailStatusAuthFailed
	case emailAuthStatusNeedsManual:
		return gptEmailStatusNeedsManualVerify
	default:
		return gptEmailStatusOAuthPending
	}
}

func mailboxAuthStatus(mailbox *pb.EmailMailbox) string {
	if mailbox == nil {
		return emailAuthStatusOAuthPending
	}
	authStatus := strings.TrimSpace(mailbox.GetAuthStatus())
	if authStatus != "" {
		return authStatus
	}
	if strings.TrimSpace(mailbox.GetRefreshToken()) != "" {
		return emailAuthStatusAuthorized
	}
	return emailAuthStatusOAuthPending
}

func canonicalEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	local, domain, ok := strings.Cut(normalized, "@")
	if !ok || local == "" || domain == "" {
		return normalized
	}
	local, _, _ = strings.Cut(local, "+")
	return local + "@" + domain
}

func redactEmail(email string) string {
	email = strings.TrimSpace(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "<redacted>"
	}
	local := parts[0]
	if len(local) > 2 {
		local = local[:2]
	}
	return local + "***@" + parts[1]
}
