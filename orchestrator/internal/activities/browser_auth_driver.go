package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"orchestrator/pb"
)

const (
	browserAuthStageQueued             = "queued"
	browserAuthStageStarting           = "starting"
	browserAuthStageEmailEntry         = "email_entry"
	browserAuthStageCredentialEntry    = "credential_entry"
	browserAuthStageOTPRequestClick    = "otp_request_click"
	browserAuthStageOTPRequestClicked  = "otp_request_clicked"
	browserAuthStageWaitingForOTP      = "waiting_for_otp"
	browserAuthStageOTPSubmit          = "otp_submit"
	browserAuthStageSessionCapture     = "session_capture"
	browserAuthStageSucceeded          = "succeeded"
	browserAuthStageFailed             = "failed"
	browserAuthStageCancelled          = "cancelled"
	browserAuthDefaultRegistrationName = "Alex Morgan"
	browserAuthDefaultBirthday         = "01/15/1990"
)

type BrowserAuthConfig struct {
	ProxyRef       string
	Locale         string
	AcceptLanguage string
	Timezone       string
	UserAgent      string
	WindowWidth    int
	WindowHeight   int
	SessionTTL     time.Duration
	CommandTimeout time.Duration
	BlockImages    bool
	GeoIP          bool
	Humanize       string
}

func (c BrowserAuthConfig) withDefaults() BrowserAuthConfig {
	if c.ProxyRef == "" {
		c.ProxyRef = "register"
	}
	if c.Locale == "" {
		c.Locale = "en-US"
	}
	if c.AcceptLanguage == "" {
		c.AcceptLanguage = "en-US,en;q=0.9"
	}
	if c.WindowWidth < 800 {
		c.WindowWidth = 1365
	}
	if c.WindowHeight < 600 {
		c.WindowHeight = 768
	}
	if c.SessionTTL <= 0 {
		c.SessionTTL = 30 * time.Minute
	}
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = 120 * time.Second
	}
	if c.Humanize == "" {
		c.Humanize = "true"
	}
	return c
}

type browserAuthFlowStore struct {
	mu    sync.Mutex
	flows map[string]*browserAuthFlow
}

func newBrowserAuthFlowStore() *browserAuthFlowStore {
	return &browserAuthFlowStore{flows: map[string]*browserAuthFlow{}}
}

func (s *browserAuthFlowStore) add(flow *browserAuthFlow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows[flow.flowID] = flow
}

func (s *browserAuthFlowStore) get(flowID string) *browserAuthFlow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flows[flowID]
}

func (s *browserAuthFlowStore) remove(flowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flows, flowID)
}

type browserAuthFlow struct {
	mu         sync.Mutex
	flowID     string
	mode       string
	jobID      string
	email      string
	password   string
	fullName   string
	birthday   string
	sessionID  string
	stage      string
	message    string
	startedAt  int64
	updatedAt  int64
	otpAction  int64
	otpWait    int64
	taskSeq    int64
	otpNeed    bool
	done       bool
	success    bool
	errMessage string
	result     *pb.RegisterResponse

	ctx      context.Context
	cancel   context.CancelFunc
	otpCh    chan string
	doneCh   chan struct{}
	doneOnce sync.Once
}

type browserAuthSession struct {
	sessionToken string
	accessToken  string
}

func newBrowserAuthFlow(mode, jobID string, account *pb.Account) *browserAuthFlow {
	now := time.Now().Unix()
	ctx, cancel := context.WithCancel(context.Background())
	fullName := strings.TrimSpace(strings.Join([]string{account.GetFirstName(), account.GetLastName()}, " "))
	if fullName == "" {
		fullName = browserAuthDefaultRegistrationName
	}
	birthday := strings.TrimSpace(account.GetDob())
	if birthday == "" {
		birthday = browserAuthDefaultBirthday
	}
	return &browserAuthFlow{
		flowID:    uuid.NewString(),
		mode:      strings.TrimSpace(mode),
		jobID:     strings.TrimSpace(jobID),
		email:     strings.TrimSpace(account.GetEmail()),
		password:  account.GetPassword(),
		fullName:  fullName,
		birthday:  birthday,
		stage:     browserAuthStageQueued,
		message:   "browser auth queued",
		startedAt: now,
		updatedAt: now,
		ctx:       ctx,
		cancel:    cancel,
		otpCh:     make(chan string, 1),
		doneCh:    make(chan struct{}),
	}
}

func (s *Server) browserAuthStart(ctx context.Context, mode, jobID string, account *pb.Account) (*pb.StartRegisterResponse, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	flow := newBrowserAuthFlow(mode, jobID, account)
	s.browserAuthFlows.add(flow)
	go flow.run(s.browserAutomationClient, s.browserAuthConfig)
	return flow.startResponse(), nil
}

func (s *Server) browserAuthComplete(ctx context.Context, mode, flowID, otp string) (*pb.RegisterResponse, error) {
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.RegisterResponse{Success: false, ErrorMessage: fmt.Sprintf("browser %s flow not found", mode)}, nil
	}
	resp, err := flow.complete(ctx, otp)
	if err == nil {
		s.browserAuthFlows.remove(flowID)
	}
	return resp, err
}

func (s *Server) browserAuthResendOTP(ctx context.Context, mode, flowID string) (*pb.BrowserAuthResendOTPOutput, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.BrowserAuthResendOTPOutput{
			FlowId:       flowID,
			Success:      false,
			ErrorMessage: fmt.Sprintf("browser %s flow not found", mode),
		}, nil
	}
	return flow.resendEmailOTP(s.browserAutomationClient, s.browserAuthConfig)
}

func (s *Server) browserAuthStatus(ctx context.Context, flowID string) (*pb.BrowserFlowStatusResponse, error) {
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.BrowserFlowStatusResponse{Found: false, FlowId: flowID, ErrorMessage: "browser flow not found"}, nil
	}
	return flow.statusResponse(), nil
}

func (s *Server) browserAuthCancel(ctx context.Context, mode, flowID string) (*pb.CancelRegisterResponse, error) {
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.CancelRegisterResponse{Success: true}, nil
	}
	flow.cancelFlow(fmt.Sprintf("browser %s cancelled", mode))
	s.browserAuthFlows.remove(flowID)
	return &pb.CancelRegisterResponse{Success: true}, nil
}

func (f *browserAuthFlow) run(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) {
	defer f.finish()
	f.setStatus(browserAuthStageStarting, "starting browser session")
	if err := f.startSession(client, cfg); err != nil {
		f.fail(err)
		return
	}
	defer f.stopSession(client)
	if f.mode == browserAuthModeRegister {
		f.runRegister(client, cfg)
		return
	}
	f.runLogin(client, cfg)
}

func (f *browserAuthFlow) runRegister(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) {
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}
	if err := f.openRegisterEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}

	f.setStatus(browserAuthStageEmailEntry, "submitting register email")
	if _, err := f.submitRegisterEmail(client, cfg); err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting register password")
	passwordEntry, err := f.openRegisterPasswordEntry(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	if passwordEntry == "existing_login_password" {
		if err := f.loginExistingAccountFromRegister(client, cfg); err != nil {
			f.fail(err)
		}
		return
	}
	otpIssuedAfter, err := f.submitRegisterPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	f.markOTPRequestClickedAt(unixSecondsFromMillis(otpIssuedAfter))
	otp, err := f.waitForOTP()
	if err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageOTPSubmit, "submitting register OTP")
	if _, err := f.submitRegisterOTP(client, cfg, otp); err != nil {
		f.fail(err)
		return
	}
	if _, err := f.completeRegisterProfile(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if err := f.captureResult(client, cfg, true); err != nil {
		f.fail(err)
		return
	}
}

