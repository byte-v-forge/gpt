package activities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
	"github.com/byte-v-forge/common-lib/timex"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	defaultSMSLeaseDuration = 20 * time.Minute
	defaultSMSPollInterval  = 5 * time.Second
	defaultSMSAcquireWait   = 2 * time.Minute
)

type smsOfferQuery struct {
	ApplicationKey     string
	CountryISO2        string
	CountryCallingCode string
	ProviderKey        string
}

func goPaySMSQuery() smsOfferQuery {
	return smsOfferQuery{ApplicationKey: "gopay", CountryISO2: "ID", CountryCallingCode: "62"}
}

func (s *Server) acquireSMSNumber(ctx context.Context, query smsOfferQuery, requestID string, labels map[string]string) (activationID string, phone string, err error) {
	if s.smsClient == nil || s.smsCatalogClient == nil {
		return "", "", fmt.Errorf("sms client not configured")
	}
	offer, err := s.selectSMSOffer(ctx, query)
	if err != nil {
		return "", "", err
	}
	request := &smsv1.AcquireNumberRequest{
		RequestId:     strings.TrimSpace(requestID),
		AcquireParams: offer.GetAcquireParams(),
		LeaseDuration: durationOrNil(defaultSMSLeaseDuration),
	}
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
	activation := resp.GetOrder()
	if activation == nil {
		return "", "", fmt.Errorf("AcquireNumber: empty sms order")
	}
	activation, err = s.waitSMSActivationAcquired(ctx, activation, defaultSMSAcquireWait)
	if err != nil {
		return activation.GetOrderId(), "", err
	}
	return activation.GetOrderId(), smsActivationPhone(activation), nil
}

