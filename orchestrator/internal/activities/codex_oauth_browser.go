package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

type codexOAuthBrowserResult struct {
	authSecretKey     string
	phoneReuseCount   int32
	phoneReuseLimit   int32
	addPhoneConfirmed bool
	addPhoneRequired  bool
}

func (s *Server) codexOAuthBrowserAccount(ctx context.Context, accountID string) (*pb.Account, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(account.GetEmail()) == "" || strings.TrimSpace(account.GetPassword()) == "" {
		return nil, fmt.Errorf("account email/password is required")
	}
	return account, nil
}

func codexOAuthBrowserData(label string, phone *CodexOAuthPhoneLease) map[string]any {
	data := map[string]any{"label": label}
	if phone == nil {
		return data
	}
	data["profile_key"] = phone.GetProfileKey()
	data["phone_reused"] = phone.GetReused()
	data["phone_reuse_count"] = phone.GetReuseCount()
	data["phone_reuse_limit"] = phone.GetReuseLimit()
	data["phone_expires_at_unix"] = phone.GetExpiresAtUnix()
	data["phone_activation_id"] = phone.GetActivationId()
	data["phone_country_iso2"] = phone.GetCountryIso2()
	data["phone_country_code"] = phone.GetCountryCallingCode()
	data["phone_mask"] = maskPhone(phone.GetPhoneE164(), phone.GetPhoneNational())
	if strings.TrimSpace(phone.GetCountryIso2()) != "" {
		data["verification_channel"] = "sms"
	}
	return data
}

func codexOAuthAddPhoneRequiredError() error {
	return fmt.Errorf("codex_oauth_add_phone_required")
}
