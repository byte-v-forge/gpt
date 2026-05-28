package activities

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/common-lib/stringx"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	"orchestrator/db"
)

func (s *Server) markCodexPhoneSuccess(ctx context.Context, phone *CodexOAuthPhoneLease, accountID, jobID, label string) error {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" || s.db == nil {
		return nil
	}
	var row db.CodexOAuthPhoneLease
	if err := s.db.WithContext(ctx).First(&row, "activation_id = ?", phone.GetActivationId()).Error; err != nil {
		return err
	}
	row.UseCount++
	row.LastJobID = jobID
	row.LastAccountID = accountID
	row.LastError = ""
	row.LastFailureKind = ""
	row.Label = strings.TrimSpace(label)
	if row.Label == "" {
		row.Label = phone.GetLabel()
	}
	if row.UseCount >= row.MaxUseCount {
		row.Status = codexOAuthLeaseExhausted
		_ = s.completeSMSActivation(ctx, row.ActivationID, "codex-oauth-exhausted-"+jobID)
	} else {
		row.Status = codexOAuthLeaseAvailable
	}
	phone.ReuseCount = row.UseCount
	phone.ReuseLimit = row.MaxUseCount
	return s.db.WithContext(ctx).Save(&row).Error
}

func (s *Server) releaseCodexPhoneAfterFailure(ctx context.Context, activationID, accountID, jobID, label, message string) error {
	return s.releaseCodexPhone(ctx, &CodexOAuthPhoneLease{ActivationId: activationID, Label: label}, accountID, jobID, label, codexOAuthFailureLikelyUsedPhone(message), message)
}

func (s *Server) releaseCodexPhone(ctx context.Context, phone *CodexOAuthPhoneLease, accountID, jobID, label string, phoneUsed bool, message string) error {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" || s.db == nil {
		return nil
	}
	var row db.CodexOAuthPhoneLease
	if err := s.db.WithContext(ctx).First(&row, "activation_id = ?", phone.GetActivationId()).Error; err != nil {
		return err
	}
	row.LastJobID = jobID
	row.LastAccountID = accountID
	row.LastError = stringx.CompactSnippet(message, 500)
	failureKind, terminalStatus := codexOAuthPhoneFailureDisposition(message)
	row.LastFailureKind = failureKind
	if strings.TrimSpace(label) != "" {
		row.Label = strings.TrimSpace(label)
	}
	if row.Status != codexOAuthLeaseInUse {
		return s.db.WithContext(ctx).Save(&row).Error
	}
	if !phoneUsed && s.codexOAuthActivationHasCode(ctx, row.ActivationID) {
		phoneUsed = true
	}
	if terminalStatus != "" {
		row.Status = terminalStatus
		if !phoneUsed {
			if err := s.cancelCodexOAuthSMSActivation(ctx, row.ActivationID, jobID, message); err != nil {
				row.LastError = stringx.CompactSnippet(row.LastError+"; cancel failed: "+err.Error(), 500)
			}
		} else if terminalStatus == codexOAuthLeaseExhausted {
			_ = s.completeSMSActivation(ctx, row.ActivationID, "codex-oauth-page-exhausted-"+jobID)
		}
		return s.db.WithContext(ctx).Save(&row).Error
	}
	if row.ExpiresAt > 0 && row.ExpiresAt <= time.Now().Unix()+int64(codexOAuthPhoneMinRemainingSeconds(s.codexOAuthConfig.withDefaults())) {
		row.Status = codexOAuthLeaseExpired
		row.LastFailureKind = codexOAuthFirstNonEmpty(row.LastFailureKind, "phone_expired")
		if !phoneUsed {
			if err := s.cancelCodexOAuthSMSActivation(ctx, row.ActivationID, jobID, "phone_expired"); err != nil {
				row.LastError = stringx.CompactSnippet(row.LastError+"; cancel failed: "+err.Error(), 500)
			}
		}
		return s.db.WithContext(ctx).Save(&row).Error
	}
	switch {
	case !phoneUsed:
		row.Status = codexOAuthLeaseAvailable
	case row.UseCount > 0 && row.UseCount < row.MaxUseCount:
		row.Status = codexOAuthLeaseAvailable
	default:
		if row.UseCount >= row.MaxUseCount {
			row.Status = codexOAuthLeaseExhausted
		} else {
			row.Status = codexOAuthLeaseFailed
		}
	}
	return s.db.WithContext(ctx).Save(&row).Error
}

