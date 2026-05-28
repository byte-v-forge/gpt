package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
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
		ActivationID:       activation.GetOrderId(),
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

func (s *Server) acquireCodexSMSActivation(ctx context.Context, jobID, accountID, label string, reuseLimit int32, cfg CodexOAuthConfig) (*smsv1.SmsOrder, error) {
	if s.smsClient == nil || s.smsCatalogClient == nil {
		return nil, fmt.Errorf("sms client not configured")
	}
	query := smsOfferQuery{
		ApplicationKey:     "openai",
		CountryISO2:        cfg.PhoneCountryISO2,
		CountryCallingCode: cfg.PhoneCountryCallingCode,
	}
	offer, err := s.selectSMSOffer(ctx, query)
	if err != nil {
		return nil, err
	}
	request := &smsv1.AcquireNumberRequest{
		RequestId:     "codex-oauth-" + strings.TrimSpace(jobID),
		AcquireParams: offer.GetAcquireParams(),
		LeaseDuration: durationOrNil(defaultSMSLeaseDuration),
	}
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
	if resp.GetOrder() == nil {
		return nil, fmt.Errorf("AcquireNumber: empty sms order")
	}
	return s.waitSMSActivationAcquired(ctx, resp.GetOrder(), defaultSMSAcquireWait)
}
