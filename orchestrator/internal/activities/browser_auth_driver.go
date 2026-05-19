package activities

import (
	"context"
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
	"google.golang.org/protobuf/types/known/structpb"
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

	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}
	if err := f.openEntry(client, cfg); err != nil {
		f.fail(err)
		return
	}
	if f.cancelled() {
		f.cancelFlow("browser auth cancelled")
		return
	}

	f.setStatus(browserAuthStageEmailEntry, "submitting email")
	if err := f.submitEmail(client, cfg); err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageCredentialEntry, "submitting credentials")
	state, err := f.submitCredentials(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	if f.handleTerminalState(client, cfg, state) {
		return
	}
	if state != "password_submitted" && state != "otp_required" {
		f.fail(fmt.Errorf("browser %s credential step did not reach OTP: %s", f.mode, state))
		return
	}

	f.markOTPRequestClicked()
	state, err = f.waitForOTPInput(client, cfg)
	if err != nil {
		f.fail(err)
		return
	}
	if f.handleTerminalState(client, cfg, state) {
		return
	}
	if state != "otp_required" {
		f.fail(fmt.Errorf("browser %s OTP input not reached: %s", f.mode, state))
		return
	}

	otp, err := f.waitForOTP()
	if err != nil {
		f.fail(err)
		return
	}
	f.setStatus(browserAuthStageOTPSubmit, "submitting OTP")
	if err := f.submitOTP(client, cfg, otp); err != nil {
		f.fail(err)
		return
	}
	if err := f.captureResult(client, cfg); err != nil {
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

func (f *browserAuthFlow) openEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	_, err := f.execute(client, cfg, "open-entry", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-chatgpt", "https://chatgpt.com/", cfg.CommandTimeout),
		evaluateCommand("click-entry", browserAuthClickEntryScript, map[string]any{"mode": f.mode}, cfg.CommandTimeout),
	})
	return err
}

func (f *browserAuthFlow) submitEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	results, err := f.execute(client, cfg, "submit-email", []*browserautomationv1.BrowserCommand{
		evaluateCommand("submit-email", browserAuthSubmitEmailScript, map[string]any{"email": f.email}, cfg.CommandTimeout),
	})
	if err != nil {
		return err
	}
	if state := browserAuthState(results, "submit-email"); state != "submitted" {
		return fmt.Errorf("browser %s email step failed: %s", f.mode, state)
	}
	return nil
}

func (f *browserAuthFlow) submitCredentials(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	results, err := f.execute(client, cfg, "submit-credentials", []*browserautomationv1.BrowserCommand{
		evaluateCommand("submit-credentials", browserAuthSubmitCredentialsScript, map[string]any{
			"mode":     f.mode,
			"password": f.password,
		}, cfg.CommandTimeout),
	})
	if err != nil {
		return "", err
	}
	return browserAuthState(results, "submit-credentials"), nil
}

func (f *browserAuthFlow) waitForOTPInput(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	results, err := f.execute(client, cfg, "wait-otp", []*browserautomationv1.BrowserCommand{
		evaluateCommand("wait-otp", browserAuthWaitOTPScript, nil, 2*cfg.CommandTimeout),
	})
	if err != nil {
		return "", err
	}
	return browserAuthState(results, "wait-otp"), nil
}

func (f *browserAuthFlow) submitOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) error {
	_, err := f.execute(client, cfg, "submit-otp", []*browserautomationv1.BrowserCommand{
		evaluateCommand("submit-otp", browserAuthSubmitOTPScript, map[string]any{
			"otp":      otp,
			"fullName": f.fullName,
			"birthday": f.birthday,
		}, 2*cfg.CommandTimeout),
	})
	return err
}

