package activities

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/byte-v-forge/common-lib/stringx"

	"orchestrator/pb"
)

var codexOAuthProtocolDeviceIDRE = regexp.MustCompile(`"deviceId"\s*:\s*"([^"]+)"`)
var codexOAuthProtocolWorkspaceIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)workspaces".{0,900}?"id","([0-9a-fA-F-]{36})"`),
	regexp.MustCompile(`(?is)"workspace_id"\s*:\s*"([0-9a-fA-F-]{36})"`),
	regexp.MustCompile(`(?is)"workspaceId"\s*:\s*"([0-9a-fA-F-]{36})"`),
}
var codexOAuthProtocolUnifiedSessionRE = regexp.MustCompile(`us_[A-Za-z0-9]{16,}`)

func runCodexOAuthProtocolURL(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, startURL, referer string, data codexOAuthProtocolNavigationData) (string, error) {
	currentURL := codexOAuthProtocolAbsoluteURL(startURL)
	if currentURL == "" {
		return state.Stage, nil
	}
	lastReferer := referer
	workspaceSelected := false
	accountSelected := false
	for hop := 0; hop < 16; hop++ {
		if codexOAuthProtocolIsCallbackURL(currentURL, state.RedirectUri) {
			state.LastUrl = currentURL
			state.Stage = "callback"
			data.setStage(state.Stage)
			return state.Stage, nil
		}
		resp, err := client.get(ctx, currentURL, lastReferer, true)
		if err != nil {
			return "", err
		}
		state.LastUrl = currentURL
		if location := codexOAuthProtocolRedirectLocation(resp, currentURL); location != "" {
			lastReferer = currentURL
			currentURL = location
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return "", fmt.Errorf("codex oauth protocol navigation failed: status %d %s", resp.StatusCode, codexOAuthProtocolSafeText(string(resp.Body), 360))
		}
		body := string(resp.Body)
		if deviceID := codexOAuthProtocolDeviceID(body); deviceID != "" && strings.TrimSpace(state.DeviceId) == "" {
			state.DeviceId = deviceID
		}
		stage := codexOAuthProtocolStageFromURL(currentURL, body)
		if stage != "" {
			state.Stage = stage
			data.setStage(stage)
		}
		if stage == "choose_account" && !accountSelected {
			accountSelected = true
			nextURL, ok := codexOAuthProtocolChooseAccountContinue(ctx, client, state, currentURL, body, data)
			if ok {
				lastReferer = currentURL
				currentURL = nextURL
				continue
			}
		}
		if stage == "consent" && !workspaceSelected {
			workspaceSelected = true
			nextURL, ok := codexOAuthProtocolWorkspaceContinue(ctx, client, state, currentURL, body, data)
			if ok {
				lastReferer = currentURL
				currentURL = nextURL
				continue
			}
		}
		return state.Stage, nil
	}
	return "", fmt.Errorf("codex oauth protocol redirect limit exceeded")
}

func advanceCodexOAuthProtocolJSON(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, resp *codexOAuthProtocolHTTPResponse, sourceStage string, data codexOAuthProtocolNavigationData) (string, error) {
	if err := codexOAuthProtocolRequireOK(resp, sourceStage); err != nil {
		return "", err
	}
	payload := codexOAuthProtocolResponseJSON(resp)
	stage, continueURL := codexOAuthProtocolStageFromJSON(payload)
	if stage != "" {
		state.Stage = stage
		state.LastPageType = codexOAuthProtocolPageType(payload)
		data.setStage(stage)
	}
	if continueURL != "" {
		state.LastContinueUrl = continueURL
		stage, err := runCodexOAuthProtocolURL(ctx, client, state, continueURL, codexOAuthProtocolRefererForStage(sourceStage), data)
		if err != nil {
			return stage, err
		}
		return stage, nil
	}
	return state.Stage, nil
}

func codexOAuthProtocolRedirectLocation(resp *codexOAuthProtocolHTTPResponse, currentURL string) string {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return ""
	}
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" {
		return ""
	}
	parsed, err := url.Parse(location)
	if err == nil && parsed.IsAbs() {
		return location
	}
	base, err := url.Parse(currentURL)
	if err != nil {
		return codexOAuthProtocolAbsoluteURL(location)
	}
	return base.ResolveReference(parsed).String()
}

