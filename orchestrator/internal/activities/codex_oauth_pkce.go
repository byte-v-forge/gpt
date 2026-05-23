package activities

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type codexOAuthPKCE struct {
	verifier  string
	challenge string
}

type codexOAuthTokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func newCodexOAuthPKCE() (codexOAuthPKCE, error) {
	verifier, err := randomURLToken(64)
	if err != nil {
		return codexOAuthPKCE{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	return codexOAuthPKCE{
		verifier:  verifier,
		challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func buildCodexOAuthAuthorizeURL(cfg CodexOAuthConfig, pkce codexOAuthPKCE, state string) string {
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", cfg.ClientID)
	values.Set("redirect_uri", cfg.RedirectURI)
	values.Set("scope", cfg.Scope)
	values.Set("code_challenge", pkce.challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("id_token_add_organizations", "true")
	values.Set("codex_cli_simplified_flow", "true")
	values.Set("state", state)
	values.Set("originator", "codex_cli_rs")
	return strings.TrimRight(cfg.AuthURL, "?") + "?" + values.Encode()
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

func exchangeCodexOAuthToken(ctx context.Context, cfg CodexOAuthConfig, code, verifier string) (codexOAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", cfg.RedirectURI)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return codexOAuthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return codexOAuthTokenResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return codexOAuthTokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token exchange failed: status %d %s", resp.StatusCode, compactBrowserAuthText(string(body), 500))
	}
	var tokens codexOAuthTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return codexOAuthTokenResponse{}, err
	}
	if tokens.IDToken == "" || tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token response missing required tokens")
	}
	return tokens, nil
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
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if padded, padErr := base64.URLEncoding.DecodeString(parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)); padErr == nil {
			payload = padded
		} else {
			return "", ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
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
