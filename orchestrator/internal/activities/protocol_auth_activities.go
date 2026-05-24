package activities

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
	"orchestrator/pb"
)

const (
	protocolAuthChatGPTLoginURL   = "https://chatgpt.com/auth/login"
	protocolAuthChatGPTCallback   = "https://chatgpt.com/api/auth/callback/openai"
	protocolAuthChatGPTSessionURL = "https://chatgpt.com/api/auth/session"
)

func (s *Server) ProtocolAuthStartActivity(ctx context.Context, input BrowserAuthStartInput) (BrowserAuthStartOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	output := BrowserAuthStartOutput{AccountId: input.GetAccountId(), OtpTimeoutSeconds: s.registrationOtpTimeout()}
	account, err := s.protocolAuthAccount(ctx, input.GetAccountId(), input.GetMode())
	if err != nil {
		return output, err
	}
	stepName, err := protocolAuthStartStepName(input.GetMode())
	if err != nil {
		return output, err
	}
	data := protocolAuthData(input.GetMode(), account)
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "starting protocol auth", data)
	defer stopHeartbeat()

	if err := s.startCodexOAuthProtocolProxySession(ctx, cfg, "account_"+input.GetMode(), data); err != nil {
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	state, err := newProtocolAuthState()
	if err != nil {
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	client, err := newCodexOAuthProtocolHTTPClient(cfg, state)
	if err != nil {
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	authURL, err := protocolAuthChatGPTAuthURL(ctx, client, state, account, input.GetMode(), data)
	var issuedAfter int64
	if err == nil {
		authorizeIssuedAfter := time.Now().Add(-2 * time.Second).Unix()
		_, err = runCodexOAuthProtocolURL(ctx, client, state, authURL, protocolAuthChatGPTLoginURL, data)
		if err == nil && state.Stage == "email_otp" {
			issuedAfter = authorizeIssuedAfter
		}
	}
	if err == nil && state.Stage != "callback" && state.Stage != "email_otp" {
		_, issuedAfter, err = protocolAuthSubmitEmail(ctx, client, state, account, input.GetMode(), data)
	}
	if issuedAfter > 0 {
		state.EmailOTPIssuedAfterUnix = issuedAfter
	}
	if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
		err = saveErr
	}
	output.FlowId = state.FlowID
	output.Email = account.GetEmail()
	output.Data = protoData(data)
	if err != nil {
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	if protocolAuthReadyStage(state.Stage) {
		result, captureErr := s.protocolAuthCaptureSession(ctx, client, state, data)
		output.Result = &result
		output.Data = protoData(data)
		err = captureErr
		if err == nil {
			_ = s.deleteCodexOAuthProtocolState(ctx, input.GetJobId(), &CodexOAuthBrowserSession{FlowId: state.FlowID})
		}
	}
	if state.Stage == "email_otp" {
		output.OtpRequired = true
		output.OtpIssuedAfterUnix = state.EmailOTPIssuedAfterUnix
	}
	data["flow_id"] = state.FlowID
	data["login_stage"] = state.Stage
	data["protocol_session_started"] = true
	output.Data = protoData(data)
	return output, step.complete(data, err)
}

func (s *Server) ProtocolAuthWaitActivity(ctx context.Context, input BrowserAuthWaitInput) (BrowserAuthWaitOutput, error) {
	output := BrowserAuthWaitOutput{
		AccountId:          input.GetAccountId(),
		FlowId:             input.GetFlowId(),
		Email:              input.GetEmail(),
		OtpTimeoutSeconds:  s.registrationOtpTimeout(),
		OtpIssuedAfterUnix: time.Now().Add(-time.Second).Unix(),
	}
	if strings.TrimSpace(input.GetFlowId()) == "" {
		return output, fmt.Errorf("protocol flow_id is required")
	}
	stepName, err := protocolAuthWaitStepName(input.GetMode())
	if err != nil {
		return output, err
	}
	account, err := s.protocolAuthAccount(ctx, input.GetAccountId(), input.GetMode())
	if err != nil {
		return output, err
	}
	data := protocolAuthData(input.GetMode(), account)
	data["flow_id"] = input.GetFlowId()
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "running protocol auth", data)
	defer stopHeartbeat()

	state, client, err := s.protocolAuthStateClient(ctx, input.GetJobId(), input.GetFlowId())
	if err != nil {
		output.Data = protoData(data)
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	_, err = protocolAuthAdvanceStage(ctx, client, state, data)
	var issuedAfter int64
	if err == nil {
		switch state.Stage {
		case "email", "":
			_, issuedAfter, err = protocolAuthSubmitEmail(ctx, client, state, account, input.GetMode(), data)
		case "create_password":
			_, issuedAfter, err = s.protocolAuthRegisterPassword(ctx, client, state, account, data)
		case "password":
			_, issuedAfter, err = protocolAuthLoginPassword(ctx, client, state, account, data)
		case "about_you":
			if input.GetMode() == browserAuthModeRegister {
				_, err = s.protocolAuthCreateAccount(ctx, client, state, account, data)
			}
		}
	}
	if issuedAfter > 0 {
		state.EmailOTPIssuedAfterUnix = issuedAfter
	}
	if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
		err = saveErr
	}
	output.Email = account.GetEmail()
	output.OtpIssuedAfterUnix = state.EmailOTPIssuedAfterUnix
	output.Data = protoData(data)
	if err != nil {
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	if protocolAuthReadyStage(state.Stage) {
		result, captureErr := s.protocolAuthCaptureSession(ctx, client, state, data)
		output.Result = &result
		output.Data = protoData(data)
		if captureErr == nil {
			_ = s.deleteCodexOAuthProtocolState(ctx, input.GetJobId(), &CodexOAuthBrowserSession{FlowId: input.GetFlowId()})
		}
		return output, step.complete(data, captureErr)
	}
	if state.Stage == "email_otp" {
		output.OtpRequired = true
		if output.OtpIssuedAfterUnix <= 0 {
			output.OtpIssuedAfterUnix = time.Now().Add(-time.Second).Unix()
			state.EmailOTPIssuedAfterUnix = output.OtpIssuedAfterUnix
			_ = s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state)
		}
		data["email_otp_issued_after_unix"] = output.OtpIssuedAfterUnix
		output.Data = protoData(data)
		return output, step.complete(data, nil)
	}
	err = fmt.Errorf("protocol auth stage not ready: %s", state.Stage)
	return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
}

func (s *Server) ProtocolAuthCompleteActivity(ctx context.Context, input BrowserAuthCompleteInput) (RegisterActivityOutput, error) {
	stepName, err := protocolAuthCompleteStepName(input.GetMode())
	if err != nil {
		return RegisterActivityOutput{}, err
	}
	account, err := s.protocolAuthAccount(ctx, input.GetAccountId(), input.GetMode())
	if err != nil {
		return RegisterActivityOutput{}, err
	}
	data := protocolAuthData(input.GetMode(), account)
	data["flow_id"] = input.GetFlowId()
	data["otp_source"] = input.GetOtpSource()
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, err
	}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepName, "completing protocol auth", data)
	defer stopHeartbeat()

	state, client, err := s.protocolAuthStateClient(ctx, input.GetJobId(), input.GetFlowId())
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	err = s.protocolAuthValidateEmailOTP(ctx, client, state, input, data)
	if err == nil && input.GetMode() == browserAuthModeRegister && (state.Stage == "about_you" || state.Stage == "email_otp" || state.Stage == "") {
		_, err = s.protocolAuthCreateAccount(ctx, client, state, account, data)
	}
	if err == nil && !protocolAuthReadyStage(state.Stage) {
		_, err = protocolAuthAdvanceStage(ctx, client, state, data)
	}
	if saveErr := s.saveCodexOAuthProtocolState(ctx, input.GetJobId(), state); saveErr != nil && err == nil {
		err = saveErr
	}
	var output RegisterActivityOutput
	if err == nil {
		output, err = s.protocolAuthCaptureSession(ctx, client, state, data)
	}
	output.Data = protoData(data)
	if err != nil {
		return output, s.completeBrowserAuthStep(ctx, input.GetJobId(), stepName, account.GetAccountId(), data, err)
	}
	_ = s.deleteCodexOAuthProtocolState(ctx, input.GetJobId(), &CodexOAuthBrowserSession{FlowId: input.GetFlowId()})
	return output, step.complete(data, nil)
}

