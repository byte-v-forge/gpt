package activities

import (
	"strings"

	"orchestrator/pb"
)

func paymentCredential(sessionToken, accessToken string) *pb.ChatGPTCredential {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken != "" {
		return &pb.ChatGPTCredential{
			Token: &pb.ChatGPTCredential_AccessToken{AccessToken: accessToken},
		}
	}
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken != "" {
		return &pb.ChatGPTCredential{
			Token: &pb.ChatGPTCredential_SessionToken{SessionToken: sessionToken},
		}
	}
	return nil
}
