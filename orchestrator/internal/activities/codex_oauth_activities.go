package activities

import (
	"context"
	"orchestrator/internal/chatgptauth"
	"strings"
)

const (
	codexOAuthLeaseAvailable = "available"
	codexOAuthLeaseInUse     = "in_use"
	codexOAuthLeaseExhausted = "exhausted"
	codexOAuthLeaseFailed    = "failed"
	codexOAuthLeaseExpired   = "expired"
)

func codexOAuthAuthSecretKey(accountID string) string {
	return chatgptauth.AccountAuthSecretKey(accountID)
}

func (s *Server) CodexOAuthAcquirePhoneActivity(ctx context.Context, input CodexOAuthAcquirePhoneInput) (*CodexOAuthPhoneLease, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	reuseLimit := input.GetMaxReuseCount()
	if reuseLimit <= 0 {
		reuseLimit = int32(cfg.PhoneMaxReuseCount)
	}
	data := map[string]any{
		"profile_key":          cfg.PhoneProfileKey,
		"country_iso2":         cfg.PhoneCountryISO2,
		"country_calling_code": cfg.PhoneCountryCallingCode,
		"max_reuse_count":      reuseLimit,
		"label":                label,
		"verification_channel": "sms",
	}
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthAcquirePhone, false, true)
	var lease *CodexOAuthPhoneLease
	_, err := step.run(func() (any, error) {
		if err := validateCodexOAuthSMSCountry(cfg.PhoneCountryISO2); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		var err error
		lease, err = s.acquireReusableCodexPhone(ctx, input.GetJobId(), input.GetAccountId(), label, reuseLimit, cfg)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		data["activation_id"] = lease.GetActivationId()
		data["phone_reused"] = lease.GetReused()
		data["phone_reuse_count"] = lease.GetReuseCount()
		data["phone_reuse_limit"] = lease.GetReuseLimit()
		data["phone_expires_at_unix"] = lease.GetExpiresAtUnix()
		data["phone_mask"] = maskPhone(lease.GetPhoneE164(), lease.GetPhoneNational())
		return data, nil
	})
	return lease, err
}

func (s *Server) CodexOAuthReleasePhoneActivity(ctx context.Context, input CodexOAuthReleasePhoneInput) error {
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthReleasePhone, true, false)
	_, err := step.run(func() (any, error) {
		if strings.TrimSpace(input.GetActivationId()) == "" {
			return map[string]any{"released": false, "reason": "activation_id_missing"}, nil
		}
		data := map[string]any{
			"activation_id": input.GetActivationId(),
			"label":         input.GetLabel(),
			"released":      true,
		}
		if err := s.releaseCodexPhoneAfterFailure(ctx, input.GetActivationId(), input.GetAccountId(), input.GetJobId(), input.GetLabel(), input.GetErrorMessage()); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		return data, nil
	})
	return err
}