func (f *browserAuthFlow) captureResult(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	f.setStatus(browserAuthStageSessionCapture, "capturing browser session")
	results, err := f.execute(client, cfg, "capture-session", []*browserautomationv1.BrowserCommand{
		evaluateCommand("capture-session", browserAuthCaptureSessionScript, nil, cfg.CommandTimeout),
		{
			CommandId:  "capture-cookies",
			CommandKey: "capture-cookies",
			Timeout:    durationpb.New(cfg.CommandTimeout),
			Operation: &browserautomationv1.BrowserCommand_GetCookies{
				GetCookies: &browserautomationv1.GetCookiesCommand{Urls: []string{"https://chatgpt.com/"}},
			},
		},
	})
	if err != nil {
		return err
	}
	result := browserAuthRegisterResponse(results)
	if result.GetSessionToken() == "" {
		return fmt.Errorf("missing session token after browser %s", f.mode)
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
		if err := f.captureResult(client, cfg); err != nil {
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
		RequestId: "gpt-browser-auth-" + f.flowID + "-" + taskKey,
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
	for _, result := range resp.GetResults() {
		if result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_FAILED ||
			result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_TIMEOUT {
			if result.GetError() != nil {
				return resp.GetResults(), errors.New(result.GetError().GetMessage())
			}
			return resp.GetResults(), fmt.Errorf("browser command %s failed", result.GetCommandKey())
		}
	}
	return resp.GetResults(), nil
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
	now := time.Now().Unix()
	f.mu.Lock()
	if f.otpAction <= 0 {
		f.otpAction = now
	}
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

func evaluateCommand(commandID, expression string, args map[string]any, timeout time.Duration) *browserautomationv1.BrowserCommand {
	structArgs, err := structpb.NewStruct(args)
	if err != nil {
		structArgs = &structpb.Struct{}
	}
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_Evaluate{
			Evaluate: &browserautomationv1.EvaluateCommand{
				Expression: expression,
				Args:       structArgs,
				Timeout:    durationpb.New(timeout),
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
	data := commandResultMap(results, commandID)
	if data == nil {
		return ""
	}
	return strings.TrimSpace(stringMapValue(data, "state"))
}

func browserAuthRegisterResponse(results []*browserautomationv1.BrowserCommandResult) *pb.RegisterResponse {
	session := commandResultMap(results, "capture-session")
	cookiesData := commandResultMap(results, "capture-cookies")
	cookies := browserCookieMaps(cookiesData)
	return &pb.RegisterResponse{
		Success:           true,
		SessionToken:      extractBrowserSessionToken(cookies),
		AccessToken:       stringMapValue(session, "access_token"),
		DeviceId:          extractCookieValue(cookies, "oai-did", "oai-device-id"),
		PlusTrialEligible: false,
		PlusTrialChecked:  false,
	}
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

const browserAuthClickEntryScript = `async ({mode}) => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const visible = (el) => {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
  };
  const clickText = (pattern) => {
    for (const el of document.querySelectorAll("a,button,input[type=submit],div[role=button]")) {
      const text = (el.innerText || el.textContent || el.value || "").trim();
      if (!el.disabled && visible(el) && pattern.test(text)) {
        el.click();
        return true;
      }
    }
    return false;
  };
  for (const selector of mode === "login"
    ? ['a[data-testid="login-button"]', 'button[data-testid="login-button"]']
    : ['a[data-testid="signup-button"]', 'button[data-testid="signup-button"]']) {
    const el = document.querySelector(selector);
    if (visible(el)) {
      el.click();
      await sleep(1500);
      return {state: "clicked", url: location.href};
    }
  }
  clickText(mode === "login" ? /^log in$/i : /^sign up$/i);
  await sleep(1500);
  return {state: "clicked", url: location.href};
}`

const browserAuthSubmitEmailScript = `async ({email}) => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const visible = (el) => {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
  };
  const fill = (el, value) => {
    el.focus();
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
    if (setter) setter.call(el, value); else el.value = value;
    el.dispatchEvent(new Event("input", {bubbles: true}));
    el.dispatchEvent(new Event("change", {bubbles: true}));
  };
  const inputSelector = 'input[type="email"],input[name*="email" i],input[autocomplete="email"],input[placeholder*="email" i],input[aria-label*="email" i],input[name="username"]';
  for (let i = 0; i < 80; i++) {
    const input = Array.from(document.querySelectorAll(inputSelector)).find(visible);
    if (input) {
      fill(input, email);
      await sleep(250);
      for (const el of document.querySelectorAll("button,input[type=submit],a,div[role=button]")) {
        const text = (el.innerText || el.textContent || el.value || "").trim();
        if (!el.disabled && visible(el) && /^(continue|next)$/i.test(text)) {
          el.click();
          await sleep(1200);
          return {state: "submitted", url: location.href};
        }
      }
      if (input.form) {
        if (input.form.requestSubmit) input.form.requestSubmit(); else input.form.submit();
        await sleep(1200);
        return {state: "submitted", url: location.href};
      }
    }
    await sleep(750);
  }
  return {state: "email_input_missing", url: location.href};
}`

const browserAuthSubmitCredentialsScript = `async ({mode, password}) => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const visible = (el) => {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
  };
  const fill = (el, value) => {
    el.focus();
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    if (setter) setter.call(el, value); else el.value = value;
    el.dispatchEvent(new Event("input", {bubbles: true}));
    el.dispatchEvent(new Event("change", {bubbles: true}));
  };
  const clickByText = (pattern) => {
    for (const el of document.querySelectorAll("button,input[type=submit],a,div[role=button]")) {
      const text = (el.innerText || el.textContent || el.value || "").trim();
      if (!el.disabled && visible(el) && pattern.test(text)) {
        el.click();
        return true;
      }
    }
    return false;
  };
  const otpInput = () => Array.from(document.querySelectorAll('input[autocomplete="one-time-code"],input[name="code"],input[inputmode="numeric"],input[aria-label*="code" i],input[placeholder*="code" i],input[maxlength="1"]')).find(visible);
  for (let i = 0; i < 120; i++) {
    const text = document.body ? document.body.innerText || "" : "";
    if (/user_already_exists|account already exists/i.test(text)) return {state: "user_already_exists", url: location.href};
    if (location.hostname.endsWith("chatgpt.com") && !location.hostname.startsWith("auth.")) return {state: "session_ready", url: location.href};
    if (otpInput()) return {state: "otp_required", url: location.href};
    if (mode === "login") clickByText(/^(continue with password|use password|password)$/i);
    const passwordInput = Array.from(document.querySelectorAll('input[type="password"],input[name="password"]')).find(visible);
    if (passwordInput) {
      fill(passwordInput, password);
      await sleep(300);
      clickByText(/^(continue|next|log in|sign up)$/i);
      await sleep(1500);
      return {state: "password_submitted", url: location.href};
    }
    await sleep(1000);
  }
  return {state: "credential_input_missing", url: location.href};
}`

const browserAuthWaitOTPScript = `async () => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const visible = (el) => {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
  };
  const otpInput = () => Array.from(document.querySelectorAll('input[autocomplete="one-time-code"],input[name="code"],input[inputmode="numeric"],input[aria-label*="code" i],input[placeholder*="code" i],input[maxlength="1"]')).find(visible);
  const clickContinue = () => {
    for (const el of document.querySelectorAll("button,input[type=submit],a,div[role=button]")) {
      const text = (el.innerText || el.textContent || el.value || "").trim();
      if (!el.disabled && visible(el) && /^(continue|next)$/i.test(text)) {
        el.click();
        return true;
      }
    }
    return false;
  };
  for (let i = 0; i < 160; i++) {
    const text = document.body ? document.body.innerText || "" : "";
    if (/user_already_exists|account already exists/i.test(text)) return {state: "user_already_exists", url: location.href};
    if (location.hostname.endsWith("chatgpt.com") && !location.hostname.startsWith("auth.")) return {state: "session_ready", url: location.href};
    if (otpInput()) return {state: "otp_required", url: location.href};
    if (i % 10 === 5) clickContinue();
    await sleep(1000);
  }
  return {state: "otp_input_missing", url: location.href};
}`

const browserAuthSubmitOTPScript = `async ({otp, fullName, birthday}) => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const visible = (el) => {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
  };
  const fill = (el, value) => {
    el.focus();
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
    if (setter) setter.call(el, value); else el.value = value;
    el.dispatchEvent(new Event("input", {bubbles: true}));
    el.dispatchEvent(new Event("change", {bubbles: true}));
  };
  const clickSubmit = (pattern) => {
    for (const el of document.querySelectorAll("button,input[type=submit],a,div[role=button]")) {
      const text = (el.innerText || el.textContent || el.value || "").trim();
      if (!el.disabled && visible(el) && pattern.test(text)) {
        el.click();
        return true;
      }
    }
    return false;
  };
  const fillAboutYou = () => {
    const inputs = Array.from(document.querySelectorAll("input")).filter((el) => {
      const type = (el.type || "").toLowerCase();
      return visible(el) && !["hidden", "submit", "button", "checkbox", "radio", "password"].includes(type);
    });
    if (inputs.length < 2) return false;
    const birthIndex = inputs.findIndex((el) => {
      const blob = [el.type, el.name, el.placeholder, el.getAttribute("aria-label") || ""].join(" ").toLowerCase();
      return blob.includes("birth") || blob.includes("birthday") || blob.includes("dob") || blob.includes("mm/dd/yyyy");
    });
    const birth = inputs[birthIndex >= 0 ? birthIndex : 1];
    const name = inputs.find((el) => el !== birth) || inputs[0];
    fill(name, fullName || "Alex Morgan");
    fill(birth, birthday || "01/15/1990");
    clickSubmit(/^(finish|create|agree|continue|next)$/i);
    return true;
  };
  const single = Array.from(document.querySelectorAll('input[autocomplete="one-time-code"],input[name="code"],input[inputmode="numeric"],input[aria-label*="code" i],input[placeholder*="code" i],input[type="text"]')).find(visible);
  const digits = Array.from(document.querySelectorAll('input[maxlength="1"]')).filter(visible);
  if (single && single.getAttribute("maxlength") !== "1") {
    fill(single, otp);
  } else if (digits.length >= 6) {
    otp.slice(0, 6).split("").forEach((ch, index) => fill(digits[index], ch));
  } else if (single) {
    fill(single, otp);
  } else {
    return {state: "otp_input_missing", url: location.href};
  }
  await sleep(500);
  clickSubmit(/^(continue|verify|next)$/i);
  for (let i = 0; i < 160; i++) {
    const text = document.body ? document.body.innerText || "" : "";
    if (/user_already_exists|account already exists/i.test(text)) return {state: "user_already_exists", url: location.href};
    if (location.hostname.endsWith("chatgpt.com") && !location.hostname.startsWith("auth.")) return {state: "session_ready", url: location.href};
    if (location.href.includes("about-you")) fillAboutYou();
    await sleep(1000);
  }
  return {state: "session_timeout", url: location.href};
}`

const browserAuthCaptureSessionScript = `async () => {
  let data = {};
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 8000);
    const response = await fetch("/api/auth/session", {credentials: "include", signal: controller.signal});
    clearTimeout(timer);
    data = await response.json();
  } catch (_) {}
  return {
    url: location.href,
    access_token: data.accessToken || data.access_token || ""
  };
}`
