package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

const codexOAuthProtocolStateSecretPrefix = "codex_oauth_protocol_state:"

type codexOAuthProtocolState struct {
	FlowID                   string                     `json:"flow_id"`
	OAuthState               string                     `json:"oauth_state"`
	PKCESecretKey            string                     `json:"pkce_secret_key"`
	ClientID                 string                     `json:"client_id"`
	RedirectURI              string                     `json:"redirect_uri"`
	AuthorizeURL             string                     `json:"authorize_url"`
	DeviceID                 string                     `json:"device_id,omitempty"`
	LastURL                  string                     `json:"last_url,omitempty"`
	LastContinueURL          string                     `json:"last_continue_url,omitempty"`
	Stage                    string                     `json:"stage,omitempty"`
	LastPageType             string                     `json:"last_page_type,omitempty"`
	EmailOTPIssuedAfterUnix  int64                      `json:"email_otp_issued_after_unix,omitempty"`
	PhoneStateKnown          bool                       `json:"phone_state_known,omitempty"`
	PhonePresent             bool                       `json:"phone_present,omitempty"`
	PhoneVerificationChannel string                     `json:"phone_verification_channel,omitempty"`
	PhoneMask                string                     `json:"phone_mask,omitempty"`
	Cookies                  []codexOAuthProtocolCookie `json:"cookies,omitempty"`
}

type codexOAuthProtocolCookie struct {
	HostKey     string `json:"host_key"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Path        string `json:"path,omitempty"`
	Domain      string `json:"domain,omitempty"`
	ExpiresUnix int64  `json:"expires_unix,omitempty"`
	MaxAge      int    `json:"max_age,omitempty"`
	Secure      bool   `json:"secure,omitempty"`
	HTTPOnly    bool   `json:"http_only,omitempty"`
	SameSite    int    `json:"same_site,omitempty"`
}

func newCodexOAuthProtocolState(jobID string, cfg CodexOAuthConfig, pkce codexOAuthPKCE) (codexOAuthProtocolState, error) {
	flowID, err := randomURLToken(18)
	if err != nil {
		return codexOAuthProtocolState{}, err
	}
	oauthState, err := randomURLToken(32)
	if err != nil {
		return codexOAuthProtocolState{}, err
	}
	cfg = cfg.withDefaults()
	state := codexOAuthProtocolState{
		FlowID:        flowID,
		OAuthState:    oauthState,
		ClientID:      cfg.ClientID,
		RedirectURI:   cfg.RedirectURI,
		PKCESecretKey: codexOAuthPKCESecretKey(jobID, flowID),
	}
	state.AuthorizeURL = buildCodexOAuthAuthorizeURL(cfg, pkce, oauthState)
	return state, nil
}

func codexOAuthProtocolStateSecretKey(jobID, flowID string) string {
	return codexOAuthProtocolStateSecretPrefix + strings.TrimSpace(jobID) + ":" + strings.TrimSpace(flowID)
}

func (s *Server) saveCodexOAuthProtocolState(ctx context.Context, jobID string, state *codexOAuthProtocolState) error {
	if state == nil || strings.TrimSpace(state.FlowID) == "" {
		return fmt.Errorf("codex oauth protocol state missing flow_id")
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.saveRuntimeSecret(ctx, codexOAuthProtocolStateSecretKey(jobID, state.FlowID), string(payload))
}

func (s *Server) loadCodexOAuthProtocolState(ctx context.Context, jobID string, session *CodexOAuthBrowserSession) (*codexOAuthProtocolState, error) {
	if session == nil || strings.TrimSpace(session.GetFlowId()) == "" {
		return nil, fmt.Errorf("codex oauth protocol session is required")
	}
	payload := s.loadRuntimeSecret(ctx, codexOAuthProtocolStateSecretKey(jobID, session.GetFlowId()))
	if strings.TrimSpace(payload) == "" {
		return nil, fmt.Errorf("codex oauth protocol state is missing")
	}
	var state codexOAuthProtocolState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return nil, fmt.Errorf("decode codex oauth protocol state: %w", err)
	}
	if state.FlowID == "" {
		state.FlowID = session.GetFlowId()
	}
	if state.PKCESecretKey == "" {
		state.PKCESecretKey = session.GetPkceSecretKey()
	}
	return &state, nil
}

func (s *Server) deleteCodexOAuthProtocolState(ctx context.Context, jobID string, session *CodexOAuthBrowserSession) error {
	if session == nil || strings.TrimSpace(session.GetFlowId()) == "" {
		return nil
	}
	return s.deleteRuntimeSecret(ctx, codexOAuthProtocolStateSecretKey(jobID, session.GetFlowId()))
}

func codexOAuthProtocolData(label string) map[string]any {
	return map[string]any{
		"label":  label,
		"driver": "protocol",
	}
}

func (state *codexOAuthProtocolState) applyCookieSnapshot(cookies map[string][]*fhttp.Cookie) {
	state.Cookies = nil
	for hostKey, values := range cookies {
		hostKey = strings.TrimSpace(hostKey)
		if hostKey == "" {
			continue
		}
		for _, cookie := range values {
			if cookie == nil || strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
				continue
			}
			entry := codexOAuthProtocolCookie{
				HostKey:  hostKey,
				Name:     cookie.Name,
				Value:    cookie.Value,
				Path:     cookie.Path,
				Domain:   cookie.Domain,
				MaxAge:   cookie.MaxAge,
				Secure:   cookie.Secure,
				HTTPOnly: cookie.HttpOnly,
				SameSite: int(cookie.SameSite),
			}
			if !cookie.Expires.IsZero() {
				entry.ExpiresUnix = cookie.Expires.Unix()
			}
			state.Cookies = append(state.Cookies, entry)
			if strings.EqualFold(cookie.Name, "oai-did") && strings.TrimSpace(state.DeviceID) == "" {
				state.DeviceID = strings.TrimSpace(cookie.Value)
			}
		}
	}
}

func codexOAuthProtocolCookieFromState(cookie codexOAuthProtocolCookie) *fhttp.Cookie {
	out := &fhttp.Cookie{
		Name:     cookie.Name,
		Value:    cookie.Value,
		Path:     cookie.Path,
		Domain:   cookie.Domain,
		MaxAge:   cookie.MaxAge,
		Secure:   cookie.Secure,
		HttpOnly: cookie.HTTPOnly,
		SameSite: fhttp.SameSite(cookie.SameSite),
	}
	if cookie.ExpiresUnix > 0 {
		out.Expires = time.Unix(cookie.ExpiresUnix, 0)
	}
	return out
}
