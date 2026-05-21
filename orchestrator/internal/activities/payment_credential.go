package activities

import (
	"strings"

	"orchestrator/pb"
)

func paymentCredential(sessionToken, accessToken string) *pb.ChatGPTCredential {
	accessToken = strings.TrimSpace(accessToken)
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken != "" || accessToken != "" {
		return &pb.ChatGPTCredential{SessionToken: sessionToken, AccessToken: accessToken}
	}
	return nil
}
