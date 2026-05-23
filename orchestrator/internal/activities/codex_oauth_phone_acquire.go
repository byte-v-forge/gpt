package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"orchestrator/db"
)

func (s *Server) acquireReusableCodexPhone(ctx context.Context, jobID, accountID, label string, reuseLimit int32, cfg CodexOAuthConfig) (*CodexOAuthPhoneLease, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	now := time.Now().Unix()
	minRemaining := codexOAuthPhoneMinRemainingSeconds(cfg)
	reusableAfter := now + int64(minRemaining)
	var reused db.CodexOAuthPhoneLease
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&db.CodexOAuthPhoneLease{}).
			Where("status = ? AND expires_at > 0 AND expires_at <= ?", codexOAuthLeaseAvailable, reusableAfter).
			Updates(map[string]any{
				"status":            codexOAuthLeaseExpired,
				"last_failure_kind": "phone_expired",
				"last_error":        fmt.Sprintf("phone lease expires before required reuse window: expires_at <= %d", reusableAfter),
			}).Error; err != nil {
			return err
		}
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND use_count < max_use_count", codexOAuthLeaseAvailable).
			Where("profile_key = ?", strings.TrimSpace(cfg.PhoneProfileKey)).
			Where("expires_at > ?", reusableAfter).
			Order("updated_at DESC")
		if cfg.PhoneCountryISO2 != "" {
			query = query.Where("country_iso2 = ?", cfg.PhoneCountryISO2)
		}
		if err := query.First(&reused).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		reused.Status = codexOAuthLeaseInUse
		reused.LastJobID = jobID
		reused.LastAccountID = accountID
		reused.LastError = ""
		reused.LastFailureKind = ""
		if strings.TrimSpace(label) != "" {
			reused.Label = label
		}
		return tx.Save(&reused).Error
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reused.ActivationID) != "" {
		return codexOAuthPhoneLeaseFromRow(reused, reused.UseCount > 0), nil
	}

	activation, err := s.acquireCodexSMSActivation(ctx, jobID, accountID, label, reuseLimit, cfg)
	if err != nil {
		return nil, err
	}
	row := db.CodexOAuthPhoneLease{
		ActivationID:       activation.GetActivationId(),
		PhoneE164:          activation.GetPhoneNumber().GetE164Number(),
		PhoneNational:      activation.GetPhoneNumber().GetNationalNumber(),
		CountryISO2:        activation.GetPhoneNumber().GetCountryIso2(),
		CountryCallingCode: activation.GetPhoneNumber().GetCountryCallingCode(),
		ProfileKey:         cfg.PhoneProfileKey,
		Status:             codexOAuthLeaseInUse,
		Label:              label,
		UseCount:           0,
		MaxUseCount:        reuseLimit,
		ExpiresAt:          codexOAuthActivationExpiresAt(activation),
		LastJobID:          jobID,
		LastAccountID:      accountID,
	}
	if row.CountryISO2 == "" {
		row.CountryISO2 = cfg.PhoneCountryISO2
	}
	if row.CountryCallingCode == "" {
		row.CountryCallingCode = cfg.PhoneCountryCallingCode
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return codexOAuthPhoneLeaseFromRow(row, false), nil
}

func (s *Server) acquireCodexSMSActivation(ctx context.Context, jobID, accountID, label string, reuseLimit int32, cfg CodexOAuthConfig) (*smsv1.SmsActivation, error) {
	if s.smsClient == nil {
		return nil, fmt.Errorf("sms client not configured")
	}
	request := &smsv1.AcquireNumberRequest{
		RequestId:     "codex-oauth-" + strings.TrimSpace(jobID),
		ProfileKey:    strings.TrimSpace(cfg.PhoneProfileKey),
		LeaseDuration: durationOrNil(defaultSMSLeaseDuration),
		Target: &smsv1.SmsTarget{
			ApplicationKey:     "openai",
			CountryIso2:        cfg.PhoneCountryISO2,
			CountryCallingCode: cfg.PhoneCountryCallingCode,
			MaxPrice: &smsv1.DecimalMoney{
				CurrencyCode:  "USD",
				AmountDecimal: cfg.PhoneMaxPriceUSD,
			},
		},
		Labels: map[string]string{
			"domain":          "gpt",
			"workflow":        "codex_oauth",
			"action":          actionCodexOAuthAddPhone,
			"job_id":          jobID,
			"account_id":      accountID,
			"label":           label,
			"profile_key":     cfg.PhoneProfileKey,
			"max_reuse_count": fmt.Sprintf("%d", reuseLimit),
		},
	}
	normalizeAcquireNumberRequest(request)
	resp, err := s.smsClient.AcquireNumber(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("AcquireNumber: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("AcquireNumber: empty response")
	}
	if resp.GetError() != nil {
		return nil, fmt.Errorf("AcquireNumber: %s", smsErrorText(resp.GetError()))
	}
	if resp.GetActivation() == nil {
		return nil, fmt.Errorf("AcquireNumber: empty activation")
	}
	return resp.GetActivation(), nil
}