func (s *Server) ProtocolAuthCancelActivity(ctx context.Context, input BrowserAuthCancelInput) error {
	return s.deleteCodexOAuthProtocolState(ctx, input.GetJobId(), &CodexOAuthBrowserSession{FlowId: input.GetFlowId()})
}

func newProtocolAuthState() (*codexOAuthProtocolState, error) {
	flowID, err := randomURLToken(18)
	if err != nil {
		return nil, err
	}
	return &codexOAuthProtocolState{FlowID: flowID, RedirectURI: protocolAuthChatGPTCallback}, nil
}

func protocolAuthChatGPTAuthURL(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, mode string, data map[string]any) (string, error) {
	csrfResp, err := client.get(ctx, "https://chatgpt.com/api/auth/csrf", protocolAuthChatGPTLoginURL, false)
	if err != nil {
		return "", err
	}
	if err := codexOAuthProtocolRequireOK(csrfResp, "chatgpt csrf"); err != nil {
		return "", err
	}
	csrf := strings.TrimSpace(stringAny(codexOAuthProtocolResponseJSON(csrfResp)["csrfToken"]))
	if csrf == "" {
		return "", fmt.Errorf("chatgpt csrf token missing")
	}
	if strings.TrimSpace(state.DeviceID) == "" {
		state.DeviceID = uuid.NewString()
	}
	signinURL := "https://chatgpt.com/api/auth/signin/openai"
	if account != nil && strings.TrimSpace(account.GetEmail()) != "" {
		query := url.Values{}
		query.Set("prompt", "login")
		query.Set("ext-oai-did", state.DeviceID)
		query.Set("auth_session_logging_id", uuid.NewString())
		query.Set("ext-passkey-client-capabilities", "1111")
		query.Set("screen_hint", protocolAuthChatGPTScreenHint(mode))
		query.Set("login_hint", strings.TrimSpace(account.GetEmail()))
		signinURL += "?" + query.Encode()
		data["chatgpt_signin_login_hint"] = true
		data["chatgpt_signin_screen_hint"] = protocolAuthChatGPTScreenHint(mode)
	}
	form := url.Values{}
	form.Set("csrfToken", csrf)
	form.Set("callbackUrl", "https://chatgpt.com/login")
	form.Set("json", "true")
	authResp, err := client.postForm(ctx, signinURL, protocolAuthChatGPTLoginURL, form)
	if err != nil {
		return "", err
	}
	if err := codexOAuthProtocolRequireOK(authResp, "chatgpt signin openai"); err != nil {
		return "", err
	}
	authURL := strings.TrimSpace(stringAny(codexOAuthProtocolResponseJSON(authResp)["url"]))
	if authURL == "" {
		return "", fmt.Errorf("chatgpt auth url missing")
	}
	state.AuthorizeURL = authURL
	state.RedirectURI = protocolAuthChatGPTCallback
	state.OAuthState = codexOAuthProtocolQueryFirst(authURL, "state")
	data["chatgpt_auth_url_ready"] = true
	return authURL, nil
}

