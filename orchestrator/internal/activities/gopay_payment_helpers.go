package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) paymentActivityAccount(ctx context.Context, input *GoPayActivityInput) (*pb.Account, error) {
	accountID := strings.TrimSpace(input.GetAccountId())
	if accountID != "" {
		return s.getAccount(ctx, accountID)
	}
	sessionToken := strings.TrimSpace(input.GetSessionToken())
	accessToken := strings.TrimSpace(input.GetAccessToken())
	if sessionToken == "" && accessToken == "" {
		return nil, fmt.Errorf("account_id, session_token, or access_token is required")
	}
	return &pb.Account{
		SessionToken: sessionToken,
		AccessToken:  accessToken,
	}, nil
}

func isStalePreparedPaymentFlow(resp *pb.StartGoPayResponse, err error) bool {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if resp != nil && resp.GetErrorMessage() != "" {
		if message != "" {
			message += " "
		}
		message += resp.GetErrorMessage()
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "prepared payment flow not found") ||
		strings.Contains(message, "payment flow not found")
}

func goPayPaymentHeartbeatFields(input GoPayActivityInput) map[string]any {
	return map[string]any{
		"account_id_present":          strings.TrimSpace(input.GetAccountId()) != "",
		"session_token_present":       strings.TrimSpace(input.GetSessionToken()) != "",
		"access_token_present":        strings.TrimSpace(input.GetAccessToken()) != "",
		"use_account_token":           input.GetUseAccountToken(),
		"tokenization":                strings.TrimSpace(input.GetTokenization()),
		"checkout_url_present":        strings.TrimSpace(input.GetCheckoutUrl()) != "",
		"checkout_session_id_present": strings.TrimSpace(input.GetCheckoutSessionId()) != "",
		"prepared_flow_present":       strings.TrimSpace(input.GetPreparedFlowId()) != "",
		"gopay_phone_present":         strings.TrimSpace(input.GetGopayPhone()) != "",
		"skip_account_balance_check":  input.GetSkipAccountBalanceCheck(),
		"country_code_present":        strings.TrimSpace(input.GetCountryCode()) != "",
	}
}
