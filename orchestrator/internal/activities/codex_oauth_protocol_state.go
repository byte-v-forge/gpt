package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/byte-v-forge/common-lib/randx"
	"google.golang.org/protobuf/encoding/protojson"

	"orchestrator/pb"
)

const codexOAuthProtocolStateSecretPrefix = "codex_oauth_protocol_state:"

func newCodexOAuthProtocolState(jobID string, cfg CodexOAuthConfig, pkce codexOAuthPKCE) (*pb.CodexOAuthProtocolState, error) {
	flowID, err := randx.Base64URL(18)
	if err != nil {
		return nil, err
	}
	oauthState, err := randx.Base64URL(32)
	if err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()
	state := &pb.CodexOAuthProtocolState{
		FlowId:        flowID,
		OauthState:    oauthState,
		ClientId:      cfg.ClientID,
		RedirectUri:   cfg.RedirectURI,
		PkceSecretKey: codexOAuthPKCESecretKey(jobID, flowID),
	}
	state.AuthorizeUrl = buildCodexOAuthAuthorizeURL(cfg, pkce, oauthState)
	return state, nil
}

func codexOAuthProtocolStateSecretKey(jobID, flowID string) string {
	return codexOAuthProtocolStateSecretPrefix + strings.TrimSpace(jobID) + ":" + strings.TrimSpace(flowID)
}

func (s *Server) saveCodexOAuthProtocolState(ctx context.Context, jobID string, state *pb.CodexOAuthProtocolState) error {
	if state == nil || strings.TrimSpace(state.GetFlowId()) == "" {
		return fmt.Errorf("codex oauth protocol state missing flow_id")
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(state)
	if err != nil {
		return err
	}
	return s.saveRuntimeSecret(ctx, codexOAuthProtocolStateSecretKey(jobID, state.GetFlowId()), string(payload))
}

func (s *Server) loadCodexOAuthProtocolState(ctx context.Context, jobID string, session *CodexOAuthBrowserSession) (*pb.CodexOAuthProtocolState, error) {
	if session == nil || strings.TrimSpace(session.GetFlowId()) == "" {
		return nil, fmt.Errorf("codex oauth protocol session is required")
	}
	payload := s.loadRuntimeSecret(ctx, codexOAuthProtocolStateSecretKey(jobID, session.GetFlowId()))
	if strings.TrimSpace(payload) == "" {
		return nil, fmt.Errorf("codex oauth protocol state is missing")
	}
	state := &pb.CodexOAuthProtocolState{}
	if err := protojson.Unmarshal([]byte(payload), state); err != nil {
		return nil, fmt.Errorf("decode codex oauth protocol state: %w", err)
	}
	if state.GetFlowId() == "" {
		state.FlowId = session.GetFlowId()
	}
	if state.GetPkceSecretKey() == "" {
		state.PkceSecretKey = session.GetPkceSecretKey()
	}
	return state, nil
}

func (s *Server) deleteCodexOAuthProtocolState(ctx context.Context, jobID string, session *CodexOAuthBrowserSession) error {
	if session == nil || strings.TrimSpace(session.GetFlowId()) == "" {
		return nil
	}
	return s.deleteRuntimeSecret(ctx, codexOAuthProtocolStateSecretKey(jobID, session.GetFlowId()))
}

func codexOAuthProtocolData(label string) *codexOAuthStepData {
	data := newCodexOAuthStepData(label, nil)
	data.setDriver("protocol")
	return data
}

func applyCodexOAuthProtocolCookieSnapshot(state *pb.CodexOAuthProtocolState, cookies map[string][]*fhttp.Cookie) {
	if state == nil {
		return
	}
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
			entry := &pb.CodexOAuthProtocolCookie{
				HostKey:  hostKey,
				Name:     cookie.Name,
				Value:    cookie.Value,
				Path:     cookie.Path,
				Domain:   cookie.Domain,
				MaxAge:   int32(cookie.MaxAge),
				Secure:   cookie.Secure,
				HttpOnly: cookie.HttpOnly,
				SameSite: int32(cookie.SameSite),
			}
			if !cookie.Expires.IsZero() {
				entry.ExpiresUnix = cookie.Expires.Unix()
			}
			state.Cookies = append(state.Cookies, entry)
			if strings.EqualFold(cookie.Name, "oai-did") && strings.TrimSpace(state.GetDeviceId()) == "" {
				state.DeviceId = strings.TrimSpace(cookie.Value)
			}
		}
	}
}

func codexOAuthProtocolCookieFromState(cookie *pb.CodexOAuthProtocolCookie) *fhttp.Cookie {
	if cookie == nil {
		return nil
	}
	out := &fhttp.Cookie{
		Name:     cookie.GetName(),
		Value:    cookie.GetValue(),
		Path:     cookie.GetPath(),
		Domain:   cookie.GetDomain(),
		MaxAge:   int(cookie.GetMaxAge()),
		Secure:   cookie.GetSecure(),
		HttpOnly: cookie.GetHttpOnly(),
		SameSite: fhttp.SameSite(cookie.GetSameSite()),
	}
	if cookie.GetExpiresUnix() > 0 {
		out.Expires = time.Unix(cookie.GetExpiresUnix(), 0)
	}
	return out
}
