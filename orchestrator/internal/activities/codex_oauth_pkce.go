package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/fingerprinthttp"
	"github.com/byte-v-forge/common-lib/jwtx"
	"golang.org/x/oauth2"
)

type codexOAuthPKCE struct {
	verifier string
}

type codexOAuthTokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func newCodexOAuthPKCE() (codexOAuthPKCE, error) {
	return codexOAuthPKCE{verifier: oauth2.GenerateVerifier()}, nil
}

func buildCodexOAuthAuthorizeURL(cfg CodexOAuthConfig, pkce codexOAuthPKCE, state string) string {
	cfg = cfg.withDefaults()
	oauthCfg := codexOAuthConfig(cfg)
	return oauthCfg.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(pkce.verifier),
		oauth2.SetAuthURLParam("id_token_add_organizations", "true"),
		oauth2.SetAuthURLParam("codex_cli_simplified_flow", "true"),
		oauth2.SetAuthURLParam("originator", "codex_cli_rs"),
	)
}

func codexOAuthCodeFromCallback(rawURL string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	values := parsed.Query()
	if errText := strings.TrimSpace(values.Get("error")); errText != "" {
		return "", "", fmt.Errorf("codex oauth callback error: %s", errText)
	}
	code := strings.TrimSpace(values.Get("code"))
	state := strings.TrimSpace(values.Get("state"))
	if code == "" {
		return "", "", fmt.Errorf("codex oauth callback code missing")
	}
	return code, state, nil
}

func exchangeCodexOAuthTokenWithProfile(ctx context.Context, cfg CodexOAuthConfig, code, verifier string, profile fingerprinthttp.Profile) (codexOAuthTokenResponse, error) {
	cfg = cfg.withDefaults()
	profile = profile.WithDefaults(codexOAuthProtocolDefaultProfile(cfg))
	profile.ProxyURL = cfg.TokenProxyURL
	client, err := newGptClient(cfg, nil, profile)
	if err != nil {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token client init failed: %w", err)
	}
	defer client.Close()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {cfg.RedirectURI},
		"code_verifier": {strings.TrimSpace(verifier)},
	}
	resp, err := client.postForm(ctx, cfg.TokenURL, "https://auth.openai.com/", form, map[string]string{"Accept": "application/json"})
	if err != nil {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token exchange failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token exchange failed: status %d %s", resp.StatusCode, codexOAuthProtocolSafeText(string(resp.Body), 300))
	}
	tokens := codexOAuthTokenResponse{
		IDToken:      strings.TrimSpace(stringAny(resp.JSON["id_token"])),
		AccessToken:  strings.TrimSpace(stringAny(resp.JSON["access_token"])),
		RefreshToken: strings.TrimSpace(stringAny(resp.JSON["refresh_token"])),
	}
	if tokens.IDToken == "" || tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token response missing required tokens")
	}
	return tokens, nil
}

func codexOAuthConfig(cfg CodexOAuthConfig) oauth2.Config {
	return oauth2.Config{
		ClientID:    cfg.ClientID,
		RedirectURL: cfg.RedirectURI,
		Scopes:      strings.Fields(cfg.Scope),
		Endpoint: oauth2.Endpoint{
			AuthURL:   strings.TrimRight(cfg.AuthURL, "?"),
			TokenURL:  cfg.TokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

func buildCodexAuthJSON(tokens codexOAuthTokenResponse) ([]byte, error) {
	_, accountID := codexOAuthAuthClaims(tokens.IDToken)
	tokenPayload := map[string]any{
		"id_token":      tokens.IDToken,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}
	if accountID != "" {
		tokenPayload["account_id"] = accountID
	}
	auth := map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens":         tokenPayload,
		"last_refresh":   time.Now().UTC().Format(time.RFC3339),
	}
	return json.MarshalIndent(auth, "", "  ")
}

func codexOAuthAuthClaims(idToken string) (string, string) {
	claims := jwtx.PayloadOrNil(idToken)
	if claims == nil {
		return "", ""
	}
	email := claimString(claims["email"])
	accountID := ""
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		accountID = claimString(auth["chatgpt_account_id"])
	}
	if email == "" {
		if profile, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
			email = claimString(profile["email"])
		}
	}
	return email, accountID
}

func claimString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