func (f *browserAuthFlow) runLogin(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) {
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}
	if err := f.openLoginEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		if err := f.captureResult(client, cfg, true); err != nil {
			f.fail(err)
		}
		return
	}
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}

	f.setStatus(browserAuthStageEmailEntry, "submitting login email")
	if _, err := f.submitLoginEmail(client, cfg); err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting login password")
	if err := f.openLoginPasswordEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	state, otpIssuedAfter, err := f.submitLoginPassword(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	if state == "otp_required" {
		f.markOTPRequestClickedAt(unixSecondsFromMillis(otpIssuedAfter))
		otp, err := f.waitForOTP()
		if err != nil {
			f.fail(err)
			return
		}
		f.setStatus(browserAuthStageOTPSubmit, "submitting login OTP")
		if _, err := f.submitLoginOTP(client, cfg, otp); err != nil {
			f.fail(err)
			return
		}
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if err := f.captureResult(client, cfg, true); err != nil {
		f.fail(err)
		return
	}
}

func (f *browserAuthFlow) startSession(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	ctx, cancel := context.WithTimeout(f.ctx, cfg.CommandTimeout)
	defer cancel()
	resp, err := client.StartBrowserSession(ctx, &browserautomationv1.StartBrowserSessionRequest{
		RequestId: "gpt-browser-auth-" + f.flowID,
		Profile: &browserautomationv1.BrowserProfile{
			BrowserKind: browserautomationv1.BrowserKind_BROWSER_KIND_FIREFOX,
			Locale:      cfg.Locale,
			Timezone:    cfg.Timezone,
			UserAgent:   cfg.UserAgent,
			Viewport: &browserautomationv1.BrowserViewport{
				Width:  int32(cfg.WindowWidth),
				Height: int32(cfg.WindowHeight),
			},
			ProxyRef: cfg.ProxyRef,
			ExtraHttpHeaders: map[string]string{
				"Accept-Language": cfg.AcceptLanguage,
			},
			InitScripts: []string{browserAuthLanguageOverrideScript(cfg.Locale)},
			Labels: map[string]string{
				"domain":                   "gpt",
				"workflow":                 "browser_auth",
				"mode":                     f.mode,
				"job_id":                   f.jobID,
				"camoufox.geoip":           boolLabel(cfg.GeoIP),
				"camoufox.block_images":    boolLabel(cfg.BlockImages),
				"camoufox.humanize":        cfg.Humanize,
				"camoufox.enable_cache":    "false",
				"camoufox.disable_coop":    "true",
				"camoufox.main_world_eval": "false",
			},
		},
		Ttl: durationpb.New(cfg.SessionTTL),
	})
	if err != nil {
		return err
	}
	if resp.GetError() != nil {
		return errors.New(resp.GetError().GetMessage())
	}
	sessionID := resp.GetSession().GetSessionId()
	if sessionID == "" {
		return fmt.Errorf("browser automation returned empty session_id")
	}
	f.mu.Lock()
	f.sessionID = sessionID
	f.mu.Unlock()
	return nil
}

func (f *browserAuthFlow) stopSession(client browserautomationv1.BrowserAutomationServiceClient) {
	sessionID := f.getSessionID()
	if sessionID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = client.StopBrowserSession(ctx, &browserautomationv1.StopBrowserSessionRequest{
		SessionId: sessionID,
		Reason:    "gpt browser auth finished",
	})
}

func (f *browserAuthFlow) openRegisterEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	return f.openAuthLoginEntry(client, cfg)
}