func protocolAuthChatGPTScreenHint(mode string) string {
	if mode == browserAuthModeRegister {
		return "login_or_signup"
	}
	return "login"
}

func protocolAuthSubmitEmail(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, mode string, data map[string]any) (string, int64, error) {
	issuedAfter := time.Now().Add(-time.Second).Unix()
	referer := "https://auth.openai.com/log-in"
	screenHint := "login"
	if mode == browserAuthModeRegister {
		referer = "https://auth.openai.com/create-account"
		screenHint = "signup"
	}
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/authorize/continue", referer, map[string]any{
		"username":    map[string]any{"value": account.GetEmail(), "kind": "email"},
		"screen_hint": screenHint,
	}, codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue"))
	if err != nil {
		return "", 0, err
	}
	stage, err := protocolAuthAdvanceJSON(ctx, client, state, resp, referer, "email", data)
	if err != nil {
		return stage, 0, err
	}
	if stage == "email_otp" {
		if mode == browserAuthModeRegister {
			sendIssuedAfter := time.Now().Add(-2 * time.Second).Unix()
			stage, err = protocolAuthKickoffRegisterOTP(ctx, client, state, data)
			return stage, sendIssuedAfter, err
		}
		return stage, issuedAfter, nil
	}
	return stage, 0, nil
}

