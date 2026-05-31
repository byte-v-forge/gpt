package api

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/activities"
	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
)

func (s *Server) CreateGPTAccount(ctx context.Context, req *pb.CreateGPTAccountRequest) (*pb.CreateGPTAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	email := strings.TrimSpace(req.GetEmail())
	if email == "" {
		allocated, err := activities.NewAccountEmailAllocator(s.accountClient).Allocate(ctx, accountID, nil, requestEmailStrategy(req.GetEmailStrategy()))
		if err != nil {
			return &pb.CreateGPTAccountResponse{ErrorMessage: err.Error()}, nil
		}
		email = strings.TrimSpace(allocated)
	}
	if email == "" {
		return &pb.CreateGPTAccountResponse{ErrorMessage: "email allocator returned empty email"}, nil
	}

	resp, err := s.accountClient.CreateAccount(ctx, &pb.CreateAccountRequest{
		Account:    gptaccount.New(accountID, email, ""),
		Credential: &pb.AccountCredential{Password: req.GetPassword()},
	})
	if err != nil {
		return &pb.CreateGPTAccountResponse{ErrorMessage: err.Error()}, nil
	}
	if err := s.generateAccountFingerprint(ctx, gptaccount.ID(resp.GetAccount()), accountfingerprint.GenerateParams{
		CountryCode: req.GetCountryCode(),
		Region:      req.GetRegion(),
	}); err != nil {
		return &pb.CreateGPTAccountResponse{ErrorMessage: err.Error()}, nil
	}
	return &pb.CreateGPTAccountResponse{Account: resp.GetAccount()}, nil
}

func compactAccountIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		accountID := strings.TrimSpace(value)
		if accountID == "" || seen[accountID] {
			continue
		}
		seen[accountID] = true
		out = append(out, accountID)
	}
	return out
}

func registerAccountJobParams(accountID string, options *pb.RegisterOTPOptions, countryCode string, region string) map[string]string {
	params := map[string]string{"account_id": accountID}
	putProtocolGeoParams(params, countryCode, region)
	if options == nil {
		return params
	}
	params["registration_otp_mode"] = options.GetMode().String()
	if options.AutoResend != nil {
		params["registration_otp_auto_resend"] = boolString(options.GetAutoResend())
	}
	if options.GetFirstWaitSeconds() > 0 {
		params["registration_otp_first_wait_seconds"] = int32String(options.GetFirstWaitSeconds())
	}
	if options.GetTimeoutSeconds() > 0 {
		params["registration_otp_timeout_seconds"] = int32String(options.GetTimeoutSeconds())
	}
	return params
}

func codexOAuthJobParams(accountID string, label string) map[string]string {
	params := map[string]string{"account_id": strings.TrimSpace(accountID)}
	if label = strings.TrimSpace(label); label != "" {
		params["label"] = label
	}
	return params
}

func codexOAuthAddPhoneJobParams(accountID string, label string, maxReuseCount int32) map[string]string {
	params := codexOAuthJobParams(accountID, label)
	if maxReuseCount > 0 {
		params["max_reuse_count"] = int32String(maxReuseCount)
	}
	return params
}

func codexOAuthBatchAddPhoneJobParams(accountIDs []string, label string, maxReuseCount int32) map[string]string {
	params := map[string]string{
		"account_ids":   strings.Join(compactAccountIDs(accountIDs), ","),
		"account_count": int32String(int32(len(accountIDs))),
	}
	if label = strings.TrimSpace(label); label != "" {
		params["label"] = label
	}
	if maxReuseCount > 0 {
		params["max_reuse_count"] = int32String(maxReuseCount)
	}
	return params
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func int32String(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}

func requestEmailStrategy(strategy pb.AccountEmailStrategy) pb.AccountEmailStrategy {
	if strategy == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED {
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS
	}
	return strategy
}