func (s *Server) selectSMSOffer(ctx context.Context, query smsOfferQuery) (*smsv1.SmsPriceOffer, error) {
	query = normalizeSMSOfferQuery(query)
	if query.ApplicationKey == "" {
		return nil, fmt.Errorf("sms application key is required")
	}
	resp, err := s.smsCatalogClient.ListSmsPriceOffers(ctx, &smsv1.ListSmsPriceOffersRequest{
		ApplicationKey:     query.ApplicationKey,
		CountryIso2:        query.CountryISO2,
		CountryCallingCode: query.CountryCallingCode,
		ProviderKey:        query.ProviderKey,
	})
	if err != nil {
		return nil, fmt.Errorf("ListSmsPriceOffers: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("ListSmsPriceOffers: empty response")
	}
	if resp.GetError() != nil {
		return nil, fmt.Errorf("ListSmsPriceOffers: %s", smsErrorText(resp.GetError()))
	}
	offers := append([]*smsv1.SmsPriceOffer(nil), resp.GetOffers()...)
	offers = filterSMSOffers(offers, query)
	if len(offers) == 0 {
		return nil, fmt.Errorf("no sms offer for %s %s/%s", query.ApplicationKey, query.CountryISO2, query.CountryCallingCode)
	}
	sort.SliceStable(offers, func(i, j int) bool {
		left, right := offerPriceUSD(offers[i]), offerPriceUSD(offers[j])
		if left == right {
			return offers[i].GetProviderKey() < offers[j].GetProviderKey()
		}
		return left < right
	})
	return offers[0], nil
}

func filterSMSOffers(offers []*smsv1.SmsPriceOffer, query smsOfferQuery) []*smsv1.SmsPriceOffer {
	out := make([]*smsv1.SmsPriceOffer, 0, len(offers))
	for _, offer := range offers {
		if !smsAcquireParamsExact(offer.GetAcquireParams()) {
			continue
		}
		if query.ApplicationKey != "" && !strings.EqualFold(offer.GetApplicationKey(), query.ApplicationKey) {
			continue
		}
		if query.CountryISO2 != "" && offer.GetCountryIso2() != "" && !strings.EqualFold(offer.GetCountryIso2(), query.CountryISO2) {
			continue
		}
		if query.CountryCallingCode != "" && offer.GetCountryCallingCode() != "" && strings.TrimPrefix(offer.GetCountryCallingCode(), "+") != query.CountryCallingCode {
			continue
		}
		if query.ProviderKey != "" && !strings.EqualFold(offer.GetProviderKey(), query.ProviderKey) {
			continue
		}
		out = append(out, offer)
	}
	return out
}

func smsAcquireParamsExact(params *smsv1.SmsNumberAcquireParams) bool {
	if params == nil {
		return false
	}
	switch value := params.GetProviderParams().(type) {
	case *smsv1.SmsNumberAcquireParams_FiveSim:
		return strings.TrimSpace(value.FiveSim.GetProduct()) != "" &&
			strings.TrimSpace(value.FiveSim.GetCountry()) != "" &&
			strings.TrimSpace(value.FiveSim.GetOperator()) != ""
	case *smsv1.SmsNumberAcquireParams_SmsBower:
		return strings.TrimSpace(value.SmsBower.GetService()) != "" &&
			strings.TrimSpace(value.SmsBower.GetCountry()) != "" &&
			strings.TrimSpace(value.SmsBower.GetProviderId()) != ""
	case *smsv1.SmsNumberAcquireParams_HeroSms:
		return strings.TrimSpace(value.HeroSms.GetService()) != "" &&
			strings.TrimSpace(value.HeroSms.GetCountry()) != ""
	default:
		return false
	}
}

func normalizeSMSOfferQuery(query smsOfferQuery) smsOfferQuery {
	query.ApplicationKey = strings.TrimSpace(query.ApplicationKey)
	query.CountryISO2 = strings.ToUpper(strings.TrimSpace(query.CountryISO2))
	query.CountryCallingCode = strings.TrimPrefix(strings.TrimSpace(query.CountryCallingCode), "+")
	query.ProviderKey = strings.TrimSpace(query.ProviderKey)
	return query
}

func offerPriceUSD(offer *smsv1.SmsPriceOffer) float64 {
	if offer == nil || offer.GetPrice() == nil {
		return 0
	}
	return parseDecimal(offer.GetPrice().GetAmountDecimal())
}

func parseDecimal(value string) float64 {
	var out float64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%f", &out)
	return out
}

func (s *Server) waitSMSActivationAcquired(ctx context.Context, activation *smsv1.SmsOrder, timeout time.Duration) (*smsv1.SmsOrder, error) {
	if activation == nil {
		return nil, fmt.Errorf("sms order missing")
	}
	if smsActivationPhone(activation) != "" {
		return activation, nil
	}
	if s.smsClient == nil {
		return activation, fmt.Errorf("sms client not configured")
	}
	if timeout <= 0 {
		timeout = defaultSMSAcquireWait
	}
	deadline := time.Now().Add(timeout)
	for {
		if activation.GetStatus() != smsv1.SmsOrderStatus_SMS_ORDER_STATUS_ACQUIRE_REQUESTED {
			if smsActivationPhone(activation) != "" {
				return activation, nil
			}
			return activation, fmt.Errorf("sms order has no phone: %s", activation.GetStatus().String())
		}
		if !time.Now().Before(deadline) {
			return activation, fmt.Errorf("sms acquire timed out")
		}
		if err := timex.Sleep(ctx, defaultSMSPollInterval); err != nil {
			return activation, err
		}
		resp, err := s.smsClient.GetOrder(ctx, &smsv1.GetOrderRequest{OrderId: activation.GetOrderId()})
		if err != nil {
			return activation, fmt.Errorf("GetOrder: %w", err)
		}
		if resp.GetError() != nil {
			return activation, fmt.Errorf("GetOrder: %s", smsErrorText(resp.GetError()))
		}
		if resp.GetOrder() == nil {
			return activation, fmt.Errorf("GetOrder: empty sms order")
		}
		activation = resp.GetOrder()
		if smsActivationPhone(activation) != "" {
			return activation, nil
		}
	}
}

func smsActivationPhone(activation *smsv1.SmsOrder) string {
	phone := activation.GetPhoneNumber().GetE164Number()
	if strings.TrimSpace(phone) == "" {
		phone = activation.GetPhoneNumber().GetNationalNumber()
	}
	return strings.TrimSpace(phone)
}

func (s *Server) waitSMSCode(ctx context.Context, activationID string, timeoutSeconds int32) (string, error) {
	return s.waitSMSCodeIssuedAfter(ctx, activationID, timeoutSeconds, 0)
}

func (s *Server) waitSMSCodeIssuedAfter(ctx context.Context, activationID string, timeoutSeconds int32, issuedAfterUnix int64) (string, error) {
	if s.otpProjection == nil {
		return "", fmt.Errorf("otp projection is not configured")
	}
	if strings.TrimSpace(activationID) == "" {
		return "", fmt.Errorf("activation id missing")
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	code, ok, err := s.otpProjection.WaitSMSCode(ctx, activationID, issuedAfterUnix, timeout, defaultSMSPollInterval)
	if err != nil {
		return "", fmt.Errorf("WaitSMSCodeProjection: %w", err)
	}
	code = strings.TrimSpace(code)
	if !ok || code == "" {
		return "", fmt.Errorf("sms code not found")
	}
	return code, nil
}

func (s *Server) markSMSMessageSent(ctx context.Context, activationID string, requestID string) error {
	if s.smsClient == nil {
		return fmt.Errorf("sms client not configured")
	}
	resp, err := s.smsClient.MarkMessageSent(ctx, &smsv1.MarkMessageSentRequest{OrderId: activationID, RequestId: requestID})
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
	resp, err := s.smsClient.RequestAdditionalCode(ctx, &smsv1.RequestAdditionalCodeRequest{OrderId: activationID, RequestId: requestID})
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
	resp, err := s.smsClient.CompleteOrder(ctx, &smsv1.CompleteOrderRequest{OrderId: activationID, RequestId: requestID})
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