func (s *Server) protocolAuthRegisterPassword(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, int64, error) {
	_, _ = client.get(ctx, "https://auth.openai.com/create-account/password", "https://auth.openai.com/create-account", true)
	issuedAfter := time.Now().Add(-time.Second).Unix()
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/user/register", "https://auth.openai.com/create-account/password", map[string]any{
		"password": account.GetPassword(),
		"username": account.GetEmail(),
	}, codexOAuthProtocolSentinelHeader(ctx, client, state, data, "username_password_create"))
	if err != nil {
		return "", issuedAfter, err
	}
	stage, err := protocolAuthAdvanceJSON(ctx, client, state, resp, "https://auth.openai.com/create-account/password", "create_password", data)
	if err == nil && (stage == "" || stage == "create_password") {
		stage, err = protocolAuthKickoffRegisterOTP(ctx, client, state, data)
	}
	return stage, issuedAfter, err
}

func protocolAuthLoginPassword(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, int64, error) {
	issuedAfter := time.Now().Add(-time.Second).Unix()
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/password/verify", "https://auth.openai.com/log-in/password", map[string]any{"password": account.GetPassword()}, codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue"))
	if err != nil {
		return "", issuedAfter, err
	}
	stage, err := protocolAuthAdvanceJSON(ctx, client, state, resp, "https://auth.openai.com/log-in/password", "password", data)
	return stage, issuedAfter, err
}

func protocolAuthKickoffRegisterOTP(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, data map[string]any) (string, error) {
	sentinel := codexOAuthProtocolSentinelHeader(ctx, client, state, data, "username_password_create")
	attempts := []struct {
		method  string
		url     string
		referer string
	}{
		{fhttp.MethodPost, "https://auth.openai.com/api/accounts/passwordless/send-otp", "https://auth.openai.com/create-account/password"},
		{fhttp.MethodPost, "https://auth.openai.com/api/accounts/email-otp/resend", "https://auth.openai.com/email-verification"},
		{fhttp.MethodGet, "https://auth.openai.com/api/accounts/email-otp/send", "https://auth.openai.com/create-account/password"},
	}
	for _, attempt := range attempts {
		var resp *codexOAuthProtocolHTTPResponse
		var err error
		if attempt.method == fhttp.MethodGet {
			resp, err = client.request(ctx, attempt.method, attempt.url, attempt.referer, false, nil, sentinel)
		} else {
			resp, err = client.postJSON(ctx, attempt.url, attempt.referer, map[string]any{}, sentinel)
		}
		if err != nil {
			data["email_otp_send_error"] = codexOAuthProtocolSafeText(err.Error(), 180)
			continue
		}
		data["email_otp_send_status"] = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			state.Stage = "email_otp"
			data["login_stage"] = state.Stage
			return state.Stage, nil
		}
	}
	return "", fmt.Errorf("protocol register email otp send failed")
}

func (s *Server) protocolAuthValidateEmailOTP(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, input BrowserAuthCompleteInput, data map[string]any) error {
	otp, err := s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetOtpIssuedAfterUnix())
	if err != nil {
		return err
	}
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/email-otp/validate", "https://auth.openai.com/email-verification", map[string]any{"code": normalizeOTP(otp)}, codexOAuthProtocolSentinelHeader(ctx, client, state, data, "authorize_continue"))
	if err != nil {
		return err
	}
	_, err = protocolAuthAdvanceJSON(ctx, client, state, resp, "https://auth.openai.com/email-verification", "email_otp", data)
	return err
}

