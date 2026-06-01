package accountauth

import (
	"time"

	"github.com/byte-v-forge/common-lib/accountmodel"
	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
)

func Response(accountID string, password string, sessionToken TokenSnapshot, accessToken TokenSnapshot) *pb.AccountAuthResponse {
	return &pb.AccountAuthResponse{
		Password:                  password,
		SessionToken:              sessionToken.Value,
		SessionTokenExpiresAtUnix: sessionToken.ExpiresAtUnix,
		AccessToken:               accessToken.Value,
		AccessTokenExpiresAtUnix:  accessToken.ExpiresAtUnix,
		Account: gptaccount.Descriptor.Account(
			accountID,
			accountmodel.WithCredentials(
				accountmodel.Credential(accountmodel.CredentialKindSessionToken, sessionToken.Present, accountAuthCredentialStatus(sessionToken.Present), accountmodel.UnixTime(sessionToken.ExpiresAtUnix), time.Time{}),
				accountmodel.Credential(accountmodel.CredentialKindAccessToken, accessToken.Present, accountAuthCredentialStatus(accessToken.Present), accountmodel.UnixTime(accessToken.ExpiresAtUnix), time.Time{}),
			),
		),
	}
}

func accountAuthCredentialStatus(present bool) string {
	if !present {
		return ""
	}
	return accountmodel.CredentialStatusConfigured
}
