package activities

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/common-lib/randx"
	"strings"

	"orchestrator/pb"
)

const (
	codexOAuthBrowserMode      = "codex_oauth_add_phone"
	codexOAuthPKCESecretPrefix = "codex_oauth_pkce:"
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

func (s *Server) newCodexOAuthBrowserStartFlow(ctx context.Context, account *pb.Account, jobID, label string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, allowAddPhone bool, data map[string]any) (*codexOAuthBrowserFlow, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if allowAddPhone && phone != nil {
		if err := ensureCodexOAuthPhoneUsableForSMS(phone, cfg); err != nil {
			return nil, err
		}
	}
	pkce, err := newCodexOAuthPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randx.Base64URL(32)
	if err != nil {
		return nil, err
	}
	return &codexOAuthBrowserFlow{
		server:        s,
		ctx:           ctx,
		account:       account,
		jobID:         jobID,
		label:         label,
		phone:         phone,
		cfg:           cfg,
		allowAddPhone: allowAddPhone,
		data:          data,
		pkce:          pkce,
		state:         state,
		authorizeURL:  buildCodexOAuthAuthorizeURL(cfg, pkce, state),
		browserFlow:   newCodexOAuthBrowserAuthFlow(ctx, jobID, account, stepCodexOAuthBrowserStart),
		failure:       "codex oauth browser did not complete",
	}, nil
}

func (s *Server) newCodexOAuthBrowserSessionFlow(ctx context.Context, account *pb.Account, jobID, label string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, allowAddPhone bool, markPhoneConfirmed bool, session *CodexOAuthBrowserSession, data map[string]any, taskScope string) (*codexOAuthBrowserFlow, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if session == nil || strings.TrimSpace(session.GetFlowId()) == "" || strings.TrimSpace(session.GetSessionId()) == "" {
		return nil, fmt.Errorf("codex oauth browser session is required")
	}
	if allowAddPhone {
		if err := ensureCodexOAuthPhoneUsableForSMS(phone, cfg); err != nil {
			return nil, err
		}
	}
	browserFlow := newCodexOAuthBrowserAuthFlow(ctx, jobID, account, taskScope)
	browserFlow.flowID = strings.TrimSpace(session.GetFlowId())
	browserFlow.sessionID = strings.TrimSpace(session.GetSessionId())
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
		state:              strings.TrimSpace(session.GetState()),
		browserFlow:        browserFlow,
		failure:            "codex oauth browser did not complete",
	}, nil
}

func newCodexOAuthBrowserAuthFlow(ctx context.Context, jobID string, account *pb.Account, taskScope string) *browserAuthFlow {
	flow := newBrowserAuthFlow(codexOAuthBrowserMode, jobID, account)
	flow.setTaskScope(taskScope)
	flow.cancel()
	flow.ctx, flow.cancel = context.WithCancel(ctx)
	return flow
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

func codexOAuthPKCESecretKey(jobID, flowID string) string {
	return codexOAuthPKCESecretPrefix + strings.TrimSpace(jobID) + ":" + strings.TrimSpace(flowID)
}

func ensureCodexOAuthPhoneUsableForSMS(phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig) error {
	if err := ensureCodexOAuthPhoneLeaseUsable(phone, cfg); err != nil {
		return err
	}
	return validateCodexOAuthSMSCountry(phone.GetCountryIso2())
}
