package chatgptauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	accountAuthSecretPrefix         = "account_auth:"
	nextAuthSessionCookieName       = "__Secure-next-auth.session-token"
	nextAuthSessionCookieLegacyName = "next-auth.session-token"
	nextAuthSessionCookieChunkSize  = 4096 - 163
	accessTokenExpirySkew           = 60 * time.Second
	accessTokenFallbackTTL          = 15 * time.Minute
)

const (
	FieldChatGPTSessionToken         = "chatgpt.session_token"
	FieldChatGPTSessionExpiresAtUnix = "chatgpt.session_expires_at_unix"
	FieldChatGPTAccessToken          = "chatgpt.access_token"
	FieldChatGPTAccessExpiresAtUnix  = "chatgpt.access_expires_at_unix"
	FieldCodexAuthJSON               = "codex.auth_json"
	FieldCodexAuthExpiresAtUnix      = "codex.auth_expires_at_unix"
	FieldUpdatedAtUnix               = "updated_at_unix"
)

func AccountAuthSecretKey(accountID string) string {
	return accountAuthSecretPrefix + strings.TrimSpace(accountID)
}

func AccessTokenTTL(token string, now time.Time) (time.Duration, int64, bool) {
	return tokenTTL(token, now, accessTokenFallbackTTL)
}

func SessionTokenTTL(token string, now time.Time, fallback time.Duration) (time.Duration, int64, bool) {
	return tokenTTL(token, now, fallback)
}

func AuthJSONTTL(raw string, now time.Time, fallback time.Duration) (time.Duration, int64, bool) {
	if expiresAt, ok := authJSONExpiresAtUnix(raw); ok {
		return ttlFromExpiresAt(expiresAt, now, 0)
	}
	return fallbackTTL(fallback, now)
}

func tokenTTL(token string, now time.Time, fallback time.Duration) (time.Duration, int64, bool) {
	expiresAt, ok := JWTExpiresAtUnix(token)
	if !ok {
		return fallbackTTL(fallback, now)
	}
	return ttlFromExpiresAt(expiresAt, now, accessTokenExpirySkew)
}

func AccessTokenExpiresAtUnix(token string) (int64, bool) {
	return JWTExpiresAtUnix(token)
}

func JWTExpiresAtUnix(token string) (int64, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil {
		return 0, false
	}
	exp, err := claims.Exp.Int64()
	if err != nil || exp <= 0 {
		return 0, false
	}
	return exp, true
}

func ttlFromExpiresAt(expiresAt int64, now time.Time, skew time.Duration) (time.Duration, int64, bool) {
	if now.IsZero() {
		now = time.Now()
	}
	ttl := time.Unix(expiresAt, 0).Sub(now) - skew
	if ttl <= 0 {
		return 0, expiresAt, false
	}
	return ttl, expiresAt, true
}

