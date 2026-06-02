package activities

import (
	"context"
	"orchestrator/internal/chatgptauth"
	"orchestrator/internal/contracts"
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

func (s *Server) CodexOAuthAcquirePhoneActivity(ctx context.Context, input *CodexOAuthAcquirePhoneInput) (*CodexOAuthPhoneLease, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	reuseLimit := input.GetMaxReuseCount()
	if reuseLimit <= 0 {
		reuseLimit = int32(cfg.PhoneMaxReuseCount)
	}
	data := newCodexOAuthStepData(label, nil)
	data.setPhoneAcquireRequest(cfg, label, reuseLimit)
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthAcquirePhone, false, true)
	var lease *CodexOAuthPhoneLease
	_, err := step.run(func() (activityStepResult, error) {
		if err := validateCodexOAuthSMSCountry(cfg.PhoneCountryISO2); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		var err error
		lease, err = s.acquireReusableCodexPhone(ctx, input.GetJobId(), input.GetAccountId(), label, reuseLimit, cfg)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		data.setPhoneAcquired(lease)
		return data.messageData(), nil
	})
	return lease, err
}

func (s *Server) CodexOAuthReleasePhoneActivity(ctx context.Context, input *CodexOAuthReleasePhoneInput) error {
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthReleasePhone, true, false)
	_, err := step.run(func() (activityStepResult, error) {
		if strings.TrimSpace(input.GetActivationId()) == "" {
			data := newCodexOAuthStepData(input.GetLabel(), nil)
			data.setReleaseSkipped("activation_id_missing")
			return data.messageData(), nil
		}
		data := newCodexOAuthStepData(input.GetLabel(), nil)
		data.setReleaseRequested(input.GetActivationId(), input.GetLabel())
		if err := s.releaseCodexPhoneAfterFailure(ctx, input.GetActivationId(), input.GetAccountId(), input.GetJobId(), input.GetLabel(), input.GetErrorMessage()); err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		return data.messageData(), nil
	})
	return err
}
