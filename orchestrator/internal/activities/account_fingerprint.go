package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/byte-v-forge/common-lib/fingerprinthttp"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/pb"
)

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

func (s *Server) generateAccountFingerprint(ctx context.Context, accountID string, params accountfingerprint.GenerateParams) error {
	if s.fingerprints == nil || strings.TrimSpace(accountID) == "" {
		return nil
	}
	_, _, err := s.fingerprints.Generate(ctx, accountID, params)
	return err
}

func (s *Server) browserAuthConfigForAccount(ctx context.Context, account *pb.Account) (BrowserAuthConfig, error) {
	cfg := s.browserAuthSettings(ctx)
	profile, err := s.accountFingerprint(ctx, account.GetAccountId())
	if err != nil {
		return cfg, err
	}
	fingerprint := profile.Fingerprint()
	cfg.Locale = profile.Locale
	cfg.AcceptLanguage = fingerprint.AcceptLanguage
	cfg.Timezone = profile.Timezone
	cfg.UserAgent = fingerprint.UserAgent
	cfg.SecCHUA = fingerprint.SecCHUA
	cfg.SecCHPlatform = fingerprint.SecCHPlatform
	cfg.DeviceID = fingerprint.DeviceID
	cfg.TLSProfileName = fingerprint.TLSProfileName
	return cfg, nil
}

func (s *Server) newAccountGptClient(ctx context.Context, accountID string, cfg CodexOAuthConfig, state *codexOAuthProtocolState) (*GptClient, error) {
	profile := codexOAuthProtocolDefaultProfile(cfg)
	if strings.TrimSpace(accountID) != "" {
		accountProfile, err := s.accountFingerprint(ctx, accountID)
		if err != nil {
			return nil, err
		}
		profile = codexOAuthProtocolProfileFromAccount(accountProfile, cfg)
	}
	return newGptClient(cfg, state, profile)
}

func codexOAuthProtocolProfileFromAccount(profile accountfingerprint.Profile, cfg CodexOAuthConfig) fingerprinthttp.Profile {
	cfg = cfg.withDefaults()
	fingerprint := profile.Fingerprint()
	return fingerprinthttp.Profile{
		ProxyURL:       cfg.ProtocolProxyURL,
		TLSProfileName: fingerprint.TLSProfileName,
		UserAgent:      fingerprint.UserAgent,
		SecCHUA:        fingerprint.SecCHUA,
		SecCHPlatform:  fingerprint.SecCHPlatform,
		AcceptLanguage: codexOAuthProtocolAcceptLanguage,
		Language:       codexOAuthProtocolLanguage,
		DeviceID:       fingerprint.DeviceID,
	}
}

func codexOAuthConfigWithInputProxy(cfg CodexOAuthConfig, input ProtocolAuthStartInput) CodexOAuthConfig {
	cfg = cfg.withDefaults()
	if proxyURL := strings.TrimSpace(input.GetProxyUrl()); proxyURL != "" {
		cfg.ProtocolProxyURL = proxyURL
	}
	return cfg
}
