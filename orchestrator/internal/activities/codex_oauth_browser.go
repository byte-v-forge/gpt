package activities

import (
	"context"
	"fmt"

	"orchestrator/pb"
)

type codexOAuthBrowserResult struct {
	authSecretKey     string
	phoneReuseCount   int32
	phoneReuseLimit   int32
	addPhoneConfirmed bool
	addPhoneRequired  bool
}

func (s *Server) runCodexOAuthBrowser(ctx context.Context, account *pb.Account, jobID, label string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, allowAddPhone bool, markPhoneConfirmed bool, data map[string]any) (codexOAuthBrowserResult, error) {
	flow, err := s.newCodexOAuthBrowserFlow(ctx, account, jobID, label, phone, cfg, allowAddPhone, markPhoneConfirmed, data)
	if err != nil {
		return codexOAuthBrowserResult{}, err
	}
	if err := flow.startSession(); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	defer flow.stopSession()
	defer flow.releasePhoneOnFailure()

	if err := flow.openAuthorizeURL(); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	if err := flow.ensureLoggedIn(); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	addPhoneResult, err := flow.handleAddPhoneStage()
	if err != nil {
		return addPhoneResult, err
	}
	if err := flow.completeAuthorization(); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	if err := flow.persistAuthorization(); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	flow.success = true
	return flow.result(), nil
}

func codexOAuthAddPhoneRequiredError() error {
	return fmt.Errorf("codex_oauth_add_phone_required")
}
