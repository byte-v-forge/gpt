package activities

import (
	"context"
	"strings"

	"orchestrator/pb"
)

func (s *Server) paymentCredential(ctx context.Context, accountID, sessionToken, accessToken string) (*pb.ChatGPTCredential, error) {
	return s.paymentCredentialWithProxy(ctx, accountID, sessionToken, accessToken, "")
}

func (s *Server) paymentCredentialWithProxy(ctx context.Context, accountID, sessionToken, accessToken string, proxyURL string) (*pb.ChatGPTCredential, error) {
	accessToken = strings.TrimSpace(accessToken)
	sessionToken = strings.TrimSpace(sessionToken)
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
	if proxyURL = strings.TrimSpace(proxyURL); proxyURL != "" {
		cred.RequestProfile.ProxyUrl = proxyURL
	}
	return cred, nil
}