func (f *browserAuthFlow) openAuthLoginEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	results, err := f.execute(client, cfg, f.mode+"-open-auth-login", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-auth-login", "https://chatgpt.com/auth/login", cfg.CommandTimeout),
		clickCommand("reject-cookies", browserAuthRejectCookiesSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-cookies-dismissed", 500*time.Millisecond),
		getCookiesCommand("auth-entry-cookies", []string{"https://chatgpt.com/"}, 5*time.Second),
		countElementsCommand("count-email-input", browserAuthRegisterEmailSelector(), 2*time.Second, true),
		getPageStateCommand("auth-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if f.mode == browserAuthModeLogin && extractBrowserSessionToken(browserCookieMaps(commandResultMap(results, "auth-entry-cookies"))) != "" {
		return nil
	}
	if browserAuthMatchedCount(results, "count-email-input") > 0 {
		return nil
	}
	results, err = f.execute(client, cfg, f.mode+"-wait-auth-login-email", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-input", browserAuthRegisterEmailSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, true),
		getCookiesCommand("auth-wait-cookies", []string{"https://chatgpt.com/"}, 5*time.Second),
		getPageStateCommand("auth-wait-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if f.mode == browserAuthModeLogin && extractBrowserSessionToken(browserCookieMaps(commandResultMap(results, "auth-wait-cookies"))) != "" {
		return nil
	}
	if browserAuthCommandSucceeded(results, "wait-email-input") {
		return nil
	}
	state := browserAuthPageStateData(results, "auth-wait-state")
	return browserAuthStepError(f.mode, "entry", "email_input_missing", state)
}

func (f *browserAuthFlow) submitRegisterEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-submit-email", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-email", browserAuthRegisterEmailSelector(), f.email, 10*time.Second, false),
		clickCommand("click-email-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-email-submit-request", "https://chatgpt.com/api/auth/signin/openai", "POST", 200, 299, startedAfter, 30*time.Second, true),
		waitForSelectorCommand("wait-email-verification-code", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("email-verification-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "email-verification-state")
	if !browserAuthCommandSucceeded(results, "wait-email-submit-request") {
		return 0, browserAuthStepError(f.mode, "email", "email_submit_request_missing", state)
	}
	if !browserAuthCommandSucceeded(results, "wait-email-verification-code") {
		return 0, browserAuthStepError(f.mode, "email", "email_verification_input_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-email-submit-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) openRegisterPasswordEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	results, err := f.execute(client, cfg, "register-password-entry", []*browserautomationv1.BrowserCommand{
		clickCommand("click-continue-with-password", selectorGroup(5*time.Second, roleSelector("link", "Continue with password", true)), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-password-input", selectorGroup(20*time.Second, browserAuthRegisterPasswordSelector(), browserAuthLoginPasswordSelector()), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, true),
		countElementsCommand("count-new-password-input", browserAuthRegisterPasswordSelector(), 2*time.Second, true),
		countElementsCommand("count-current-password-input", browserAuthLoginPasswordSelector(), 2*time.Second, true),
		getPageStateCommand("password-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return "", err
	}
	state := browserAuthPageStateData(results, "password-entry-state")
	if browserAuthMatchedCount(results, "count-new-password-input") > 0 {
		return "new_password", nil
	}
	if browserAuthMatchedCount(results, "count-current-password-input") > 0 ||
		browserAuthPageHasAny(state, "log-in/password", "Enter your password", "Forgot password") {
		return "existing_login_password", nil
	}
	if !browserAuthCommandSucceeded(results, "wait-password-input") {
		return "", browserAuthStepError(f.mode, "password_entry", "password_input_missing", state)
	}
	return "", browserAuthStepError(f.mode, "password_entry", "password_input_unknown", state)
}

func (f *browserAuthFlow) loginExistingAccountFromRegister(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	f.setStatus(browserAuthStageCredentialEntry, "logging in existing account")
	state, otpIssuedAfter, err := f.submitLoginPassword(client, cfg)
	if err != nil {
		return err
	}
	if state == "otp_required" {
		f.markOTPRequestClickedAt(unixSecondsFromMillis(otpIssuedAfter))
		otp, err := f.waitForOTP()
		if err != nil {
			return err
		}
		f.setStatus(browserAuthStageOTPSubmit, "submitting existing account login OTP")
		if _, err := f.submitLoginOTP(client, cfg, otp); err != nil {
			return err
		}
	}
	if err := f.waitForBrowserAuthSessionCookie(client, cfg); err != nil {
		return err
	}
	return f.captureResult(client, cfg, true)
}

func (f *browserAuthFlow) submitRegisterPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-submit-password", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-password", browserAuthRegisterPasswordSelector(), f.password, 10*time.Second, false),
		clickCommand("click-password-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-password-register-request", "https://auth.openai.com/api/accounts/user/register", "POST", 200, 299, startedAfter, 45*time.Second, true),
		waitForSelectorCommand("wait-password-email-verification-code", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("password-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "password-submit-state")
	if !browserAuthCommandSucceeded(results, "wait-password-register-request") {
		return 0, browserAuthStepError(f.mode, "password", "password_register_request_missing", state)
	}
	if !browserAuthCommandSucceeded(results, "wait-password-email-verification-code") {
		return 0, browserAuthStepError(f.mode, "password", "email_verification_input_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-password-register-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) resendRegisterEmailOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-resend-email-otp", []*browserautomationv1.BrowserCommand{
		clickCommand("click-resend-email", selectorGroup(5*time.Second, roleSelector("button", "Resend email", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-resend-email-request", "https://auth.openai.com/api/accounts/email-otp/resend", "POST", 200, 299, startedAfter, 45*time.Second, true),
		waitForSelectorCommand("wait-resend-email-code", browserAuthRegisterOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 10*time.Second, true),
		getPageStateCommand("resend-email-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "resend-email-state")
	if !browserAuthCommandSucceeded(results, "wait-resend-email-request") {
		return 0, browserAuthStepError(f.mode, "resend", "resend_email_request_missing", state)
	}
	if !browserAuthCommandSucceeded(results, "wait-resend-email-code") {
		return 0, browserAuthStepError(f.mode, "resend", "email_verification_input_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-resend-email-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) resendEmailOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (*pb.BrowserAuthResendOTPOutput, error) {
	if f.cancelled() {
		return nil, fmt.Errorf("browser auth cancelled")
	}
	if f.mode != browserAuthModeRegister {
		return &pb.BrowserAuthResendOTPOutput{
			FlowId:       f.flowID,
			Email:        f.email,
			Success:      false,
			ErrorMessage: fmt.Sprintf("browser %s email OTP resend is not supported", f.mode),
		}, nil
	}
	f.setStatus(browserAuthStageOTPRequestClick, "resending email OTP")
	startedAt, err := f.resendRegisterEmailOTP(client, cfg)
	if err != nil {
		return nil, err
	}
	issuedAfter := unixSecondsFromMillis(startedAt)
	f.markOTPRequestClickedAt(issuedAfter)
	data := map[string]any{
		"flow_id":                        f.flowID,
		"mode":                           f.mode,
		"email":                          f.email,
		"otp_issued_after_unix":          issuedAfter,
		"otp_request_started_at_unix_ms": startedAt,
	}
	return &pb.BrowserAuthResendOTPOutput{
		FlowId:                    f.flowID,
		Email:                     f.email,
		Success:                   true,
		OtpIssuedAfterUnix:        issuedAfter,
		OtpRequestStartedAtUnixMs: startedAt,
		OtpTimeoutSeconds:         0,
		Data:                      protoData(data),
	}, nil
}

func (f *browserAuthFlow) submitRegisterOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-submit-otp", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-email-code", browserAuthRegisterOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-code-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-email-code-validate", "https://auth.openai.com/api/accounts/email-otp/validate", "POST", 200, 299, startedAfter, 45*time.Second, true),
		waitForSelectorCommand("wait-about-you-name", browserAuthRegisterProfileNameSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("otp-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	state := browserAuthPageStateData(results, "otp-submit-state")
	if !browserAuthCommandSucceeded(results, "wait-email-code-validate") {
		return 0, browserAuthStepError(f.mode, "otp", "email_code_validate_request_missing", state)
	}
	if !browserAuthCommandSucceeded(results, "wait-about-you-name") {
		return 0, browserAuthStepError(f.mode, "otp", "profile_name_input_missing", state)
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-email-code-validate")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) completeRegisterProfile(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "register-complete-profile", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-full-name", browserAuthRegisterProfileNameSelector(), f.fullName, 10*time.Second, false),
		fillCommand("fill-age", browserAuthRegisterAgeSelector(), browserAuthAgeFromBirthday(f.birthday), 10*time.Second, false),
		clickCommand("click-finish-creating-account", selectorGroup(5*time.Second, roleSelector("button", "Finish creating account", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-create-account-request", "https://auth.openai.com/api/accounts/create_account", "POST", 200, 299, startedAfter, 90*time.Second, true),
		waitTimeoutCommand("wait-after-create-account", 3*time.Second),
		getPageStateCommand("profile-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	if !browserAuthCommandSucceeded(results, "wait-create-account-request") {
		return 0, browserAuthStepError(f.mode, "profile", "create_account_request_missing", browserAuthPageStateData(results, "profile-submit-state"))
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-create-account-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) openLoginEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	return f.openAuthLoginEntry(client, cfg)
}

func (f *browserAuthFlow) submitLoginEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "login-submit-email", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-email", browserAuthRegisterEmailSelector(), f.email, 10*time.Second, false),
		clickCommand("click-email-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-email-submit-request", "https://chatgpt.com/api/auth/signin/openai", "POST", 200, 299, startedAfter, 30*time.Second, true),
		waitForSelectorGroupCommand("wait-email-verification-or-password", browserAuthLoginEmailAdvancedSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 35*time.Second, true),
		getPageStateCommand("email-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	if !browserAuthCommandSucceeded(results, "wait-email-verification-or-password") {
		return 0, browserAuthStepError(f.mode, "email", "email_advance_missing", browserAuthPageStateData(results, "email-submit-state"))
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-email-submit-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) openLoginPasswordEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	results, err := f.execute(client, cfg, "login-password-entry", []*browserautomationv1.BrowserCommand{
		clickCommand("click-continue-with-password", selectorGroup(5*time.Second, roleSelector("link", "Continue with password", true)), 10*time.Second, false),
		waitForSelectorCommand("wait-current-password-input", browserAuthLoginPasswordSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 30*time.Second, true),
		getPageStateCommand("password-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if !browserAuthCommandSucceeded(results, "wait-current-password-input") {
		return browserAuthStepError(f.mode, "password_entry", "password_input_missing", browserAuthPageStateData(results, "password-entry-state"))
	}
	return nil
}

func (f *browserAuthFlow) submitLoginPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "login-submit-password", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-password", browserAuthLoginPasswordSelector(), f.password, 10*time.Second, false),
		clickCommand("click-password-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-login-auth-request", "https://auth.openai.com", "POST", 200, 399, startedAfter, 45*time.Second, true),
		waitTimeoutCommand("wait-after-password", 5*time.Second),
		waitForSelectorCommand("wait-login-otp", browserAuthLoginOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 2*time.Second, true),
		getPageStateCommand("password-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return "", 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-login-auth-request")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	if browserAuthCommandSucceeded(results, "wait-login-otp") {
		return "otp_required", startedAt, nil
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", startedAt, nil
	}
	state := browserAuthPageStateData(results, "password-submit-state")
	if browserAuthPageHasAny(state, "Open profile menu", "Claim offer", "New chat", "auth/callback/openai") {
		return "password_submitted", startedAt, nil
	}
	return "", startedAt, browserAuthStepError(f.mode, "password", "session_or_otp_missing", state)
}

func (f *browserAuthFlow) submitLoginOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) (int64, error) {
	startedAfter := time.Now().Add(-time.Second).UnixMilli()
	results, err := f.execute(client, cfg, "login-submit-otp", []*browserautomationv1.BrowserCommand{
		fillCommand("fill-login-code", browserAuthLoginOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-code-continue", selectorGroup(5*time.Second, roleSelector("button", "Continue", true)), 10*time.Second, false),
		waitForNetworkCommandWithContinue("wait-login-code-validate", "https://auth.openai.com", "POST", 200, 399, startedAfter, 45*time.Second, true),
		waitTimeoutCommand("wait-after-login-code", 5*time.Second),
		getPageStateCommand("otp-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return 0, err
	}
	startedAt := browserAuthNetworkRequestStartedAtUnixMs(results, "wait-login-code-validate")
	if startedAt <= 0 {
		startedAt = startedAfter
	}
	return startedAt, nil
}

func (f *browserAuthFlow) openEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	if _, err := f.execute(client, cfg, "open-entry", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-chatgpt", "https://chatgpt.com/", cfg.CommandTimeout),
	}); err != nil {
		return "", err
	}
	if f.hasBrowserAuthEmailInput(client, cfg) {
		return "email_ready", nil
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", nil
	}
	if f.tryBrowserAuthEntryClick(client, cfg, "direct-entry") {
		return "clicked", nil
	}
	if f.tryBrowserAuthProfileMenuClick(client, cfg) {
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return "session_ready", nil
		}
		if f.tryBrowserAuthEntryClick(client, cfg, "profile-entry") {
			return "clicked", nil
		}
	}
	if f.hasBrowserAuthEmailInput(client, cfg) {
		return "email_ready", nil
	}
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", nil
	}
	data := f.browserAuthPageState(client, cfg, "entry-state")
	return "entry_missing", browserAuthStepError(f.mode, "entry", "entry_missing", data)
}

func (f *browserAuthFlow) submitEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	if f.hasBrowserAuthSessionCookie(client, cfg) {
		return "session_ready", nil
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) {
		_ = f.tryBrowserAuthEntryClick(client, cfg, "email-entry")
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) {
		_ = f.tryBrowserAuthEmailProviderClick(client, cfg)
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) && f.tryBrowserAuthProfileMenuClick(client, cfg) {
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return "session_ready", nil
		}
		_ = f.tryBrowserAuthEntryClick(client, cfg, "email-profile-entry")
	}
	if !f.hasBrowserAuthEmailInput(client, cfg) {
		data := f.browserAuthPageState(client, cfg, "email-state")
		return "email_input_missing", browserAuthStepError(f.mode, "email", "email_input_missing", data)
	}
	results, err := f.execute(client, cfg, "submit-email", []*browserautomationv1.BrowserCommand{
		typeTextCommand("type-email", browserAuthEmailSelector(), f.email, 15*time.Millisecond, 5*time.Second, true, false),
	})
	if err != nil {
		return "", err
	}
	if !browserAuthCommandSucceeded(results, "type-email") {
		data := f.browserAuthPageState(client, cfg, "email-state")
		return "email_input_missing", browserAuthStepError(f.mode, "email", "email_input_missing", data)
	}
	if f.tryBrowserAuthEmailSubmitForm(client, cfg) && f.browserAuthEmailSubmitted(client, cfg) {
		return "submitted", nil
	}
	if f.tryBrowserAuthEmailSubmitPress(client, cfg) && f.browserAuthEmailSubmitted(client, cfg) {
		return "submitted", nil
	}
	if f.tryBrowserAuthEmailSubmitClick(client, cfg) && f.browserAuthEmailSubmitted(client, cfg) {
		return "submitted", nil
	}
	data := f.browserAuthPageState(client, cfg, "email-state")
	return "email_submit_missing", browserAuthStepError(f.mode, "email", "email_submit_missing", data)
}

func (f *browserAuthFlow) hasBrowserAuthEmailInput(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "check-email-input", []*browserautomationv1.BrowserCommand{
		countElementsCommand("count-email-input", browserAuthEmailSelector(), 2*time.Second, true),
	})
	return err == nil && browserAuthMatchedCount(results, "count-email-input") > 0
}

func (f *browserAuthFlow) hasBrowserAuthSessionCookie(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "check-session-cookie", []*browserautomationv1.BrowserCommand{
		getCookiesCommand("check-session-cookie", []string{"https://chatgpt.com/"}, 5*time.Second),
	})
	if err != nil {
		return false
	}
	return extractBrowserSessionToken(browserCookieMaps(commandResultMap(results, "check-session-cookie"))) != ""
}

func (f *browserAuthFlow) hasBrowserAuthPasswordInput(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "check-password-input", []*browserautomationv1.BrowserCommand{
		countElementsCommand("count-password-input", browserAuthPasswordSelector(), 2*time.Second, true),
	})
	return err == nil && browserAuthMatchedCount(results, "count-password-input") > 0
}

func (f *browserAuthFlow) browserAuthEmailSubmitted(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	if f.hasBrowserAuthSessionCookie(client, cfg) || f.hasBrowserAuthPasswordInput(client, cfg) {
		return true
	}
	return !f.hasBrowserAuthEmailInput(client, cfg)
}

func (f *browserAuthFlow) tryBrowserAuthEntryClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, key string) bool {
	results, err := f.execute(client, cfg, key, []*browserautomationv1.BrowserCommand{
		clickCommand("click-entry", browserAuthEntrySelector(f.mode), 3*time.Second, true),
		waitTimeoutCommand("wait-entry", 1500*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "click-entry")
}

func (f *browserAuthFlow) tryBrowserAuthProfileMenuClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "open-profile-menu", []*browserautomationv1.BrowserCommand{
		clickCommand("open-profile-menu", browserAuthProfileMenuSelector(), 2*time.Second, true),
		waitTimeoutCommand("wait-profile-menu", 700*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "open-profile-menu")
}

func (f *browserAuthFlow) tryBrowserAuthEmailProviderClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "choose-email-provider", []*browserautomationv1.BrowserCommand{
		clickCommand("click-email-provider", browserAuthEmailProviderSelector(), 2*time.Second, true),
		waitTimeoutCommand("wait-email-provider", 700*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "click-email-provider")
}

func (f *browserAuthFlow) tryBrowserAuthEmailSubmitClick(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "click-email-submit", []*browserautomationv1.BrowserCommand{
		clickCommand("click-email-submit", browserAuthEmailSubmitSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-email-submit", 1200*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "click-email-submit")
}

func (f *browserAuthFlow) tryBrowserAuthEmailSubmitForm(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "submit-email-form", []*browserautomationv1.BrowserCommand{
		submitFormCommand("submit-email-form", browserAuthEmailSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-email-form-submit", 1200*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "submit-email-form")
}

func (f *browserAuthFlow) tryBrowserAuthEmailSubmitPress(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "press-email-submit", []*browserautomationv1.BrowserCommand{
		pressCommand("press-email-enter", browserAuthEmailSelector(), "Enter", 2*time.Second, true),
		waitTimeoutCommand("wait-email-enter", 1200*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "press-email-enter")
}

func (f *browserAuthFlow) browserAuthPageState(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, commandID string) map[string]any {
	results, err := f.execute(client, cfg, commandID, []*browserautomationv1.BrowserCommand{
		getPageStateCommand(commandID, true, true, false, 5*time.Second),
	})
	if err != nil {
		return map[string]any{"state": "page_state_failed", "title": err.Error()}
	}
	return browserAuthPageStateData(results, commandID)
}

func (f *browserAuthFlow) completePostOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	waitLimit := 2 * cfg.CommandTimeout
	if waitLimit < 90*time.Second {
		waitLimit = 90 * time.Second
	}
	deadline := time.Now().Add(waitLimit)
	var last map[string]any
	for time.Now().Before(deadline) {
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return nil
		}
		last = f.browserAuthPageState(client, cfg, "post-otp-state")
		if browserAuthPageHasAny(last, "age") {
			_ = f.tryBrowserAuthAgeProfile(client, cfg)
		} else {
			_ = f.tryBrowserAuthBirthdayProfile(client, cfg)
		}
		_ = f.tryBrowserAuthPostOTPContinue(client, cfg)
		if f.hasBrowserAuthSessionCookie(client, cfg) {
			return nil
		}
		_, _ = f.execute(client, cfg, "wait-post-otp", []*browserautomationv1.BrowserCommand{
			waitTimeoutCommand("wait-post-otp", 1500*time.Millisecond),
		})
	}
	if last == nil {
		last = f.browserAuthPageState(client, cfg, "post-otp-state")
	}
	return browserAuthStepError(f.mode, "post_otp", "session_cookie_missing", last)
}

func (f *browserAuthFlow) waitForBrowserAuthSessionCookie(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	f.setStatus(browserAuthStageSessionCapture, "waiting for browser session cookie")
	waitLimit := cfg.CommandTimeout
	if waitLimit < 60*time.Second {
		waitLimit = 60 * time.Second
	}
	deadline := time.Now().Add(waitLimit)
	var last map[string]any
	for time.Now().Before(deadline) {
		results, err := f.execute(client, cfg, "wait-session-cookie", []*browserautomationv1.BrowserCommand{
			getCookiesCommand("wait-session-cookies", []string{"https://chatgpt.com/"}, 5*time.Second),
			getPageStateCommand("wait-session-state", true, true, false, 5*time.Second),
		})
		if err == nil {
			state := browserAuthPageStateData(results, "wait-session-state")
			if state != nil {
				last = state
			}
			cookies := browserCookieMaps(commandResultMap(results, "wait-session-cookies"))
			if extractBrowserSessionToken(cookies) != "" {
				return nil
			}
		}
		_, _ = f.execute(client, cfg, "wait-session-cookie-delay", []*browserautomationv1.BrowserCommand{
			waitTimeoutCommand("wait-session-cookie-delay", 1500*time.Millisecond),
		})
	}
	if last == nil {
		last = f.browserAuthPageState(client, cfg, "wait-session-state")
	}
	return browserAuthStepError(f.mode, "session", "session_cookie_missing", last)
}

func (f *browserAuthFlow) tryBrowserAuthAgeProfile(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "complete-registration-profile", []*browserautomationv1.BrowserCommand{
		fillGroupCommand("fill-profile-name", browserAuthProfileNameSelector(), f.fullName, 2*time.Second, true),
		fillGroupCommand("fill-profile-age", browserAuthAgeSelector(), browserAuthAgeFromBirthday(f.birthday), 2*time.Second, true),
		clickCommand("submit-profile", browserAuthPostOTPContinueSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-profile-submit", 1500*time.Millisecond),
	})
	return err == nil && browserAuthAnyCommandSucceeded(results,
		"fill-profile-name",
		"fill-profile-age",
		"submit-profile",
	)
}

func (f *browserAuthFlow) tryBrowserAuthBirthdayProfile(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	birthday := browserAuthBirthdayPartsFrom(f.birthday)
	results, err := f.execute(client, cfg, "complete-registration-profile", []*browserautomationv1.BrowserCommand{
		fillGroupCommand("fill-profile-name", browserAuthProfileNameSelector(), f.fullName, 2*time.Second, true),
		fillGroupCommand("fill-profile-birthday", browserAuthBirthdaySelector(), birthday.US, 2*time.Second, true),
		fillGroupCommand("fill-profile-month", browserAuthMonthInputSelector(), birthday.Month, time.Second, true),
		fillGroupCommand("fill-profile-day", browserAuthDayInputSelector(), birthday.Day, time.Second, true),
		fillGroupCommand("fill-profile-year", browserAuthYearInputSelector(), birthday.Year, time.Second, true),
		selectOptionGroupCommand("select-profile-month", browserAuthMonthSelectSelector(), []string{birthday.Month, birthday.MonthPadded}, []string{birthday.MonthName, birthday.MonthShort}, nil, time.Second, true),
		selectOptionGroupCommand("select-profile-day", browserAuthDaySelectSelector(), []string{birthday.Day, birthday.DayPadded}, nil, nil, time.Second, true),
		selectOptionGroupCommand("select-profile-year", browserAuthYearSelectSelector(), []string{birthday.Year}, nil, nil, time.Second, true),
		clickCommand("submit-profile", browserAuthPostOTPContinueSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-profile-submit", 1500*time.Millisecond),
	})
	return err == nil && browserAuthAnyCommandSucceeded(results,
		"fill-profile-name",
		"fill-profile-birthday",
		"fill-profile-month",
		"fill-profile-day",
		"fill-profile-year",
		"select-profile-month",
		"select-profile-day",
		"select-profile-year",
		"submit-profile",
	)
}

func (f *browserAuthFlow) tryBrowserAuthPostOTPContinue(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) bool {
	results, err := f.execute(client, cfg, "post-otp-continue", []*browserautomationv1.BrowserCommand{
		clickCommand("post-otp-continue", browserAuthPostOTPContinueSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-post-otp-continue", 1500*time.Millisecond),
	})
	return err == nil && browserAuthCommandSucceeded(results, "post-otp-continue")
}

func (f *browserAuthFlow) captureResult(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, requireCredentials bool) error {
	f.setStatus(browserAuthStageSessionCapture, "capturing browser session")
	results, err := f.execute(client, cfg, "capture-session", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-session-endpoint", "https://chatgpt.com/api/auth/session", 30*time.Second),
		extractTextCommand("extract-session-body", cssSelector("body"), 10*time.Second, false),
		getCookiesCommand("capture-cookies", []string{"https://chatgpt.com/"}, 10*time.Second),
	})
	if err != nil {
		return err
	}
	result := browserAuthRegisterResponse(results)
	if requireCredentials && result.GetSessionToken() == "" {
		return fmt.Errorf("missing session token after browser %s", f.mode)
	}
	if requireCredentials && result.GetAccessToken() == "" {
		return fmt.Errorf("missing access token after browser %s", f.mode)
	}
	f.mu.Lock()
	f.result = result
	f.success = true
	f.errMessage = ""
	f.done = true
	f.stage = browserAuthStageSucceeded
	f.message = fmt.Sprintf("browser %s completed", f.mode)
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
	return nil
}

func (f *browserAuthFlow) handleTerminalState(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, state string) bool {
	switch state {
	case "session_ready":
		if err := f.captureResult(client, cfg, true); err != nil {
			f.fail(err)
		}
		return true
	case "user_already_exists":
		f.fail(fmt.Errorf("account already exists"))
		return true
	default:
		return false
	}
}

func (f *browserAuthFlow) execute(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, taskKey string, commands []*browserautomationv1.BrowserCommand) ([]*browserautomationv1.BrowserCommandResult, error) {
	sessionID := f.getSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("browser session is not ready")
	}
	ctx, cancel := context.WithTimeout(f.ctx, taskTimeout(commands, cfg.CommandTimeout))
	defer cancel()
	resp, err := client.ExecuteBrowserCommands(ctx, &browserautomationv1.ExecuteBrowserCommandsRequest{
		RequestId: f.nextTaskRequestID(taskKey),
		Input: &browserautomationv1.BrowserTaskInput{
			SessionId:   sessionID,
			TaskKey:     "gpt.browser_auth." + taskKey,
			ScenarioKey: "gpt.browser_auth." + f.mode,
			Timeout:     durationpb.New(taskTimeout(commands, cfg.CommandTimeout)),
			Commands:    commands,
			Labels: map[string]string{
				"domain":   "gpt",
				"workflow": "browser_auth",
				"mode":     f.mode,
				"job_id":   f.jobID,
				"flow_id":  f.flowID,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != nil {
		return resp.GetResults(), errors.New(resp.GetError().GetMessage())
	}
	continueOnError := map[string]bool{}
	for _, command := range commands {
		if command.GetContinueOnError() {
			continueOnError[command.GetCommandId()] = true
		}
	}
	for _, result := range resp.GetResults() {
		if result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_FAILED ||
			result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_TIMEOUT {
			if continueOnError[result.GetCommandId()] {
				continue
			}
			if result.GetError() != nil {
				return resp.GetResults(), errors.New(result.GetError().GetMessage())
			}
			return resp.GetResults(), fmt.Errorf("browser command %s failed", result.GetCommandKey())
		}
	}
	return resp.GetResults(), nil
}

func (f *browserAuthFlow) nextTaskRequestID(taskKey string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.taskSeq++
	return fmt.Sprintf("gpt-browser-auth-%s-%04d-%s", f.flowID, f.taskSeq, taskKey)
}

func (f *browserAuthFlow) complete(ctx context.Context, otp string) (*pb.RegisterResponse, error) {
	code := normalizeOTP(otp)
	if code == "" {
		return &pb.RegisterResponse{Success: false, ErrorMessage: "otp is required"}, nil
	}
	select {
	case f.otpCh <- code:
	case <-f.doneCh:
		return f.registerResponse(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-f.doneCh:
		return f.registerResponse(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *browserAuthFlow) waitForOTP() (string, error) {
	now := time.Now().Unix()
	f.mu.Lock()
	f.otpNeed = true
	if f.otpAction <= 0 {
		f.otpAction = now
	}
	f.otpWait = now
	f.stage = browserAuthStageWaitingForOTP
	f.message = "waiting for orchestrator-supplied OTP"
	f.updatedAt = now
	f.mu.Unlock()

	select {
	case otp := <-f.otpCh:
		if otp == "" {
			return "", fmt.Errorf("OTP is empty")
		}
		return otp, nil
	case <-f.ctx.Done():
		return "", f.ctx.Err()
	}
}

func (f *browserAuthFlow) markOTPRequestClicked() {
	f.markOTPRequestClickedAt(time.Now().Unix())
}

func (f *browserAuthFlow) markOTPRequestClickedAt(issuedAfterUnix int64) {
	now := time.Now().Unix()
	if issuedAfterUnix <= 0 {
		issuedAfterUnix = now
	}
	f.mu.Lock()
	f.otpAction = issuedAfterUnix
	f.stage = browserAuthStageOTPRequestClicked
	f.message = "OTP request action clicked"
	f.updatedAt = now
	f.mu.Unlock()
}

func (f *browserAuthFlow) setStatus(stage, message string) {
	now := time.Now().Unix()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stage = stage
	f.message = message
	f.updatedAt = now
}

func (f *browserAuthFlow) fail(err error) {
	message := "browser auth failed"
	if err != nil {
		message = err.Error()
	}
	if errors.Is(err, context.Canceled) {
		f.cancelFlow("browser auth cancelled")
		return
	}
	f.mu.Lock()
	f.success = false
	f.errMessage = message
	f.result = &pb.RegisterResponse{Success: false, ErrorMessage: message}
	f.done = true
	f.stage = browserAuthStageFailed
	f.message = message
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
}

func (f *browserAuthFlow) cancelFlow(message string) {
	f.cancel()
	f.mu.Lock()
	f.success = false
	f.errMessage = message
	f.result = &pb.RegisterResponse{Success: false, ErrorMessage: message}
	f.done = true
	f.stage = browserAuthStageCancelled
	f.message = message
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
	f.finish()
}

func (f *browserAuthFlow) finish() {
	f.doneOnce.Do(func() {
		close(f.doneCh)
	})
}

func (f *browserAuthFlow) cancelled() bool {
	select {
	case <-f.ctx.Done():
		return true
	default:
		return false
	}
}

func (f *browserAuthFlow) getSessionID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessionID
}

func (f *browserAuthFlow) startResponse() *pb.StartRegisterResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &pb.StartRegisterResponse{
		Success:                       true,
		FlowId:                        f.flowID,
		OtpRequired:                   f.otpNeed,
		OtpIssuedAfterUnix:            f.otpAction,
		Stage:                         f.stage,
		StatusMessage:                 f.message,
		OtpWaitStartedAtUnix:          f.otpWait,
		OtpRequestActionStartedAtUnix: f.otpAction,
		Result:                        cloneRegisterResult(f.result),
	}
}

func (f *browserAuthFlow) statusResponse() *pb.BrowserFlowStatusResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := &pb.BrowserFlowStatusResponse{
		Found:                         true,
		FlowId:                        f.flowID,
		Mode:                          f.mode,
		Stage:                         f.stage,
		StatusMessage:                 f.message,
		OtpRequired:                   f.otpNeed,
		Done:                          f.done,
		Success:                       f.success,
		ErrorMessage:                  f.errMessage,
		StartedAtUnix:                 f.startedAt,
		UpdatedAtUnix:                 f.updatedAt,
		OtpIssuedAfterUnix:            f.otpAction,
		OtpWaitStartedAtUnix:          f.otpWait,
		OtpRequestActionStartedAtUnix: f.otpAction,
		Result:                        cloneRegisterResult(f.result),
	}
	return resp
}

func (f *browserAuthFlow) registerResponse() *pb.RegisterResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.result != nil {
		return cloneRegisterResult(f.result)
	}
	if f.done && f.errMessage != "" {
		return &pb.RegisterResponse{Success: false, ErrorMessage: f.errMessage}
	}
	return &pb.RegisterResponse{Success: false, ErrorMessage: "browser flow did not complete"}
}

func cloneRegisterResult(in *pb.RegisterResponse) *pb.RegisterResponse {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*pb.RegisterResponse)
}

func navigateCommand(commandID, url string, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_Navigate{
			Navigate: &browserautomationv1.NavigateCommand{
				Url:       url,
				WaitUntil: browserautomationv1.BrowserNavigationWaitUntil_BROWSER_NAVIGATION_WAIT_UNTIL_DOM_CONTENT_LOADED,
				Timeout:   durationpb.New(timeout),
			},
		},
	}
}

func clickCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Click{
			Click: &browserautomationv1.ClickCommand{
				SelectorGroup: selectorGroup,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func fillCommand(commandID string, selector *browserautomationv1.BrowserSelector, value string, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Fill{
			Fill: &browserautomationv1.FillCommand{
				Selector: selector,
				Value:    value,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func fillGroupCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, value string, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Fill{
			Fill: &browserautomationv1.FillCommand{
				SelectorGroup: selectorGroup,
				Value:         value,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func pressCommand(commandID string, selector *browserautomationv1.BrowserSelector, key string, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Press{
			Press: &browserautomationv1.PressCommand{
				Selector: selector,
				Key:      key,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func waitForSelectorCommand(commandID string, selector *browserautomationv1.BrowserSelector, state browserautomationv1.BrowserSelectorState, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForSelector{
			WaitForSelector: &browserautomationv1.WaitForSelectorCommand{
				Selector: selector,
				State:    state,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func waitForSelectorGroupCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, state browserautomationv1.BrowserSelectorState, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForSelector{
			WaitForSelector: &browserautomationv1.WaitForSelectorCommand{
				SelectorGroup: selectorGroup,
				State:         state,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func typeTextCommand(commandID string, selector *browserautomationv1.BrowserSelector, text string, delay, timeout time.Duration, clearBefore, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_TypeText{
			TypeText: &browserautomationv1.TypeTextCommand{
				Selector:    selector,
				Text:        text,
				Delay:       durationpb.New(delay),
				Timeout:     durationpb.New(timeout),
				ClearBefore: clearBefore,
			},
		},
	}
}

func selectOptionGroupCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, values, labels []string, indexes []int32, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_SelectOption{
			SelectOption: &browserautomationv1.SelectOptionCommand{
				SelectorGroup: selectorGroup,
				Values:        compactStringValues(values),
				Labels:        compactStringValues(labels),
				Indexes:       indexes,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func waitForNetworkCommand(commandID string, urlSubstring string, method string, statusMin int32, statusMax int32, startedAfterUnixMs int64, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return waitForNetworkCommandWithContinue(commandID, urlSubstring, method, statusMin, statusMax, startedAfterUnixMs, timeout, false)
}

func waitForNetworkCommandWithContinue(commandID string, urlSubstring string, method string, statusMin int32, statusMax int32, startedAfterUnixMs int64, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForNetworkRequest{
			WaitForNetworkRequest: &browserautomationv1.WaitForNetworkRequestCommand{
				Filter: &browserautomationv1.BrowserNetworkRequestFilter{
					UrlSubstring:       urlSubstring,
					Method:             method,
					StatusCodeMin:      statusMin,
					StatusCodeMax:      statusMax,
					StartedAfterUnixMs: startedAfterUnixMs,
				},
				Timeout:         durationpb.New(timeout),
				RequireResponse: true,
			},
		},
	}
}

func submitFormCommand(commandID string, selector *browserautomationv1.BrowserSelector, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_SubmitForm{
			SubmitForm: &browserautomationv1.SubmitFormCommand{
				Selector: selector,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func countElementsCommand(commandID string, selector *browserautomationv1.BrowserSelector, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_CountElements{
			CountElements: &browserautomationv1.CountElementsCommand{
				Selector: selector,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func getPageStateCommand(commandID string, title, text, html bool, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_GetPageState{
			GetPageState: &browserautomationv1.GetPageStateCommand{
				IncludeTitle: title,
				IncludeText:  text,
				IncludeHtml:  html,
			},
		},
	}
}

func extractTextCommand(commandID string, selector *browserautomationv1.BrowserSelector, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_ExtractText{
			ExtractText: &browserautomationv1.ExtractTextCommand{
				Selector: selector,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func getCookiesCommand(commandID string, urls []string, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_GetCookies{
			GetCookies: &browserautomationv1.GetCookiesCommand{Urls: urls},
		},
	}
}

func waitTimeoutCommand(commandID string, duration time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(duration + time.Second),
		Operation: &browserautomationv1.BrowserCommand_WaitForTimeout{
			WaitForTimeout: &browserautomationv1.WaitForTimeoutCommand{
				Duration: durationpb.New(duration),
			},
		},
	}
}

func taskTimeout(commands []*browserautomationv1.BrowserCommand, fallback time.Duration) time.Duration {
	timeout := fallback
	for _, command := range commands {
		if command.GetTimeout() == nil {
			continue
		}
		if commandTimeout := command.GetTimeout().AsDuration(); commandTimeout > timeout {
			timeout = commandTimeout
		}
	}
	return timeout + 15*time.Second
}

func browserAuthState(results []*browserautomationv1.BrowserCommandResult, commandID string) string {
	state, _ := browserAuthStateData(results, commandID)
	return state
}

func browserAuthStateData(results []*browserautomationv1.BrowserCommandResult, commandID string) (string, map[string]any) {
	data := commandResultMap(results, commandID)
	if data == nil {
		return "", nil
	}
	return strings.TrimSpace(stringMapValue(data, "state")), data
}

func browserAuthNetworkRequestStartedAtUnixMs(results []*browserautomationv1.BrowserCommandResult, commandID string) int64 {
	data := commandResultMap(results, commandID)
	if data == nil {
		return 0
	}
	request, ok := data["request"].(map[string]any)
	if !ok {
		return 0
	}
	return int64MapValue(request, "started_at_unix_ms")
}

func browserAuthStepError(mode, step, state string, data map[string]any) error {
	if state == "" {
		state = "unknown"
	}
	return fmt.Errorf("browser %s %s step failed: %s%s", mode, step, state, browserAuthFailureContext(data))
}

func browserAuthFailureContext(data map[string]any) string {
	if data == nil {
		return ""
	}
	fields := make([]string, 0, 4)
	appendText := func(key, label string, max int) {
		if value := compactBrowserAuthText(stringMapValue(data, key), max); value != "" {
			fields = append(fields, label+"="+value)
		}
	}
	appendList := func(key, label string) {
		if values := browserAuthStringList(data[key], 5, 80); len(values) > 0 {
			fields = append(fields, label+"="+strings.Join(values, " | "))
		}
	}
	appendText("url", "url", 160)
	appendText("title", "title", 120)
	appendList("inputs", "inputs")
	appendList("actions", "actions")
	if len(fields) == 0 {
		return ""
	}
	return " (" + strings.Join(fields, "; ") + ")"
}

func browserAuthStringList(value any, limit, maxLen int) []string {
	out := make([]string, 0, limit)
	add := func(raw any) {
		if len(out) >= limit {
			return
		}
		if text := compactBrowserAuthText(fmt.Sprint(raw), maxLen); text != "" {
			out = append(out, text)
		}
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			add(item)
		}
	case []string:
		for _, item := range typed {
			add(item)
		}
	case string:
		add(typed)
	}
	return out
}

func compactBrowserAuthText(value string, maxLen int) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	if maxLen > 0 && len(value) > maxLen {
		return value[:maxLen] + "..."
	}
	return value
}

func browserAuthRegisterResponse(results []*browserautomationv1.BrowserCommandResult) *pb.RegisterResponse {
	cookiesData := commandResultMap(results, "capture-cookies")
	cookies := browserCookieMaps(cookiesData)
	session := browserAuthSessionFromResults(results)
	if session.sessionToken == "" {
		session.sessionToken = extractBrowserSessionToken(cookies)
	}
	return &pb.RegisterResponse{
		Success:           true,
		SessionToken:      session.sessionToken,
		AccessToken:       session.accessToken,
		DeviceId:          extractCookieValue(cookies, "oai-did", "oai-device-id"),
		PlusTrialEligible: false,
		PlusTrialChecked:  false,
	}
}

func browserAuthSessionFromResults(results []*browserautomationv1.BrowserCommandResult) browserAuthSession {
	data := browserAuthSessionData(results)
	session := browserAuthSession{}
	if token := stringMapValue(data, "sessionToken"); token != "" {
		session.sessionToken = token
	} else {
		session.sessionToken = stringMapValue(data, "session_token")
	}
	if token := stringMapValue(data, "accessToken"); token != "" {
		session.accessToken = token
	} else {
		session.accessToken = stringMapValue(data, "access_token")
	}
	return session
}

func browserAuthSessionData(results []*browserautomationv1.BrowserCommandResult) map[string]any {
	body := browserAuthCommandText(results, "extract-session-body")
	if body == "" {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil
	}
	return data
}

func browserAuthCommandText(results []*browserautomationv1.BrowserCommandResult, commandID string) string {
	for _, result := range results {
		if result.GetCommandId() == commandID {
			return strings.TrimSpace(result.GetText())
		}
	}
	return ""
}

func commandResultMap(results []*browserautomationv1.BrowserCommandResult, commandID string) map[string]any {
	for _, result := range results {
		if result.GetCommandId() != commandID {
			continue
		}
		if result.GetJsonValue() == nil {
			return nil
		}
		if value, ok := result.GetJsonValue().AsInterface().(map[string]any); ok {
			return value
		}
		return nil
	}
	return nil
}

func browserAuthCommandSucceeded(results []*browserautomationv1.BrowserCommandResult, commandID string) bool {
	for _, result := range results {
		if result.GetCommandId() == commandID {
			return result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_SUCCEEDED
		}
	}
	return false
}

func browserAuthMatchedCount(results []*browserautomationv1.BrowserCommandResult, commandID string) int {
	for _, result := range results {
		if result.GetCommandId() == commandID &&
			result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_SUCCEEDED {
			return int(result.GetMatchedCount())
		}
	}
	return 0
}

func browserAuthAnyCommandSucceeded(results []*browserautomationv1.BrowserCommandResult, commandIDs ...string) bool {
	wanted := map[string]bool{}
	for _, commandID := range commandIDs {
		wanted[commandID] = true
	}
	for _, result := range results {
		if wanted[result.GetCommandId()] && result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_SUCCEEDED {
			return true
		}
	}
	return false
}

func browserAuthPageStateData(results []*browserautomationv1.BrowserCommandResult, commandID string) map[string]any {
	data := commandResultMap(results, commandID)
	if data == nil {
		return nil
	}
	out := map[string]any{}
	if url := sanitizeBrowserAuthURL(stringMapValue(data, "url")); url != "" {
		out["url"] = url
	}
	if title := stringMapValue(data, "title"); title != "" {
		out["title"] = title
	}
	if inputs := browserAuthStringList(data["inputs"], 5, 120); len(inputs) > 0 {
		out["inputs"] = inputs
	}
	if actions := browserAuthStringList(data["actions"], 8, 120); len(actions) > 0 {
		out["actions"] = actions
	} else if hints := browserAuthTextHints(stringMapValue(data, "text")); len(hints) > 0 {
		out["actions"] = hints
	}
	return out
}

func browserAuthPageHasAny(data map[string]any, terms ...string) bool {
	if data == nil {
		return false
	}
	haystack := strings.ToLower(fmt.Sprint(data["url"]) + " " + fmt.Sprint(data["title"]) + " " + fmt.Sprint(data["inputs"]) + " " + fmt.Sprint(data["actions"]))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(haystack, term) {
			return true
		}
	}
	return false
}

func sanitizeBrowserAuthURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if before, _, ok := strings.Cut(raw, "?"); ok {
		raw = before
	}
	if before, _, ok := strings.Cut(raw, "#"); ok {
		raw = before
	}
	return raw
}

func browserAuthTextHints(text string) []string {
	keywords := []string{
		"Sign up", "Log in", "Sign in", "Create account", "Continue", "Next",
		"Open profile menu", "New chat", "Settings", "Log out", "Email",
	}
	seen := map[string]bool{}
	hints := make([]string, 0, 8)
	for _, rawLine := range strings.Split(text, "\n") {
		line := compactBrowserAuthText(rawLine, 80)
		if line == "" || seen[line] {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				seen[line] = true
				hints = append(hints, line)
				break
			}
		}
		if len(hints) >= 8 {
			break
		}
	}
	return hints
}

func stringMapValue(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func int64MapValue(data map[string]any, key string) int64 {
	if data == nil {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return n
	default:
		n, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(typed)), 10, 64)
		return n
	}
}

func unixSecondsFromMillis(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value / 1000
}

func browserCookieMaps(data map[string]any) []map[string]string {
	if data == nil {
		return nil
	}
	rawCookies, ok := data["cookies"].([]any)
	if !ok {
		return nil
	}
	cookies := make([]map[string]string, 0, len(rawCookies))
	for _, raw := range rawCookies {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cookies = append(cookies, map[string]string{
			"name":   stringMapValue(item, "name"),
			"value":  stringMapValue(item, "value"),
			"domain": stringMapValue(item, "domain"),
		})
	}
	return cookies
}

func extractBrowserSessionToken(cookies []map[string]string) string {
	type cookiePart struct {
		name  string
		value string
	}
	parts := make([]cookiePart, 0)
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie["name"])
		value := strings.TrimSpace(cookie["value"])
		if value != "" && isBrowserSessionCookieName(name) {
			parts = append(parts, cookiePart{name: name, value: value})
		}
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Slice(parts, func(i, j int) bool {
		baseI, suffixI := browserSessionCookieOrder(parts[i].name)
		baseJ, suffixJ := browserSessionCookieOrder(parts[j].name)
		if baseI != baseJ {
			return baseI < baseJ
		}
		if suffixI != suffixJ {
			return suffixI < suffixJ
		}
		return parts[i].name < parts[j].name
	})
	if _, suffix := browserSessionCookieOrder(parts[0].name); suffix < 0 {
		return parts[0].value
	}
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(part.value)
	}
	return builder.String()
}

func isBrowserSessionCookieName(name string) bool {
	base, _ := browserSessionCookieOrder(name)
	return base < 99
}

func browserSessionCookieOrder(name string) (int, int) {
	bases := []string{
		"__Secure-next-auth.session-token",
		"next-auth.session-token",
		"__Secure-authjs.session-token",
		"authjs.session-token",
	}
	for baseOrder, base := range bases {
		if name == base {
			return baseOrder, -1
		}
		prefix := base + "."
		if strings.HasPrefix(name, prefix) {
			if suffix, err := strconv.Atoi(strings.TrimPrefix(name, prefix)); err == nil {
				return baseOrder, suffix
			}
		}
	}
	return 99, 0
}

func extractCookieValue(cookies []map[string]string, names ...string) string {
	for _, cookie := range cookies {
		for _, name := range names {
			if cookie["name"] == name {
				return cookie["value"]
			}
		}
	}
	return ""
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

type browserAuthBirthdayParts struct {
	Month       string
	MonthPadded string
	MonthName   string
	MonthShort  string
	Day         string
	DayPadded   string
	Year        string
	US          string
}

func browserAuthBirthdayPartsFrom(value string) browserAuthBirthdayParts {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r < '0' || r > '9'
	})
	month, day, year := 1, 15, 1990
	if len(parts) >= 3 {
		first, _ := strconv.Atoi(parts[0])
		second, _ := strconv.Atoi(parts[1])
		third, _ := strconv.Atoi(parts[2])
		if len(parts[0]) == 4 {
			year, month, day = first, second, third
		} else {
			month, day, year = first, second, third
		}
	}
	if month < 1 || month > 12 {
		month = 1
	}
	if day < 1 || day > 31 {
		day = 15
	}
	if year < 1900 || year > 2100 {
		year = 1990
	}
	monthNames := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthName := monthNames[month-1]
	return browserAuthBirthdayParts{
		Month:       strconv.Itoa(month),
		MonthPadded: fmt.Sprintf("%02d", month),
		MonthName:   monthName,
		MonthShort:  monthName[:3],
		Day:         strconv.Itoa(day),
		DayPadded:   fmt.Sprintf("%02d", day),
		Year:        strconv.Itoa(year),
		US:          fmt.Sprintf("%02d/%02d/%04d", month, day, year),
	}
}

func browserAuthAgeFromBirthday(value string) string {
	birthday := browserAuthBirthdayPartsFrom(value)
	month, _ := strconv.Atoi(birthday.Month)
	day, _ := strconv.Atoi(birthday.Day)
	year, _ := strconv.Atoi(birthday.Year)
	now := time.Now()
	age := now.Year() - year
	if int(now.Month()) < month || (int(now.Month()) == month && now.Day() < day) {
		age--
	}
	if age < 18 || age > 100 {
		return "35"
	}
	return strconv.Itoa(age)
}

func compactStringValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func browserAuthEntrySelector(mode string) *browserautomationv1.BrowserSelectorGroup {
	if mode == "login" {
		return selectorGroup(3*time.Second,
			roleSelector("button", "Log in", false),
			roleSelector("link", "Log in", false),
			textSelector("Log in", false),
			roleSelector("button", "Sign in", false),
			roleSelector("link", "Sign in", false),
			textSelector("Sign in", false),
		)
	}
	return selectorGroup(3*time.Second,
		roleSelector("button", "Sign up for free", false),
		roleSelector("link", "Sign up for free", false),
		textSelector("Sign up for free", false),
		roleSelector("button", "Sign up", false),
		roleSelector("link", "Sign up", false),
		textSelector("Sign up", false),
		roleSelector("button", "Create account", false),
		roleSelector("link", "Create account", false),
		textSelector("Create account", false),
		roleSelector("button", "Get started", false),
		roleSelector("link", "Get started", false),
		textSelector("Get started", false),
		roleSelector("button", "Try ChatGPT", false),
		roleSelector("link", "Try ChatGPT", false),
		textSelector("Try ChatGPT", false),
	)
}

func browserAuthProfileMenuSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`button[aria-label="Open profile menu"],button[aria-label*="profile menu" i]`),
		roleSelector("button", "Open profile menu", true),
		textSelector("Open profile menu", true),
	)
}

func browserAuthEmailSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[type="email"],input[name*="email" i],input[id*="email" i],input[autocomplete="email"],input[placeholder*="email" i],input[aria-label*="email" i],input[name="username"],input[id*="username" i],input[autocomplete="username"],input[name="identifier"],input[id*="identifier" i],input[placeholder*="identifier" i],input[aria-label*="identifier" i]`)
}

func browserAuthPasswordSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[type="password"],input[name*="password" i],input[id*="password" i],input[autocomplete="current-password"],input[autocomplete="new-password"],input[placeholder*="password" i],input[aria-label*="password" i]`)
}

func browserAuthRegisterEmailSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input#email[name="email"][type="email"][placeholder="Email address"][aria-label="Email address"]`)
}

func browserAuthRejectCookiesSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		roleSelector("button", "Reject non-essential", true),
	)
}

func browserAuthRegisterPasswordSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="new-password"][autocomplete="new-password"][placeholder="Password"][type="password"]`)
}

func browserAuthRegisterOTPSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="code"][autocomplete="one-time-code"][placeholder="Code"]`)
}

func browserAuthLoginEmailAdvancedSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(5*time.Second,
		browserAuthLoginOTPSelector(),
		roleSelector("link", "Continue with password", true),
		browserAuthLoginPasswordSelector(),
	)
}

func browserAuthLoginPasswordSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="current-password"][type="password"]`)
}

func browserAuthLoginOTPSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="code"][autocomplete="one-time-code"][placeholder="Code"]`)
}

func browserAuthRegisterProfileNameSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="name"][autocomplete="name"][placeholder="Full name"][type="text"]`)
}

func browserAuthRegisterAgeSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="age"][autocomplete="off"][placeholder="Age"][type="number"]`)
}

func browserAuthEmailProviderSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		roleSelector("button", "Continue with email", false),
		textSelector("Continue with email", false),
		roleSelector("button", "Continue", true),
		textSelector("Continue", true),
		roleSelector("button", "Use email", false),
		textSelector("Use email", false),
		roleSelector("button", "Sign up with email", false),
		textSelector("Sign up with email", false),
		roleSelector("button", "Log in with email", false),
		textSelector("Log in with email", false),
	)
}

func browserAuthEmailSubmitSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(3*time.Second,
		roleSelector("button", "Continue", true),
		textSelector("Continue", true),
		roleSelector("button", "Next", true),
		textSelector("Next", true),
		roleSelector("button", "Sign up", true),
		textSelector("Sign up", true),
		roleSelector("button", "Create account", true),
		textSelector("Create account", true),
		roleSelector("button", "Log in", true),
		textSelector("Log in", true),
	)
}

func browserAuthProfileNameSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`input[name="name"],input[name*="full" i],input[name*="display" i],input[id*="name" i],input[autocomplete="name"],input[placeholder*="name" i],input[aria-label*="name" i]`),
		labelSelector("Full name", false),
		labelSelector("Name", true),
		placeholderSelector("Full name", false),
		placeholderSelector("Name", true),
		roleSelector("textbox", "Full name", false),
		roleSelector("textbox", "Name", true),
	)
}

func browserAuthAgeSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`input[name*="age" i],input[id*="age" i],input[placeholder*="age" i],input[aria-label*="age" i],input[type="number"]`),
		labelSelector("Age", false),
		placeholderSelector("Age", false),
		roleSelector("spinbutton", "Age", false),
		roleSelector("textbox", "Age", false),
	)
}

func browserAuthBirthdaySelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`input[name*="birth" i],input[name*="dob" i],input[id*="birth" i],input[id*="dob" i],input[autocomplete="bday"],input[placeholder*="MM/DD" i],input[placeholder*="birth" i],input[aria-label*="birth" i],input[aria-label*="date of birth" i]`),
		labelSelector("Birthday", false),
		labelSelector("Date of birth", false),
		placeholderSelector("MM/DD/YYYY", false),
		roleSelector("textbox", "Birthday", false),
		roleSelector("textbox", "Date of birth", false),
	)
}

func browserAuthMonthInputSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`input[name*="month" i],input[id*="month" i],input[placeholder*="month" i],input[aria-label*="month" i]`),
		labelSelector("Month", false),
		placeholderSelector("Month", false),
		roleSelector("textbox", "Month", false),
	)
}

func browserAuthDayInputSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`input[name*="day" i],input[id*="day" i],input[placeholder*="day" i],input[aria-label*="day" i]`),
		labelSelector("Day", false),
		placeholderSelector("Day", false),
		roleSelector("textbox", "Day", false),
	)
}

func browserAuthYearInputSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`input[name*="year" i],input[id*="year" i],input[placeholder*="year" i],input[aria-label*="year" i]`),
		labelSelector("Year", false),
		placeholderSelector("Year", false),
		roleSelector("textbox", "Year", false),
	)
}

func browserAuthMonthSelectSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`select[name*="month" i],select[id*="month" i],select[aria-label*="month" i]`),
		labelSelector("Month", false),
	)
}

func browserAuthDaySelectSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`select[name*="day" i],select[id*="day" i],select[aria-label*="day" i]`),
		labelSelector("Day", false),
	)
}

func browserAuthYearSelectSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`select[name*="year" i],select[id*="year" i],select[aria-label*="year" i]`),
		labelSelector("Year", false),
	)
}

func browserAuthPostOTPContinueSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(3*time.Second,
		cssSelector(`button[type="submit"],input[type="submit"]`),
		roleSelector("button", "Continue", true),
		textSelector("Continue", true),
		roleSelector("button", "Next", true),
		textSelector("Next", true),
		roleSelector("button", "Finish", true),
		textSelector("Finish", true),
		roleSelector("button", "Finish creating account", true),
		textSelector("Finish creating account", true),
		roleSelector("button", "Create", true),
		textSelector("Create", true),
		roleSelector("button", "Agree", true),
		textSelector("Agree", true),
	)
}

func selectorGroup(timeout time.Duration, selectors ...*browserautomationv1.BrowserSelector) *browserautomationv1.BrowserSelectorGroup {
	return &browserautomationv1.BrowserSelectorGroup{
		Selectors: selectors,
		Timeout:   durationpb.New(timeout),
	}
}

func cssSelector(value string) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_CSS,
		Value: value,
	}
}

func textSelector(value string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_TEXT,
		Value: value,
		Exact: exact,
	}
}

func roleSelector(role, name string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:     browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_ROLE,
		Value:    name,
		RoleName: role,
		Exact:    exact,
	}
}

func labelSelector(value string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_LABEL,
		Value: value,
		Exact: exact,
	}
}

func placeholderSelector(value string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_PLACEHOLDER,
		Value: value,
		Exact: exact,
	}
}

func browserAuthLanguageOverrideScript(locale string) string {
	if locale == "" {
		locale = "en-US"
	}
	return fmt.Sprintf(`(() => {
  const language = %q;
  const languages = [language, "en"];
  const define = (object, property, value) => {
    try {
      Object.defineProperty(object, property, {get: () => value, configurable: true});
    } catch (_) {}
  };
  define(Navigator.prototype, "language", language);
  define(Navigator.prototype, "languages", languages);
})();`, locale)
}
