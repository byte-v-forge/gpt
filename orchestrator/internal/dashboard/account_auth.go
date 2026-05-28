package dashboard

import (
	"context"
	"strings"

	"orchestrator/internal/accountauth"
	"orchestrator/internal/chatgptauth"
	"orchestrator/pb"
)

func normalizeAccountAuthInput(sessionInput, accessInput string) (string, string) {
	return chatgptauth.NormalizeAccountAuthInput(sessionInput, accessInput)
}

func (s *server) saveAccountAuth(ctx context.Context, accountID string, sessionToken string, accessToken string) error {
	if err := accountauth.SaveChatGPTSessionToken(ctx, s.runtimeSecrets, accountID, sessionToken); err != nil {
		return err
	}
	return accountauth.SaveChatGPTAccessToken(ctx, s.runtimeSecrets, accountID, accessToken)
}

func (s *server) accountAuthTokens(ctx context.Context, accountID string) (string, string, error) {
	return accountauth.ChatGPTTokens(ctx, s.runtimeSecrets, accountID)
}

func (s *server) paymentCredential(ctx context.Context, accountID, sessionToken, accessToken string) (*pb.ChatGPTCredential, error) {
	accessToken = strings.TrimSpace(accessToken)
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" && accessToken == "" {
		return nil, nil
	}
	cred := &pb.ChatGPTCredential{SessionToken: sessionToken, AccessToken: accessToken}
	if s.fingerprints == nil || strings.TrimSpace(accountID) == "" {
		return cred, nil
	}
	profile, ok, err := s.fingerprints.Get(ctx, accountID)
	if err != nil || !ok {
		return cred, err
	}
	fingerprint := profile.Fingerprint()
	cred.RequestProfile = &pb.ChatGPTRequestProfile{
		TlsProfile:      fingerprint.TLSProfileName,
		UserAgent:       fingerprint.UserAgent,
		SecChUa:         fingerprint.SecCHUA,
		SecChUaPlatform: fingerprint.SecCHPlatform,
		AcceptLanguage:  fingerprint.AcceptLanguage,
		OaiLanguage:     fingerprint.Language,
		Locale:          profile.Locale,
		DeviceId:        fingerprint.DeviceID,
		Platform:        profile.OSFamily,
		Timezone:        profile.Timezone,
	}
	return cred, nil
}
