package activities

import (
	"context"
	"fmt"

	"orchestrator/pb"
)

type codexOAuthBrowserFlow struct {
	server             *Server
	ctx                context.Context
	account            *pb.Account
	jobID              string
	label              string
	phone              *CodexOAuthPhoneLease
	cfg                CodexOAuthConfig
	allowAddPhone      bool
	markPhoneConfirmed bool
	data               map[string]any

	pkce         codexOAuthPKCE
	state        string
	authorizeURL string
	browserFlow  *browserAuthFlow

	authJSON    []byte
	secretKey   string
	phoneUsed   bool
	success     bool
	failure     string
	phoneAdded  bool
	phoneNeeded bool
}

func (s *Server) newCodexOAuthBrowserFlow(ctx context.Context, account *pb.Account, jobID, label string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, allowAddPhone bool, markPhoneConfirmed bool, data map[string]any) (*codexOAuthBrowserFlow, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if allowAddPhone {
		if err := ensureCodexOAuthPhoneLeaseUsable(phone, cfg); err != nil {
			return nil, err
		}
	}
	pkce, err := newCodexOAuthPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomURLToken(32)
	if err != nil {
		return nil, err
	}
	return &codexOAuthBrowserFlow{
		server:             s,
		ctx:                ctx,
		account:            account,
		jobID:              jobID,
		label:              label,
		phone:              phone,
		cfg:                cfg,
		allowAddPhone:      allowAddPhone,
		markPhoneConfirmed: markPhoneConfirmed,
		data:               data,
		pkce:               pkce,
		state:              state,
		authorizeURL:       buildCodexOAuthAuthorizeURL(cfg, pkce, state),
		browserFlow:        newBrowserAuthFlow("codex_oauth_add_phone", jobID, account),
		failure:            "codex oauth browser did not complete",
	}, nil
}

func (f *codexOAuthBrowserFlow) startSession() error {
	return f.browserFlow.startSession(f.server.browserAutomationClient, f.server.browserAuthConfig)
}

func (f *codexOAuthBrowserFlow) stopSession() {
	f.browserFlow.stopSession(f.server.browserAutomationClient)
}

func (f *codexOAuthBrowserFlow) releasePhoneOnFailure() {
	if f.success {
		return
	}
	_ = f.server.releaseCodexPhone(f.ctx, f.phone, f.account.GetAccountId(), f.jobID, f.label, f.phoneUsed, f.failure)
}

func (f *codexOAuthBrowserFlow) fail(err error) error {
	if err != nil {
		f.failure = err.Error()
	}
	return err
}

func (f *codexOAuthBrowserFlow) result() codexOAuthBrowserResult {
	reuseCount := int32(0)
	reuseLimit := int32(0)
	if f.phone != nil {
		reuseCount = f.phone.GetReuseCount()
		reuseLimit = f.phone.GetReuseLimit()
	}
	return codexOAuthBrowserResult{
		authSecretKey:     f.secretKey,
		phoneReuseCount:   reuseCount,
		phoneReuseLimit:   reuseLimit,
		addPhoneConfirmed: f.phoneAdded,
		addPhoneRequired:  f.phoneNeeded,
	}
}
