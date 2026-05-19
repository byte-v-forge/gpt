package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

type SMSTargetConfig struct {
	ApplicationKey     string
	CountryISO2        string
	CountryCallingCode string
	MaxPriceCurrency   string
	MaxPriceDecimal    string
	LeaseDuration      time.Duration
	PollInterval       time.Duration
}

func (c SMSTargetConfig) withDefaults() SMSTargetConfig {
	if strings.TrimSpace(c.ApplicationKey) == "" {
		c.ApplicationKey = "gopay"
	}
	if strings.TrimSpace(c.CountryISO2) == "" {
		c.CountryISO2 = "ID"
	}
	if strings.TrimSpace(c.CountryCallingCode) == "" {
		c.CountryCallingCode = "62"
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Second
	}
	return c
}

func (s *Server) acquireSMSNumber(ctx context.Context, requestID string, labels map[string]string) (activationID string, phone string, err error) {
	if s.smsClient == nil {
		return "", "", fmt.Errorf("sms client not configured")
	}
	target := s.smsTarget.withDefaults()
	resp, err := s.smsClient.AcquireNumber(ctx, &smsv1.AcquireNumberRequest{
		RequestId: requestID,
		Target: &smsv1.SmsTarget{
			ApplicationKey:     target.ApplicationKey,
			CountryIso2:        strings.ToUpper(target.CountryISO2),
			CountryCallingCode: strings.TrimPrefix(target.CountryCallingCode, "+"),
			MaxPrice:           smsMoney(target.MaxPriceCurrency, target.MaxPriceDecimal),
		},
		LeaseDuration: durationOrNil(target.LeaseDuration),
		Labels:        labels,
	})
	if err != nil {
		return "", "", fmt.Errorf("AcquireNumber: %w", err)
	}
	if resp == nil {
		return "", "", fmt.Errorf("AcquireNumber: empty response")
	}
	if resp.GetError() != nil {
		return "", "", fmt.Errorf("AcquireNumber: %s", smsErrorText(resp.GetError()))
	}
	activation := resp.GetActivation()
	if activation == nil {
		return "", "", fmt.Errorf("AcquireNumber: empty activation")
	}
	phone = activation.GetPhoneNumber().GetE164Number()
	if strings.TrimSpace(phone) == "" {
		phone = activation.GetPhoneNumber().GetNationalNumber()
	}
	return activation.GetActivationId(), phone, nil
}

func (s *Server) waitSMSCode(ctx context.Context, activationID string, timeoutSeconds int32) (string, error) {
	if s.smsClient == nil {
		return "", fmt.Errorf("sms client not configured")
	}
	if strings.TrimSpace(activationID) == "" {
		return "", fmt.Errorf("activation id missing")
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	resp, err := s.smsClient.WaitForCode(ctx, &smsv1.WaitForCodeRequest{
		ActivationId: activationID,
		Timeout:      durationOrNil(timeout),
		PollInterval: durationOrNil(s.smsTarget.withDefaults().PollInterval),
	})
	if err != nil {
		return "", fmt.Errorf("WaitForCode: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("WaitForCode: empty response")
	}
	if resp.GetError() != nil {
		return "", fmt.Errorf("WaitForCode: %s", smsErrorText(resp.GetError()))
	}
	code := strings.TrimSpace(resp.GetCode().GetValue())
	if code == "" {
		return "", fmt.Errorf("WaitForCode returned empty code")
	}
	return code, nil
}

func (s *Server) markSMSMessageSent(ctx context.Context, activationID string, requestID string) error {
	if s.smsClient == nil {
		return fmt.Errorf("sms client not configured")
	}
	resp, err := s.smsClient.MarkMessageSent(ctx, &smsv1.MarkMessageSentRequest{ActivationId: activationID, RequestId: requestID})
	if err != nil {
		return fmt.Errorf("MarkMessageSent: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("MarkMessageSent: empty response")
	}
	if resp.GetError() != nil {
		return fmt.Errorf("MarkMessageSent: %s", smsErrorText(resp.GetError()))
	}
	return nil
}

func (s *Server) requestAdditionalSMSCode(ctx context.Context, activationID string, requestID string) error {
	if s.smsClient == nil {
		return fmt.Errorf("sms client not configured")
	}
	resp, err := s.smsClient.RequestAdditionalCode(ctx, &smsv1.RequestAdditionalCodeRequest{ActivationId: activationID, RequestId: requestID})
	if err != nil {
		return fmt.Errorf("RequestAdditionalCode: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("RequestAdditionalCode: empty response")
	}
	if resp.GetError() != nil {
		return fmt.Errorf("RequestAdditionalCode: %s", smsErrorText(resp.GetError()))
	}
	return nil
}

func (s *Server) completeSMSActivation(ctx context.Context, activationID string, requestID string) error {
	if s.smsClient == nil {
		return fmt.Errorf("sms client not configured")
	}
	resp, err := s.smsClient.CompleteActivation(ctx, &smsv1.CompleteActivationRequest{ActivationId: activationID, RequestId: requestID})
	if err != nil {
		return fmt.Errorf("CompleteActivation: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("CompleteActivation: empty response")
	}
	if resp.GetError() != nil {
		return fmt.Errorf("CompleteActivation: %s", smsErrorText(resp.GetError()))
	}
	return nil
}

func smsMoney(currency string, amount string) *smsv1.DecimalMoney {
	currency = strings.TrimSpace(currency)
	amount = strings.TrimSpace(amount)
	if currency == "" && amount == "" {
		return nil
	}
	return &smsv1.DecimalMoney{CurrencyCode: currency, AmountDecimal: amount}
}

func durationOrNil(value time.Duration) *durationpb.Duration {
	if value <= 0 {
		return nil
	}
	return durationpb.New(value)
}

func smsErrorText(err *smsv1.SmsError) string {
	if err == nil {
		return ""
	}
	parts := []string{}
	if err.GetCode() != smsv1.SmsErrorCode_SMS_ERROR_CODE_UNSPECIFIED {
		parts = append(parts, err.GetCode().String())
	}
	if strings.TrimSpace(err.GetMessage()) != "" {
		parts = append(parts, strings.TrimSpace(err.GetMessage()))
	}
	if len(parts) == 0 {
		return "unknown sms error"
	}
	return strings.Join(parts, ": ")
}