func (s *Server) protocolAuthCreateAccount(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, account *pb.Account, data map[string]any) (string, error) {
	name, birthdate := protocolAuthProfile(account)
	data["profile_name_present"] = name != ""
	data["profile_birthdate_fixed"] = birthdate == "2000-01-01"
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/create_account", "https://auth.openai.com/about-you", map[string]any{
		"name":      name,
		"birthdate": birthdate,
	}, codexOAuthProtocolSentinelHeader(ctx, client, state, data, "oauth_create_account"))
	if err != nil {
		return "", err
	}
	return protocolAuthAdvanceJSON(ctx, client, state, resp, "https://auth.openai.com/about-you", "about_you", data)
}

func protocolAuthAdvanceJSON(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, resp *codexOAuthProtocolHTTPResponse, referer, sourceStage string, data map[string]any) (string, error) {
	if err := codexOAuthProtocolRequireOK(resp, sourceStage); err != nil {
		return "", err
	}
	payload := codexOAuthProtocolResponseJSON(resp)
	stage, continueURL := codexOAuthProtocolStageFromJSON(payload)
	if stage != "" {
		state.Stage = stage
		state.LastPageType = codexOAuthProtocolPageType(payload)
		data["login_stage"] = stage
	}
	if continueURL != "" {
		state.LastContinueURL = continueURL
		return runCodexOAuthProtocolURL(ctx, client, state, continueURL, referer, data)
	}
	return state.Stage, nil
}

func protocolAuthAdvanceStage(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, data map[string]any) (string, error) {
	if state.Stage == "" && state.LastURL != "" {
		state.Stage = codexOAuthProtocolStageFromURL(state.LastURL, "")
	}
	if state.Stage == "" && state.LastContinueURL != "" {
		return runCodexOAuthProtocolURL(ctx, client, state, state.LastContinueURL, codexOAuthProtocolRefererForStage(state.Stage), data)
	}
	return state.Stage, nil
}

func (s *Server) protocolAuthCaptureSession(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, data map[string]any) (RegisterActivityOutput, error) {
	if _, err := protocolAuthCallbackURL(ctx, client, state, data); err != nil {
		if session, sessionErr := protocolAuthSession(ctx, client, state, data); sessionErr == nil {
			return session, nil
		}
		return RegisterActivityOutput{Data: protoData(data)}, err
	}
	if err := protocolAuthConsumeCallback(ctx, client, state); err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, err
	}
	return protocolAuthSession(ctx, client, state, data)
}

func protocolAuthCallbackURL(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, data map[string]any) (string, error) {
	if codexOAuthProtocolIsCallbackURL(state.LastURL, state.RedirectURI) {
		data["callback_url_captured"] = true
		return state.LastURL, nil
	}
	for _, candidate := range []string{state.LastContinueURL, state.AuthorizeURL} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, err := runCodexOAuthProtocolURL(ctx, client, state, candidate, codexOAuthProtocolRefererForStage(state.Stage), data); err != nil {
			return "", err
		}
		if codexOAuthProtocolIsCallbackURL(state.LastURL, state.RedirectURI) {
			data["callback_url_captured"] = true
			return state.LastURL, nil
		}
	}
	return "", fmt.Errorf("protocol auth callback stage not ready: %s", state.Stage)
}

