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
	ProviderKeys       []string

	UseRouteRecommendation bool
	RouteStrategy          string
	RouteLimit             int32
	MinAvailableCount      int32
	MaxPriceAmount         string
	MaxPriceCurrency       string
	FailureScopeKey        string
	FailureThreshold       int32
	FailureWindowSeconds   int32
	DisableTTLSeconds      int32
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
		AcquireParams: smsAcquireParamsFromOffer(offer, query),
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
	if query.UseRouteRecommendation {
		return s.recommendSMSOffer(ctx, query)
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

func (s *Server) recommendSMSOffer(ctx context.Context, query smsOfferQuery) (*smsv1.SmsPriceOffer, error) {
	resp, err := s.smsCatalogClient.RecommendSmsRoutes(ctx, &smsv1.RecommendSmsRoutesRequest{
		Target: &smsv1.SmsTarget{
			ApplicationKey:     query.ApplicationKey,
			CountryIso2:        query.CountryISO2,
			CountryCallingCode: query.CountryCallingCode,
		},
		Policy:       smsRoutePolicy(query),
		ProviderKeys: smsProviderKeys(query),
	})
	if err != nil {
		return nil, fmt.Errorf("RecommendSmsRoutes: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("RecommendSmsRoutes: empty response")
	}
	if resp.GetError() != nil {
		return nil, fmt.Errorf("RecommendSmsRoutes: %s", smsErrorText(resp.GetError()))
	}
	for _, recommendation := range resp.GetRecommendations() {
		offer := recommendation.GetOffer()
		if smsOfferRefExact(offer.GetOfferRef()) {
			return offer, nil
		}
	}
	return nil, fmt.Errorf("no recommended sms route for %s %s/%s", query.ApplicationKey, query.CountryISO2, query.CountryCallingCode)
}

func filterSMSOffers(offers []*smsv1.SmsPriceOffer, query smsOfferQuery) []*smsv1.SmsPriceOffer {
	out := make([]*smsv1.SmsPriceOffer, 0, len(offers))
	for _, offer := range offers {
		if !smsOfferRefExact(offer.GetOfferRef()) {
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

func smsAcquireParamsFromOffer(offer *smsv1.SmsPriceOffer, query smsOfferQuery) *smsv1.SmsNumberAcquireParams {
	if offer == nil {
		return nil
	}
	return &smsv1.SmsNumberAcquireParams{
		OfferRef:           offer.GetOfferRef(),
		ApplicationKey:     firstNonEmpty(offer.GetApplicationKey(), query.ApplicationKey),
		CountryIso2:        firstNonEmpty(offer.GetCountryIso2(), query.CountryISO2),
		CountryCallingCode: firstNonEmpty(strings.TrimPrefix(offer.GetCountryCallingCode(), "+"), query.CountryCallingCode),
		MinAvailableCount:  query.MinAvailableCount,
		MaxPrice:           smsMaxPrice(query),
		RouteFailurePolicy: smsFailurePolicy(query),
	}
}

func smsOfferRefExact(ref *smsv1.SmsOfferRef) bool {
	if ref == nil {
		return false
	}
	target := ref.GetTarget()
	return strings.TrimSpace(ref.GetProviderKey()) != "" &&
		strings.TrimSpace(ref.GetOfferId()) != "" &&
		strings.TrimSpace(target.GetApplicationKey()) != "" &&
		(strings.TrimSpace(target.GetCountryIso2()) != "" || strings.TrimSpace(target.GetCountryCallingCode()) != "")
}

func normalizeSMSOfferQuery(query smsOfferQuery) smsOfferQuery {
	query.ApplicationKey = strings.TrimSpace(query.ApplicationKey)
	query.CountryISO2 = strings.ToUpper(strings.TrimSpace(query.CountryISO2))
	query.CountryCallingCode = strings.TrimPrefix(strings.TrimSpace(query.CountryCallingCode), "+")
	query.ProviderKey = strings.TrimSpace(query.ProviderKey)
	query.ProviderKeys = normalizeSMSProviderKeys(query.ProviderKeys)
	query.RouteStrategy = strings.TrimSpace(query.RouteStrategy)
	query.MaxPriceAmount = strings.TrimSpace(query.MaxPriceAmount)
	query.MaxPriceCurrency = strings.ToUpper(strings.TrimSpace(query.MaxPriceCurrency))
	query.FailureScopeKey = strings.TrimSpace(query.FailureScopeKey)
	if query.RouteLimit < 0 {
		query.RouteLimit = 0
	}
	if query.MinAvailableCount < 0 {
		query.MinAvailableCount = 0
	}
	if query.FailureThreshold < 0 {
		query.FailureThreshold = 0
	}
	if query.FailureWindowSeconds < 0 {
		query.FailureWindowSeconds = 0
	}
	if query.DisableTTLSeconds < 0 {
		query.DisableTTLSeconds = 0
	}
	return query
}

func smsRoutePolicy(query smsOfferQuery) *smsv1.SmsRoutePolicy {
	policy := &smsv1.SmsRoutePolicy{
		Strategy:          smsRouteStrategy(query.RouteStrategy),
		Limit:             query.RouteLimit,
		MinAvailableCount: query.MinAvailableCount,
	}
	policy.MaxPrice = smsMaxPrice(query)
	policy.FailurePolicy = smsFailurePolicy(query)
	return policy
}

func smsMaxPrice(query smsOfferQuery) *smsv1.DecimalMoney {
	if query.MaxPriceAmount == "" {
		return nil
	}
	return &smsv1.DecimalMoney{
		AmountDecimal: query.MaxPriceAmount,
		CurrencyCode:  query.MaxPriceCurrency,
	}
}

func smsFailurePolicy(query smsOfferQuery) *smsv1.SmsRouteFailurePolicy {
	if !smsRouteFailurePolicyConfigured(query) {
		return nil
	}
	return &smsv1.SmsRouteFailurePolicy{
		ScopeKey:             query.FailureScopeKey,
		FailureThreshold:     query.FailureThreshold,
		FailureWindowSeconds: query.FailureWindowSeconds,
		DisableTtlSeconds:    query.DisableTTLSeconds,
	}
}

func smsRouteStrategy(value string) smsv1.SmsRouteStrategy {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "lowest_price", "lowest-price", "price", "cheap", "cheapest":
		return smsv1.SmsRouteStrategy_SMS_ROUTE_STRATEGY_LOWEST_PRICE
	case "most_available", "most-available", "available", "availability", "inventory", "stock":
		return smsv1.SmsRouteStrategy_SMS_ROUTE_STRATEGY_MOST_AVAILABLE
	case "balanced", "balance":
		return smsv1.SmsRouteStrategy_SMS_ROUTE_STRATEGY_BALANCED
	default:
		return smsv1.SmsRouteStrategy_SMS_ROUTE_STRATEGY_LOWEST_PRICE
	}
}

func smsRouteFailurePolicyConfigured(query smsOfferQuery) bool {
	return query.FailureScopeKey != "" ||
		query.FailureThreshold > 0 ||
		query.FailureWindowSeconds > 0 ||
		query.DisableTTLSeconds > 0
}

func smsProviderKeys(query smsOfferQuery) []string {
	keys := make([]string, 0, len(query.ProviderKeys)+1)
	if query.ProviderKey != "" {
		keys = append(keys, query.ProviderKey)
	}
	keys = append(keys, query.ProviderKeys...)
	return normalizeSMSProviderKeys(keys)
}

func normalizeSMSProviderKeys(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
