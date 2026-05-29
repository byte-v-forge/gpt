package activities

import (
	"context"
	"orchestrator/internal/accountauth"
	"strings"
	"time"
)

func (s *Server) loadRuntimeSecret(ctx context.Context, key string) string {
	if s == nil || s.runtimeSecrets == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	value, found, err := s.runtimeSecrets.Load(ctx, key)
	if err != nil || !found {
		return ""
	}
	return strings.TrimSpace(value)
}

func (s *Server) saveRuntimeSecret(ctx context.Context, key string, value string) error {
	if s == nil || s.runtimeSecrets == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	return s.runtimeSecrets.Save(ctx, key, value)
}

func (s *Server) saveRuntimeSecretTTL(ctx context.Context, key string, value string, ttl time.Duration) error {
	if s == nil || s.runtimeSecrets == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	return s.runtimeSecrets.SaveTTL(ctx, key, value, ttl)
}

func (s *Server) saveChatGPTAccessToken(ctx context.Context, accountID string, accessToken string) error {
	return accountauth.SaveChatGPTAccessToken(ctx, s.runtimeSecrets, accountID, accessToken)
}

func (s *Server) saveChatGPTSessionToken(ctx context.Context, accountID string, sessionToken string) error {
	return accountauth.SaveChatGPTSessionToken(ctx, s.runtimeSecrets, accountID, sessionToken)
}

func (s *Server) saveCodexAuthJSON(ctx context.Context, accountID string, authJSON string) error {
	return accountauth.SaveCodexAuthJSON(ctx, s.runtimeSecrets, accountID, authJSON)
}

func (s *Server) cachedChatGPTSessionToken(ctx context.Context, accountID string) string {
	value, _, _ := accountauth.LoadChatGPTSessionToken(ctx, s.runtimeSecrets, accountID)
	return strings.TrimSpace(value)
}

func (s *Server) cachedChatGPTAccessToken(ctx context.Context, accountID string) string {
	value, _, _ := accountauth.LoadChatGPTAccessToken(ctx, s.runtimeSecrets, accountID)
	return strings.TrimSpace(value)
}

func (s *Server) deleteRuntimeSecret(ctx context.Context, key string) error {
	if s == nil || s.runtimeSecrets == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	return s.runtimeSecrets.Delete(ctx, key)
}
