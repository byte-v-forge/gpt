package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	defaultSMSLeaseDuration = 20 * time.Minute
	defaultSMSPollInterval  = 5 * time.Second
)

func goPaySMSRequest(requestID string, labels map[string]string) *smsv1.AcquireNumberRequest {
	return &smsv1.AcquireNumberRequest{
		RequestId:     requestID,
		ProfileKey:    "gopay-id",
		LeaseDuration: durationOrNil(defaultSMSLeaseDuration),
		Labels:        labels,
	}
}

func (s *Server) acquireSMSNumber(ctx context.Context, request *smsv1.AcquireNumberRequest) (activationID string, phone string, err error) {
	if s.smsClient == nil {
		return "", "", fmt.Errorf("sms client not configured")
	}
	if request == nil || (strings.TrimSpace(request.GetProfileKey()) == "" && request.GetTarget() == nil) {
		return "", "", fmt.Errorf("sms acquire request profile or target is required")
	}
	normalizeAcquireNumberRequest(request)
	resp, err := s.smsClient.AcquireNumber(ctx, request)
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

func normalizeAcquireNumberRequest(request *smsv1.AcquireNumberRequest) {
	if request == nil {
		return
	}
	request.ProfileKey = strings.TrimSpace(request.GetProfileKey())
	request.ProviderKey = strings.TrimSpace(request.GetProviderKey())
	request.ProviderConfigId = strings.TrimSpace(request.GetProviderConfigId())
	if request.GetTarget() == nil {
		return
	}
	request.Target.ApplicationKey = strings.TrimSpace(request.GetTarget().GetApplicationKey())
	request.Target.CountryIso2 = strings.ToUpper(strings.TrimSpace(request.GetTarget().GetCountryIso2()))
	request.Target.CountryCallingCode = strings.TrimPrefix(strings.TrimSpace(request.GetTarget().GetCountryCallingCode()), "+")
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
		PollInterval: durationOrNil(defaultSMSPollInterval),
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