func fallbackTTL(fallback time.Duration, now time.Time) (time.Duration, int64, bool) {
	if fallback <= 0 {
		return 0, 0, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	return fallback, now.Add(fallback).Unix(), true
}

func NormalizeAccountAuthInput(sessionInput, accessInput string) (string, string) {
	sessionToken := strings.TrimSpace(sessionInput)
	accessToken := ExtractAccessToken(accessInput)
	if payloadSession, payloadAccess := authSessionJSONTokens(sessionToken); payloadSession != "" || payloadAccess != "" {
		if payloadSession != "" {
			sessionToken = payloadSession
		}
		if accessToken == "" {
			accessToken = payloadAccess
		}
	}
	if payloadSession, payloadAccess := authSessionJSONTokens(accessInput); payloadSession != "" || payloadAccess != "" {
		if sessionToken == "" {
			sessionToken = payloadSession
		}
		if payloadAccess != "" {
			accessToken = payloadAccess
		}
	}
	if parsedSession := ExtractSessionToken(sessionToken); parsedSession != "" {
		sessionToken = parsedSession
	}
	return strings.TrimSpace(sessionToken), strings.TrimSpace(accessToken)
}

func ExtractAccessToken(raw string) string {
	text := strings.TrimSpace(raw)
	if _, accessToken := authSessionJSONTokens(text); accessToken != "" {
		return accessToken
	}
	return text
}

func ExtractSessionToken(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if sessionToken, _ := authSessionJSONTokens(text); sessionToken != "" {
		return sessionToken
	}
	exact := ""
	chunks := map[int]string{}
	for _, part := range strings.Split(text, ";") {
		name, value, ok := parseSessionCookiePart(part)
		if !ok {
			continue
		}
		if name == nextAuthSessionCookieName || name == nextAuthSessionCookieLegacyName {
			exact = value
			continue
		}
		if index, ok := sessionCookieChunkIndex(name); ok {
			chunks[index] = value
		}
	}
	if exact != "" {
		return exact
	}
	if len(chunks) == 0 {
		return ""
	}
	indexes := make([]int, 0, len(chunks))
	for index := range chunks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	var b strings.Builder
	for _, index := range indexes {
		b.WriteString(chunks[index])
	}
	return b.String()
}

func SessionCookieHeader(sessionToken string) string {
	token := ExtractSessionToken(sessionToken)
	if token == "" {
		token = strings.TrimSpace(sessionToken)
	}
	if token == "" {
		return ""
	}
	if strings.Contains(token, "=") {
		parts := make([]string, 0, 2)
		for _, part := range strings.Split(token, ";") {
			name, value, ok := parseSessionCookiePart(part)
			if ok {
				parts = append(parts, name+"="+value)
			}
		}
		if len(parts) > 0 {
			sort.SliceStable(parts, func(i, j int) bool { return sessionCookieSortKey(parts[i]) < sessionCookieSortKey(parts[j]) })
			return strings.Join(parts, "; ")
		}
	}
	if len(token) <= nextAuthSessionCookieChunkSize {
		return nextAuthSessionCookieName + "=" + token
	}
	parts := make([]string, 0, (len(token)+nextAuthSessionCookieChunkSize-1)/nextAuthSessionCookieChunkSize)
	for index, offset := 0, 0; offset < len(token); index, offset = index+1, offset+nextAuthSessionCookieChunkSize {
		end := offset + nextAuthSessionCookieChunkSize
		if end > len(token) {
			end = len(token)
		}
		parts = append(parts, fmt.Sprintf("%s.%d=%s", nextAuthSessionCookieName, index, token[offset:end]))
	}
	return strings.Join(parts, "; ")
}

func authSessionJSONTokens(raw string) (string, string) {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "{") {
		return "", ""
	}
	var payload struct {
		SessionToken string `json:"sessionToken"`
		AccessToken  string `json:"accessToken"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return "", ""
	}
	return strings.TrimSpace(payload.SessionToken), strings.TrimSpace(payload.AccessToken)
}

func authJSONExpiresAtUnix(raw string) (int64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" || !strings.HasPrefix(text, "{") {
		return 0, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return 0, false
	}
	if expiresAt, ok := numericExpiresAt(payload); ok {
		return expiresAt, true
	}
	if tokens, ok := payload["tokens"].(map[string]any); ok {
		if expiresAt, ok := numericExpiresAt(tokens); ok {
			return expiresAt, true
		}
		for _, key := range []string{"access_token", "id_token"} {
			if expiresAt, ok := JWTExpiresAtUnix(claimString(tokens[key])); ok {
				return expiresAt, true
			}
		}
	}
	return 0, false
}

func numericExpiresAt(payload map[string]any) (int64, bool) {
	for _, key := range []string{"expires_at_unix", "expiresAtUnix", "expires_at", "expiresAt", "auth_expires_at_unix", "authExpiresAtUnix"} {
		expiresAt, ok := numericUnix(payload[key])
		if ok {
			return expiresAt, true
		}
	}
	return 0, false
}

func numericUnix(value any) (int64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case json.Number:
		v, err := typed.Int64()
		return v, err == nil && v > 0
	case float64:
		v := int64(typed)
		return v, v > 0
	case int64:
		return typed, typed > 0
	case int:
		return int64(typed), typed > 0
	case string:
		v, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return v, err == nil && v > 0
	default:
		return 0, false
	}
}

func claimString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func parseSessionCookiePart(part string) (string, string, bool) {
	name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !ok || name == "" || value == "" {
		return "", "", false
	}
	return name, value, true
}

func sessionCookieChunkIndex(name string) (int, bool) {
	prefix := nextAuthSessionCookieName + "."
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
	return index, err == nil && index >= 0
}

func sessionCookieSortKey(part string) int {
	name, _, _ := strings.Cut(part, "=")
	if name == nextAuthSessionCookieName || name == nextAuthSessionCookieLegacyName {
		return -1
	}
	if index, ok := sessionCookieChunkIndex(name); ok {
		return index
	}
	return 1 << 30
}