func codexOAuthProtocolStageFromJSON(payload map[string]any) (string, string) {
	continueURL := codexOAuthProtocolAbsoluteURL(codexOAuthProtocolContinueURL(payload))
	if stage := codexOAuthProtocolStageFromURL(continueURL, ""); stage != "" {
		return stage, continueURL
	}
	pageType := strings.ToLower(codexOAuthProtocolPageType(payload))
	switch pageType {
	case "create_account_password":
		return "create_password", continueURL
	case "login_password":
		return "password", continueURL
	case "email_otp_verification":
		return "email_otp", continueURL
	case "create_account_profile", "create_account", "about_you":
		return "about_you", continueURL
	case "add_phone":
		return "add_phone", continueURL
	case "phone_otp_verification":
		return "phone_otp", continueURL
	case "consent", "oauth_consent":
		return "consent", continueURL
	case "external_url":
		return codexOAuthProtocolStageFromURL(continueURL, ""), continueURL
	default:
		return "", continueURL
	}
}

func codexOAuthProtocolPageType(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	page, _ := payload["page"].(map[string]any)
	return strings.TrimSpace(stringAny(page["type"]))
}

func codexOAuthProtocolContinueURL(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if value := strings.TrimSpace(stringAny(payload["continue_url"])); value != "" {
		return value
	}
	page, _ := payload["page"].(map[string]any)
	pagePayload, _ := page["payload"].(map[string]any)
	return strings.TrimSpace(stringx.FirstNonEmptyAny(pagePayload["url"], pagePayload["continue_url"]))
}

func codexOAuthProtocolStageFromURL(rawURL, body string) string {
	lowerURL := strings.ToLower(rawURL)
	lowerBody := strings.ToLower(body)
	switch {
	case strings.Contains(lowerURL, "/auth/callback"):
		return "callback"
	case strings.Contains(lowerURL, "/phone-verification") || strings.Contains(lowerURL, "/api/accounts/phone-otp/send"):
		return "phone_otp"
	case strings.Contains(lowerURL, "/add-phone"):
		return "add_phone"
	case strings.Contains(lowerURL, "/email-verification"):
		return "email_otp"
	case strings.Contains(lowerURL, "/create-account/password"):
		return "create_password"
	case strings.Contains(lowerURL, "/about-you"):
		return "about_you"
	case strings.Contains(lowerURL, "/log-in/password"):
		return "password"
	case strings.Contains(lowerURL, "/create-account"):
		return "email"
	case strings.Contains(lowerURL, "/log-in"):
		return "email"
	case strings.Contains(lowerURL, "/choose-an-account"):
		return "choose_account"
	case strings.Contains(lowerURL, "/workspace"):
		return "consent"
	case strings.Contains(lowerURL, "/sign-in-with-chatgpt/") || strings.Contains(lowerURL, "/consent"):
		return "consent"
	case strings.Contains(lowerBody, "add your phone") || strings.Contains(lowerBody, "phone number"):
		return "add_phone"
	case strings.Contains(lowerBody, "enter code") && strings.Contains(lowerBody, "phone"):
		return "phone_otp"
	case strings.Contains(lowerBody, "enter your password"):
		return "password"
	case strings.Contains(lowerBody, "create your account") || strings.Contains(lowerBody, "create a password"):
		return "create_password"
	case strings.Contains(lowerBody, "tell us about you") || strings.Contains(lowerBody, "about you"):
		return "about_you"
	case strings.Contains(lowerBody, "email verification") || strings.Contains(lowerBody, "verification code"):
		return "email_otp"
	case strings.Contains(lowerBody, "codex cli") || strings.Contains(lowerBody, "sign in with chatgpt"):
		return "consent"
	default:
		return ""
	}
}

