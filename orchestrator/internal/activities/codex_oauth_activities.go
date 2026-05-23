package activities

import (
	"context"
	"fmt"
	"strings"
)

const (
	codexOAuthLeaseAvailable = "available"
	codexOAuthLeaseInUse     = "in_use"
	codexOAuthLeaseExhausted = "exhausted"
	codexOAuthLeaseFailed    = "failed"
	codexOAuthLeaseExpired   = "expired"

	codexOAuthAuthSecretPrefix = "codex_oauth_auth_json:"
)

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
	}
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthAcquirePhone, false, true)
	var lease *CodexOAuthPhoneLease
	_, err := step.run(func() (any, error) {
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

func (s *Server) CodexOAuthRunActivity(ctx context.Context, input CodexOAuthRunInput) (CodexOAuthRunOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthRunOutput{
		PhoneLabel:      label,
		PhoneReuseCount: input.GetPhone().GetReuseCount(),
		PhoneReuseLimit: input.GetPhone().GetReuseLimit(),
	}
	data := map[string]any{
		"label":                 label,
		"profile_key":           input.GetPhone().GetProfileKey(),
		"phone_reused":          input.GetPhone().GetReused(),
		"phone_reuse_count":     input.GetPhone().GetReuseCount(),
		"phone_reuse_limit":     input.GetPhone().GetReuseLimit(),
		"phone_expires_at_unix": input.GetPhone().GetExpiresAtUnix(),
		"phone_activation_id":   input.GetPhone().GetActivationId(),
		"phone_country_iso2":    input.GetPhone().GetCountryIso2(),
		"phone_country_code":    input.GetPhone().GetCountryCallingCode(),
		"phone_mask":            maskPhone(input.GetPhone().GetPhoneE164(), input.GetPhone().GetPhoneNational()),
		"auth_secret_written":   false,
		"account_auth_written":  false,
		"add_phone_confirmed":   false,
		"callback_url_captured": false,
	}
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthBrowser, false, true)
	_, err := step.run(func() (any, error) {
		account, err := s.getAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if strings.TrimSpace(account.GetEmail()) == "" || strings.TrimSpace(account.GetPassword()) == "" {
			err = fmt.Errorf("account email/password is required")
			data["error_message"] = err.Error()
			return data, err
		}
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthBrowser, "running codex oauth browser flow", data)
		defer stopHeartbeat()
		result, err := s.runCodexOAuthBrowser(ctx, account, input.GetJobId(), label, input.GetPhone(), cfg, input.GetAllowAddPhone(), input.GetMarkPhoneConfirmedOnSuccess(), data)
		if err != nil {
			data["error_message"] = err.Error()
			output.ErrorMessage = err.Error()
			return data, err
		}
		output.Success = true
		output.AuthSecretKey = result.authSecretKey
		output.PhoneLabel = label
		output.PhoneReuseCount = result.phoneReuseCount
		output.PhoneReuseLimit = result.phoneReuseLimit
		output.AddPhoneConfirmed = result.addPhoneConfirmed
		output.AddPhoneRequired = result.addPhoneRequired
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
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
