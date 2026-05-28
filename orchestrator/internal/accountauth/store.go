package accountauth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/chatgptauth"
	"orchestrator/internal/runtimesecrets"
)

const fallbackCredentialTTL = 24 * time.Hour

func SaveChatGPTSessionToken(ctx context.Context, store runtimesecrets.Store, accountID string, token string) error {
	accountID = strings.TrimSpace(accountID)
	token = strings.TrimSpace(token)
	if accountID == "" || token == "" {
		return nil
	}
	ttl, expiresAt, ok := chatgptauth.SessionTokenTTL(token, time.Now(), defaultTTL(store))
	if !ok {
		return nil
	}
	return saveField(ctx, store, accountID, chatgptauth.FieldChatGPTSessionToken, chatgptauth.FieldChatGPTSessionExpiresAtUnix, token, expiresAt, ttl)
}

func SaveChatGPTAccessToken(ctx context.Context, store runtimesecrets.Store, accountID string, token string) error {
	accountID = strings.TrimSpace(accountID)
	token = strings.TrimSpace(token)
	if accountID == "" || token == "" {
		return nil
	}
	ttl, expiresAt, ok := chatgptauth.AccessTokenTTL(token, time.Now())
	if !ok {
		return nil
	}
	return saveField(ctx, store, accountID, chatgptauth.FieldChatGPTAccessToken, chatgptauth.FieldChatGPTAccessExpiresAtUnix, token, expiresAt, ttl)
}

func SaveCodexAuthJSON(ctx context.Context, store runtimesecrets.Store, accountID string, authJSON string) error {
	accountID = strings.TrimSpace(accountID)
	authJSON = strings.TrimSpace(authJSON)
	if accountID == "" || authJSON == "" {
		return nil
	}
	ttl, expiresAt, ok := chatgptauth.AuthJSONTTL(authJSON, time.Now(), defaultTTL(store))
	if !ok {
		return nil
	}
	return saveField(ctx, store, accountID, chatgptauth.FieldCodexAuthJSON, chatgptauth.FieldCodexAuthExpiresAtUnix, authJSON, expiresAt, ttl)
}

func LoadChatGPTSessionToken(ctx context.Context, store runtimesecrets.Store, accountID string) (string, bool, error) {
	return loadField(ctx, store, accountID, chatgptauth.FieldChatGPTSessionToken, chatgptauth.FieldChatGPTSessionExpiresAtUnix)
}

func LoadChatGPTAccessToken(ctx context.Context, store runtimesecrets.Store, accountID string) (string, bool, error) {
	return loadField(ctx, store, accountID, chatgptauth.FieldChatGPTAccessToken, chatgptauth.FieldChatGPTAccessExpiresAtUnix)
}

func LoadCodexAuthJSON(ctx context.Context, store runtimesecrets.Store, accountID string) (string, bool, error) {
	return loadField(ctx, store, accountID, chatgptauth.FieldCodexAuthJSON, chatgptauth.FieldCodexAuthExpiresAtUnix)
}

func ChatGPTTokens(ctx context.Context, store runtimesecrets.Store, accountID string) (string, string, error) {
	sessionToken, _, err := LoadChatGPTSessionToken(ctx, store, accountID)
	if err != nil {
		return "", "", err
	}
	accessToken, _, err := LoadChatGPTAccessToken(ctx, store, accountID)
	if err != nil {
		return "", "", err
	}
	return sessionToken, accessToken, nil
}

func saveField(ctx context.Context, store runtimesecrets.Store, accountID string, valueField string, expiresField string, value string, expiresAt int64, ttl time.Duration) error {
	if store == nil {
		return fmt.Errorf("runtime secret store is not configured")
	}
	if ttl <= 0 || expiresAt <= 0 {
		return nil
	}
	return store.HashSaveTTL(ctx, chatgptauth.AccountAuthSecretKey(accountID), map[string]string{
		valueField:                     value,
		expiresField:                   strconv.FormatInt(expiresAt, 10),
		chatgptauth.FieldUpdatedAtUnix: strconv.FormatInt(time.Now().Unix(), 10),
	}, ttl)
}

func loadField(ctx context.Context, store runtimesecrets.Store, accountID string, valueField string, expiresField string) (string, bool, error) {
	accountID = strings.TrimSpace(accountID)
	if store == nil || accountID == "" {
		return "", false, nil
	}
	key := chatgptauth.AccountAuthSecretKey(accountID)
	values, err := store.HashLoadMany(ctx, key, valueField, expiresField)
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(values[valueField])
	if value == "" {
		return "", false, nil
	}
	expiresAt, err := strconv.ParseInt(strings.TrimSpace(values[expiresField]), 10, 64)
	if err != nil || expiresAt <= time.Now().Unix() {
		_ = store.HashDelete(ctx, key, valueField, expiresField)
		return "", false, nil
	}
	return value, true, nil
}

func defaultTTL(store runtimesecrets.Store) time.Duration {
	if store == nil || store.DefaultTTL() <= 0 {
		return fallbackCredentialTTL
	}
	return store.DefaultTTL()
}
