package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/gptaccount"
)

const (
	codexPhoneStatusConfirmed      = "CONFIRMED"
	codexPhoneStatusOAuthNeedPhone = "OAUTH_NEED_PHONE"
)

func (s *Server) markCodexOAuthNeedPhone(ctx context.Context, accountID, label string, data *codexOAuthStepData) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	account := gptaccount.Patch(accountID)
	gptaccount.SetCredential(account, gptaccount.CredentialKindCodexPhone, false, codexPhoneStatusOAuthNeedPhone, time.Now())
	if err := s.updateAccount(ctx, account); err != nil {
		return fmt.Errorf("save codex phone need state to account db: %w", err)
	}
	if data != nil {
		data.setAccountPhoneStatus(codexPhoneStatusOAuthNeedPhone)
		data.setAccountPhoneNeedWritten(true)
	}
	return nil
}

func (s *Server) markCodexOAuthPhoneConfirmed(ctx context.Context, accountID, label string, data *codexOAuthStepData) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	account := gptaccount.Patch(accountID)
	gptaccount.SetCredential(account, gptaccount.CredentialKindCodexPhone, true, codexPhoneStatusConfirmed, time.Now())
	if err := s.updateAccount(ctx, account); err != nil {
		return fmt.Errorf("save codex phone confirmed state to account db: %w", err)
	}
	if data != nil {
		data.setAccountPhoneStatus(codexPhoneStatusConfirmed)
		data.setAccountPhoneConfirmedWritten(true)
	}
	return nil
}
