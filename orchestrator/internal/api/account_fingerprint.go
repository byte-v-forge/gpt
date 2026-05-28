package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/pb"
)

func (s *Server) generateAccountFingerprint(ctx context.Context, accountID string, params accountfingerprint.GenerateParams) error {
	if s.fingerprints == nil {
		return fmt.Errorf("account fingerprint store is not configured")
	}
	if strings.TrimSpace(accountID) == "" {
		return fmt.Errorf("account_id is required")
	}
	_, _, err := s.fingerprints.Generate(ctx, accountID, params)
	return err
}

func (s *Server) accountFingerprint(ctx context.Context, accountID string) (accountfingerprint.Profile, error) {
	if s.fingerprints == nil {
		return accountfingerprint.Profile{}, fmt.Errorf("account fingerprint store is not configured")
	}
	profile, ok, err := s.fingerprints.Get(ctx, accountID)
	if err != nil {
		return accountfingerprint.Profile{}, err
	}
	if !ok {
		return accountfingerprint.Profile{}, fmt.Errorf("account fingerprint is not generated")
	}
	return profile, nil
}

func (s *Server) paymentCredential(ctx context.Context, accountID, sessionToken, accessToken string) (*pb.ChatGPTCredential, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	accessToken = strings.TrimSpace(accessToken)
	if sessionToken == "" && accessToken == "" {
		return nil, nil
	}
	cred := &pb.ChatGPTCredential{SessionToken: sessionToken, AccessToken: accessToken}
	if strings.TrimSpace(accountID) == "" {
		return cred, nil
	}
	profile, err := s.accountFingerprint(ctx, accountID)
	if err != nil {
		return nil, err
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
