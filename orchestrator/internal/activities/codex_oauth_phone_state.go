package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/pb"
)

const (
	codexPhoneStatusConfirmed      = "CONFIRMED"
	codexPhoneStatusOAuthNeedPhone = "OAUTH_NEED_PHONE"
)

func (s *Server) markCodexOAuthNeedPhone(ctx context.Context, accountID, label string, data map[string]any) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	statusLabel := strings.TrimSpace(label)
	if statusLabel == "" {
		statusLabel = "OAuth Need Phone"
	}
	if err := s.updateAccount(ctx, &pb.Account{
		AccountId:               accountID,
		CodexPhoneConfirmed:     boolPtr(false),
		CodexPhoneLabel:         statusLabel,
		CodexPhoneUpdatedAtUnix: time.Now().Unix(),
		CodexPhoneStatus:        codexPhoneStatusOAuthNeedPhone,
	}); err != nil {
		return fmt.Errorf("save codex phone need state to account db: %w", err)
	}
	if data != nil {
		data["account_phone_status"] = codexPhoneStatusOAuthNeedPhone
		data["account_phone_need_written"] = true
	}
	return nil
}

func (s *Server) markCodexOAuthPhoneConfirmed(ctx context.Context, accountID, label string, data map[string]any) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	if err := s.updateAccount(ctx, &pb.Account{
		AccountId:               accountID,
		CodexPhoneConfirmed:     boolPtr(true),
		CodexPhoneLabel:         strings.TrimSpace(label),
		CodexPhoneUpdatedAtUnix: time.Now().Unix(),
		CodexPhoneStatus:        codexPhoneStatusConfirmed,
	}); err != nil {
		return fmt.Errorf("save codex phone confirmed state to account db: %w", err)
	}
	if data != nil {
		data["account_phone_status"] = codexPhoneStatusConfirmed
		data["account_phone_confirmed_written"] = true
	}
	return nil
}
