package appsvc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/gpt/gopay/pb"
	"github.com/byte-v-forge/gpt/gopay/protocol"
	gopayapp "github.com/byte-v-forge/gpt/gopay/protocol/app"
)

type Server struct {
	pb.UnimplementedGopayAppServiceServer
	cfg   Config
	store *StateStore
}

func NewServer(cfg Config) (*Server, error) {
	store, err := NewStateStore(cfg.StateDSN, cfg.StateTable)
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, store: store}, nil
}

func (s *Server) GetGoPayState(ctx context.Context, req *pb.GetGoPayStateRequest) (*pb.GetGoPayStateResponse, error) {
	key, err := NormalizeStateKey(req.GetUserId())
	if err != nil {
		return &pb.GetGoPayStateResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	raw, err := s.store.Load(ctx, key)
	if err != nil {
		return &pb.GetGoPayStateResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	return &pb.GetGoPayStateResponse{Success: true, UserId: key, StateJson: raw}, nil
}

func (s *Server) UpsertGoPayState(ctx context.Context, req *pb.UpsertGoPayStateRequest) (*pb.UpsertGoPayStateResponse, error) {
	key, err := NormalizeStateKey(req.GetUserId())
	if err != nil {
		return &pb.UpsertGoPayStateResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	raw, err := s.store.Save(ctx, key, firstNonEmpty(req.GetStateJson(), "{}"))
	if err != nil {
		return &pb.UpsertGoPayStateResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	return &pb.UpsertGoPayStateResponse{Success: true, UserId: key, StateJson: raw}, nil
}

func (s *Server) DeleteGoPayState(ctx context.Context, req *pb.DeleteGoPayStateRequest) (*pb.DeleteGoPayStateResponse, error) {
	key, err := NormalizeStateKey(req.GetUserId())
	if err != nil {
		return &pb.DeleteGoPayStateResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	if err := s.store.Delete(ctx, key); err != nil {
		return &pb.DeleteGoPayStateResponse{Success: false, ErrorMessage: err.Error()}, nil
	}
	return &pb.DeleteGoPayStateResponse{Success: true}, nil
}

func (s *Server) parseRequestState(raw string) stateMap {
	state, err := parseState(raw)
	if err != nil {
		return stateMap{"last_error": err.Error()}
	}
	return state
}

func (s *Server) authBody(extra map[string]any) map[string]any {
	body := map[string]any{}
	for key, value := range extra {
		body[key] = value
	}
	body["client_id"] = s.cfg.GotoClientID
	body["client_secret"] = s.cfg.GotoClientSecret
	return body
}

func (s *Server) pin(value string) string {
	return strings.TrimSpace(value)
}

func (s *Server) signupProfile(phone, name, email string) (string, string) {
	resolvedName := strings.TrimSpace(name)
	resolvedEmail := strings.TrimSpace(email)
	if resolvedName != "" {
		return resolvedName, resolvedEmail
	}
	return signupNameFromSeed(signupSeed(phone)), resolvedEmail
}

func (s *Server) signupBasicAuthorization() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(s.cfg.SignupAuthUUID))
}

func (s *Server) newClient(ctx context.Context, token string, proxyURL string, device gopayapp.DeviceFingerprint) (*gopayapp.Client, error) {
	cfg := gopayapp.ConfigFromEnv(token)
	cfg.ProxyURL = proxyURL
	cfg.Timeout = 30 * time.Second
	cfg.Device = device
	cfg.Logger = func(ctx context.Context, message string, fields map[string]any) {
		fmt.Printf("[gopay-app] %s %v\n", message, fields)
	}
	return gopayapp.NewClient(cfg)
}

func (s *Server) clientForState(ctx context.Context, state stateMap) (*gopayapp.Client, error) {
	refresh := s.ensureAccessToken(ctx, state, s.cfg.TokenRefreshMinTTL, false)
	if !anyBool(refresh["success"]) && !tokenUsable(state, "token", 0) {
		return nil, fmt.Errorf(firstNonEmpty(anyString(refresh["error"]), "token refresh failed"))
	}
	device, err := s.ensureDevice(state)
	if err != nil {
		return nil, err
	}
	return s.newClient(ctx, stateString(state, "token"), s.proxyForState(state), device)
}

func (s *Server) tmpClientForState(ctx context.Context, state stateMap) (*gopayapp.Client, error) {
	token := stateString(state, "_tmp_token")
	if token == "" {
		return nil, fmt.Errorf("temporary account token missing")
	}
	if !tmpTokenUsable(state, 0) {
		expiresAt := firstNonZero(jwtExpiresAt(token), stateInt(state, "_tmp_token_expires_at"))
		return nil, fmt.Errorf("temporary account token expired: expires_at=%d", expiresAt)
	}
	device, err := s.ensureDevice(state)
	if err != nil {
		return nil, err
	}
	return s.newClient(ctx, token, s.proxyForState(state), device)
}

func (s *Server) proxyForAttempt(attempt int, state stateMap) (string, int, int, error) {
	if len(s.cfg.ProxyPool) == 0 {
		return "", 0, 0, fmt.Errorf("GoPay proxy_urls config is required")
	}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(s.cfg.ProxyPool) {
		return "", 0, len(s.cfg.ProxyPool), fmt.Errorf("GOPAY_PROXY_POOL exhausted before login methods succeeded")
	}
	current := s.proxyIndex(stateString(state, "_gopay_proxy"))
	index := 0
	if current >= 0 && attempt <= 1 {
		index = current
	} else if current >= 0 {
		index = (current + 1) % len(s.cfg.ProxyPool)
	}
	proxyURL := s.cfg.ProxyPool[index]
	if state != nil {
		state["_gopay_proxy"] = proxyURL
	}
	return proxyURL, index + 1, len(s.cfg.ProxyPool), nil
}

func (s *Server) proxyForState(state stateMap) string {
	if len(s.cfg.ProxyPool) == 0 {
		return ""
	}
	index := s.proxyIndex(stateString(state, "_gopay_proxy"))
	if index < 0 {
		proxyURL, _, _, _ := s.proxyForAttempt(1, state)
		return proxyURL
	}
	return s.cfg.ProxyPool[index]
}

func (s *Server) proxyAttemptLimit() int {
	if len(s.cfg.ProxyPool) > 0 {
		return len(s.cfg.ProxyPool)
	}
	return 1
}

func (s *Server) proxyIndex(value string) int {
	value = strings.TrimSpace(value)
	for index, item := range s.cfg.ProxyPool {
		if strings.TrimSpace(item) == value {
			return index
		}
	}
	return -1
}

func (s *Server) ensureDevice(state stateMap) (gopayapp.DeviceFingerprint, error) {
	raw := nestedMap(state["device"])
	if len(raw) > 0 {
		device := deviceFromMap(raw)
		if device.AppID == "" || device.UniqueID == "" {
			next, err := gopayapp.NewDeviceFingerprint(gopayapp.DeviceConfigFromEnv())
			if err != nil {
				return gopayapp.DeviceFingerprint{}, err
			}
			device = mergeDevice(device, next)
		}
		state["device"] = deviceToMap(device)
		return device, nil
	}
	device, err := gopayapp.NewDeviceFingerprint(gopayapp.DeviceConfigFromEnv())
	if err != nil {
		return gopayapp.DeviceFingerprint{}, err
	}
	rawID := make([]byte, 8)
	_, _ = rand.Read(rawID)
	out := deviceToMap(device)
	out["profile_id"] = hex.EncodeToString(rawID)
	out["profile_created_at"] = time.Now().Unix()
	state["device"] = out
	return device, nil
}

func (s *Server) newLogonDevice() (gopayapp.DeviceFingerprint, map[string]any, error) {
	device, err := gopayapp.NewDeviceFingerprint(gopayapp.DeviceConfigFromEnv())
	if err != nil {
		return gopayapp.DeviceFingerprint{}, nil, err
	}
	out := deviceToMap(device)
	rawID := make([]byte, 8)
	_, _ = rand.Read(rawID)
	out["profile_id"] = hex.EncodeToString(rawID)
	out["profile_created_at"] = time.Now().Unix()
	return device, out, nil
}

func apiError(label string, resp *protocol.Response) string {
	if resp == nil {
		return label + ": no response"
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "AUTH_INVALID"
	}
	return fmt.Sprintf("%s: status %d %s", label, resp.StatusCode, compactErrorDetail(resp.Payload))
}

func responseErrors(resp *protocol.Response) []any {
	if resp == nil {
		return nil
	}
	for _, source := range []any{resp.Payload["errors"], resp.Data()["errors"]} {
		if items, ok := source.([]any); ok {
			return items
		}
	}
	return nil
}

func responseText(resp *protocol.Response) string {
	if resp == nil {
		return ""
	}
	return string(resp.Body)
}

func isRateLimited(resp *protocol.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	for _, err := range responseErrors(resp) {
		text := strings.ToLower(compactErrorDetail(err))
		if strings.Contains(text, "ratelimited") {
			return true
		}
	}
	return false
}

func loginMethodsInvalidUser(resp *protocol.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	for _, err := range responseErrors(resp) {
		text := strings.ToLower(compactErrorDetail(err))
		if strings.Contains(text, "invalid user") || strings.Contains(text, "could not find the user") {
			return true
		}
	}
	return false
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