func (s *Server) codexOAuthActivationHasCode(ctx context.Context, activationID string) bool {
	if s.smsClient == nil || strings.TrimSpace(activationID) == "" {
		return false
	}
	resp, err := s.smsClient.GetOrder(ctx, &smsv1.GetOrderRequest{OrderId: strings.TrimSpace(activationID)})
	if err != nil || resp == nil || resp.GetError() != nil {
		return false
	}
	switch resp.GetOrder().GetStatus() {
	case smsv1.SmsOrderStatus_SMS_ORDER_STATUS_CODE_RECEIVED,
		smsv1.SmsOrderStatus_SMS_ORDER_STATUS_COMPLETED:
		return true
	default:
		return false
	}
}

func (s *Server) cancelCodexOAuthSMSActivation(ctx context.Context, activationID, jobID, reason string) error {
	if s.smsClient == nil || strings.TrimSpace(activationID) == "" {
		return nil
	}
	resp, err := s.smsClient.CancelOrder(ctx, &smsv1.CancelOrderRequest{
		OrderId:   strings.TrimSpace(activationID),
		RequestId: "codex-oauth-cancel-" + strings.TrimSpace(jobID),
		Reason:    stringx.CompactSnippet(reason, 200),
	})
	if err != nil {
		return fmt.Errorf("CancelActivation: %w", err)
	}
	if smsCancelSettled(resp) {
		return nil
	}
	return fmt.Errorf("CancelActivation: %s", smsCancelResponseText(resp))
}

func codexOAuthPhoneLeaseFromRow(row db.CodexOAuthPhoneLease, reused bool) *CodexOAuthPhoneLease {
	return &CodexOAuthPhoneLease{
		ActivationId:       row.ActivationID,
		PhoneE164:          row.PhoneE164,
		PhoneNational:      row.PhoneNational,
		CountryIso2:        row.CountryISO2,
		CountryCallingCode: row.CountryCallingCode,
		Label:              row.Label,
		ReuseCount:         row.UseCount,
		ReuseLimit:         row.MaxUseCount,
		Reused:             reused,
		ProfileKey:         row.ProfileKey,
		ExpiresAtUnix:      row.ExpiresAt,
	}
}

func codexOAuthActivationExpiresAt(activation *smsv1.SmsOrder) int64 {
	if activation != nil && activation.GetExpiresAt() != nil {
		return activation.GetExpiresAt().AsTime().Unix()
	}
	return time.Now().Add(defaultSMSLeaseDuration).Unix()
}

func ensureCodexOAuthPhoneLeaseUsable(phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig) error {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" {
		return fmt.Errorf("codex oauth phone lease is missing")
	}
	if phone.GetReuseLimit() > 0 && phone.GetReuseCount() >= phone.GetReuseLimit() {
		return fmt.Errorf("phone_reuse_exhausted: reuse_count=%d reuse_limit=%d", phone.GetReuseCount(), phone.GetReuseLimit())
	}
	expiresAt := phone.GetExpiresAtUnix()
	if expiresAt > 0 && expiresAt <= time.Now().Unix()+int64(codexOAuthPhoneMinRemainingSeconds(cfg)) {
		return fmt.Errorf("phone_expired: expires_at=%d min_remaining_seconds=%d", expiresAt, codexOAuthPhoneMinRemainingSeconds(cfg))
	}
	return nil
}

func codexOAuthPhoneMinRemainingSeconds(cfg CodexOAuthConfig) int32 {
	cfg = cfg.withDefaults()
	if cfg.PhoneMinReuseRemainingSeconds > 0 {
		return cfg.PhoneMinReuseRemainingSeconds
	}
	value := cfg.PhoneWaitSeconds*2 + 60
	if value < defaultCodexOAuthPhoneMinReuseRemaining {
		return defaultCodexOAuthPhoneMinReuseRemaining
	}
	return value
}
