package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Server) saveRuntimeSecretValue(ctx context.Context, key string, value string) error {
	key = strings.TrimSpace(key)
	if key == "" || value == "" {
		return nil
	}
	if s == nil || s.runtimeSecrets == nil {
		return fmt.Errorf("runtime secret store is not configured")
	}
	return s.runtimeSecrets.Save(ctx, key, value)
}

func (s *Server) saveRuntimeSecretValueTTL(ctx context.Context, key string, value string, ttl time.Duration) error {
	key = strings.TrimSpace(key)
	if key == "" || value == "" {
		return nil
	}
	if s == nil || s.runtimeSecrets == nil {
		return fmt.Errorf("runtime secret store is not configured")
	}
	return s.runtimeSecrets.SaveTTL(ctx, key, value, ttl)
}

func (s *Server) runtimeSecretValue(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	if s == nil || s.runtimeSecrets == nil {
		return "", fmt.Errorf("runtime secret store is not configured")
	}
	value, found, err := s.runtimeSecrets.Load(ctx, key)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("runtime secret not found: %s", key)
	}
	return value, nil
}

func (s *Server) deleteRuntimeSecretValue(ctx context.Context, key string) {
	key = strings.TrimSpace(key)
	if key == "" || s == nil || s.runtimeSecrets == nil {
		return
	}
	_ = s.runtimeSecrets.Delete(ctx, key)
}