func protocolAuthConsumeCallback(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState) error {
	currentURL := state.LastURL
	lastReferer := "https://auth.openai.com/"
	for hop := 0; hop < 6 && strings.TrimSpace(currentURL) != ""; hop++ {
		resp, err := client.get(ctx, currentURL, lastReferer, true)
		if err != nil {
			return err
		}
		if location := codexOAuthProtocolRedirectLocation(resp, currentURL); location != "" {
			lastReferer = currentURL
			currentURL = location
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 399 {
			return fmt.Errorf("protocol auth callback consume failed: status %d %s", resp.StatusCode, codexOAuthProtocolSafeText(string(resp.Body), 240))
		}
		return nil
	}
	return nil
}

func protocolAuthSession(ctx context.Context, client *codexOAuthProtocolHTTPClient, state *codexOAuthProtocolState, data map[string]any) (RegisterActivityOutput, error) {
	resp, err := client.get(ctx, protocolAuthChatGPTSessionURL, "https://chatgpt.com/", false)
	if err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, err
	}
	if err := codexOAuthProtocolRequireOK(resp, "chatgpt auth session"); err != nil {
		return RegisterActivityOutput{Data: protoData(data)}, err
	}
	sessionToken := client.cookieValue("__Secure-next-auth.session-token", "chatgpt.com")
	accessToken := strings.TrimSpace(stringAny(codexOAuthProtocolResponseJSON(resp)["accessToken"]))
	data["session_token_present"] = sessionToken != ""
	data["access_token_present"] = accessToken != ""
	if sessionToken == "" || accessToken == "" {
		return RegisterActivityOutput{Data: protoData(data)}, fmt.Errorf("protocol auth session missing session_token or access_token")
	}
	return RegisterActivityOutput{
		SessionToken: sessionToken,
		AccessToken:  accessToken,
		DeviceId:     state.DeviceID,
		Data:         protoData(data),
	}, nil
}

func (s *Server) protocolAuthStateClient(ctx context.Context, jobID, flowID string) (*codexOAuthProtocolState, *codexOAuthProtocolHTTPClient, error) {
	state, err := s.loadCodexOAuthProtocolState(ctx, jobID, &CodexOAuthBrowserSession{FlowId: flowID})
	if err != nil {
		return nil, nil, err
	}
	client, err := newCodexOAuthProtocolHTTPClient(s.codexOAuthConfig.withDefaults(), state)
	if err != nil {
		return nil, nil, err
	}
	return state, client, nil
}

func (s *Server) protocolAuthAccount(ctx context.Context, accountID, mode string) (*pb.Account, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if mode == browserAuthModeRegister {
		if err := rejectUserAlreadyExistsAccount(account); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(account.GetEmail()) == "" {
		return nil, fmt.Errorf("email is required")
	}
	if strings.TrimSpace(account.GetPassword()) == "" {
		return nil, fmt.Errorf("password is required")
	}
	return account, nil
}

func protocolAuthData(mode string, account *pb.Account) map[string]any {
	data := map[string]any{"driver": "protocol", "mode": mode}
	if account != nil {
		data["account_id"] = account.GetAccountId()
		data["email"] = account.GetEmail()
	}
	return data
}

func protocolAuthReadyStage(stage string) bool {
	return stage == "callback" || stage == "consent"
}

func protocolAuthStartStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountProtocolStart, nil
	case browserAuthModeLogin:
		return stepLoginSessionProtocolStart, nil
	default:
		return "", fmt.Errorf("unsupported protocol auth mode: %s", mode)
	}
}

func protocolAuthWaitStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountProtocol, nil
	case browserAuthModeLogin:
		return stepLoginSessionProtocol, nil
	default:
		return "", fmt.Errorf("unsupported protocol auth mode: %s", mode)
	}
}

func protocolAuthCompleteStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountProtocolComplete, nil
	case browserAuthModeLogin:
		return stepLoginSessionProtocolComplete, nil
	default:
		return "", fmt.Errorf("unsupported protocol auth mode: %s", mode)
	}
}

func protocolAuthProfile(account *pb.Account) (string, string) {
	name := protocolAuthDisplayName(account)
	if name == "" {
		name = browserAuthDefaultRegistrationName
	}
	return name, "2000-01-01"
}

func protocolAuthDisplayName(account *pb.Account) string {
	if account == nil {
		return ""
	}
	name := strings.TrimSpace(strings.Join([]string{account.GetFirstName(), account.GetLastName()}, " "))
	var out strings.Builder
	lastSpace := false
	for _, r := range name {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			out.WriteRune(r)
			lastSpace = false
			continue
		}
		if r == ' ' && !lastSpace && out.Len() > 0 {
			out.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(out.String())
}