func codexOAuthProtocolDeviceID(body string) string {
	match := codexOAuthProtocolDeviceIDRE.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func codexOAuthProtocolWorkspaceContinue(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, currentURL, body string, data codexOAuthProtocolNavigationData) (string, bool) {
	workspaceID := codexOAuthProtocolWorkspaceIDFromState(state)
	if workspaceID == "" {
		workspaceID = codexOAuthProtocolQueryFirst(currentURL, "workspace_id", "id")
	}
	if workspaceID == "" {
		workspaceID = codexOAuthProtocolWorkspaceIDFromHTML(body)
	}
	if workspaceID == "" {
		return "", false
	}
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/workspace/select", "https://auth.openai.com/sign-in-with-chatgpt/codex/consent", map[string]any{"workspace_id": workspaceID})
	if err != nil {
		data.setWorkspaceSelectError(codexOAuthProtocolSafeText(err.Error(), 180))
		return "", false
	}
	data.setWorkspaceSelectStatus(resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return "", false
	}
	nextURL := codexOAuthProtocolAbsoluteURL(codexOAuthProtocolContinueURL(codexOAuthProtocolResponseJSON(resp)))
	if nextURL == "" {
		nextURL = codexOAuthProtocolRedirectLocation(resp, currentURL)
	}
	if nextURL == "" {
		return "", false
	}
	data.setWorkspaceSelected(true)
	state.LastContinueUrl = nextURL
	return nextURL, true
}

func codexOAuthProtocolChooseAccountContinue(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, currentURL, body string, data codexOAuthProtocolNavigationData) (string, bool) {
	match := codexOAuthProtocolUnifiedSessionRE.FindString(body)
	if match == "" {
		return "", false
	}
	resp, err := client.postJSON(ctx, "https://auth.openai.com/api/accounts/session/select", "https://auth.openai.com/choose-an-account", map[string]any{"session_id": match})
	if err != nil {
		data.setChooseAccountError(codexOAuthProtocolSafeText(err.Error(), 180))
		return "", false
	}
	data.setChooseAccountStatus(resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return "", false
	}
	nextURL := codexOAuthProtocolAbsoluteURL(codexOAuthProtocolContinueURL(codexOAuthProtocolResponseJSON(resp)))
	if nextURL == "" {
		nextURL = codexOAuthProtocolRedirectLocation(resp, currentURL)
	}
	if nextURL == "" {
		nextURL = currentURL
	}
	data.setChooseAccountSelected(true)
	state.LastContinueUrl = nextURL
	return nextURL, true
}

func codexOAuthProtocolWorkspaceIDFromState(state *pb.CodexOAuthProtocolState) string {
	if state == nil {
		return ""
	}
	for _, cookie := range state.Cookies {
		if cookie == nil || !strings.EqualFold(cookie.GetName(), "oai-client-auth-session") || strings.TrimSpace(cookie.GetValue()) == "" {
			continue
		}
		parts := strings.Split(cookie.GetValue(), ".")
		if len(parts) > 2 {
			parts = parts[:2]
		}
		for _, part := range parts {
			if workspaceID := codexOAuthProtocolWorkspaceIDFromBase64JSON(part); workspaceID != "" {
				return workspaceID
			}
		}
	}
	return ""
}

func codexOAuthProtocolWorkspaceIDFromBase64JSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(value + strings.Repeat("=", (4-len(value)%4)%4))
		if err != nil {
			return ""
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ""
	}
	if workspaceID := strings.TrimSpace(stringAny(decoded["workspace_id"])); workspaceID != "" {
		return workspaceID
	}
	workspaces, _ := decoded["workspaces"].([]any)
	for _, item := range workspaces {
		workspace, _ := item.(map[string]any)
		if workspaceID := strings.TrimSpace(stringAny(workspace["id"])); workspaceID != "" {
			return workspaceID
		}
	}
	return ""
}

func codexOAuthProtocolWorkspaceIDFromHTML(body string) string {
	body = strings.ReplaceAll(body, `\"`, `"`)
	for _, pattern := range codexOAuthProtocolWorkspaceIDPatterns {
		match := pattern.FindStringSubmatch(body)
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func codexOAuthProtocolQueryFirst(rawURL string, keys ...string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	values := parsed.Query()
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func codexOAuthProtocolIsCallbackURL(rawURL, redirectURI string) bool {
	if !strings.Contains(rawURL, "/auth/callback") {
		return false
	}
	if strings.TrimSpace(redirectURI) == "" {
		return true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	redirect, err := url.Parse(redirectURI)
	if err != nil {
		return true
	}
	return parsed.Host == redirect.Host && parsed.Path == redirect.Path
}
