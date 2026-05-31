package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/gptaccount"
	"strings"

	"orchestrator/pb"
)

type codexOAuthBrowserResult struct {
	authSecretKey     string
	phoneReuseCount   int32
	phoneReuseLimit   int32
	addPhoneConfirmed bool
	addPhoneRequired  bool
}

func (s *Server) codexOAuthBrowserAccount(ctx context.Context, accountID string) (*pb.Account, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(gptaccount.Email(account)) == "" {
		return nil, fmt.Errorf("account email is required")
	}
	return account, nil
}

func codexOAuthBrowserData(label string, phone *CodexOAuthPhoneLease) *codexOAuthStepData {
	return newCodexOAuthStepData(label, phone)
}

func codexOAuthAddPhoneRequiredError() error {
	return fmt.Errorf("codex_oauth_add_phone_required")
}
