package paymentsvc

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/byte-v-forge/gpt/gopay/pb"
)

func (s *Server) FetchAccessToken(ctx context.Context, req *pb.FetchAccessTokenPaymentRequest) (*pb.FetchAccessTokenPaymentResponse, error) {
	cred := requestCredential(req.GetCredential())
	if strings.TrimSpace(cred.sessionToken) == "" {
		return &pb.FetchAccessTokenPaymentResponse{Success: false, ErrorMessage: "session_token is required"}, nil
	}
	accessToken, err := s.fetchAccessToken(ctx, cred)
	if err != nil {
		return &pb.FetchAccessTokenPaymentResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	return &pb.FetchAccessTokenPaymentResponse{Success: true, AccessToken: accessToken}, nil
}

func (s *Server) fetchAccessToken(ctx context.Context, cred credential) (string, error) {
	profile := cred.chatGPTProfile(s.cfg.CheckoutProfile)
	client, err := s.newGptClient(ctx, cred, profile)
	if err != nil {
		return "", err
	}
	defer client.close()
	client.setHeader("Accept", "application/json")
	resp, err := client.request(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", requestOptions{})
	if err != nil {
		return "", fmt.Errorf("auth session fetch failed: %w", err)
	}
	if resp.status != http.StatusOK {
		return "", fmt.Errorf("auth session returned status %d", resp.status)
	}
	accessToken := strings.TrimSpace(stringAt(resp.json, "accessToken"))
	if accessToken == "" {
		return "", fmt.Errorf("auth session did not return access token")
	}
	return accessToken, nil
}
