package activities

import (
	"context"
	"net/http"
	"strings"
	"time"

	pb "orchestrator/pb"
)

type rekberinajaAddBalanceSettings struct {
	endpointURL   string
	bearerToken   string
	refreshToken  string
	deviceID      string
	store         string
	productID     string
	serviceID     string
	paymentMethod string
	invoiceEmail  string
	paymentPhone  string
	promoCode     string
	userAgent     string
	origin        string
	referer       string
	usePoin       bool
	feeTotal      int64
	pollTimeout   time.Duration
	pollInterval  time.Duration
}

func (s *Server) newRekberinajaAddBalanceSettings(ctx context.Context, cfg *pb.GoPayRekberinajaAddBalance, targetPhone string) rekberinajaAddBalanceSettings {
	bearerToken := strings.TrimSpace(cfg.GetBearerToken())
	refreshToken := strings.TrimSpace(cfg.GetRefreshToken())
	bearerToken, refreshToken = s.loadRekberinajaTokens(ctx, bearerToken, refreshToken)

	settings := rekberinajaAddBalanceSettings{
		endpointURL:   strings.TrimSpace(cfg.GetEndpointUrl()),
		bearerToken:   bearerToken,
		refreshToken:  refreshToken,
		deviceID:      strings.TrimSpace(cfg.GetDeviceId()),
		store:         strings.TrimSpace(cfg.GetStore()),
		productID:     strings.TrimSpace(cfg.GetProductId()),
		serviceID:     strings.TrimSpace(cfg.GetServiceId()),
		paymentMethod: strings.TrimSpace(cfg.GetPaymentMethod()),
		invoiceEmail:  strings.TrimSpace(cfg.GetInvoiceEmail()),
		paymentPhone:  rekberinajaPaymentPhone(targetPhone),
		promoCode:     strings.TrimSpace(cfg.GetPromoCode()),
		userAgent:     strings.TrimSpace(cfg.GetUserAgent()),
		origin:        strings.TrimSpace(cfg.GetOrigin()),
		referer:       strings.TrimSpace(cfg.GetReferer()),
		usePoin:       cfg.GetUsePoin(),
		feeTotal:      cfg.GetFeeTotal(),
		pollTimeout:   time.Duration(cfg.GetPollTimeoutSeconds()) * time.Second,
		pollInterval:  time.Duration(cfg.GetPollIntervalSeconds()) * time.Second,
	}
	if settings.paymentMethod == "" {
		settings.paymentMethod = "saldo"
	}
	if settings.store == "" {
		settings.store = "rekberinaja"
	}
	if settings.pollTimeout <= 0 {
		settings.pollTimeout = 180 * time.Second
	}
	if settings.pollInterval <= 0 {
		settings.pollInterval = 5 * time.Second
	}
	return settings
}

func (s rekberinajaAddBalanceSettings) metadata() map[string]any {
	return map[string]any{
		"endpoint_present":      s.endpointURL != "",
		"bearer_token_present":  s.bearerToken != "",
		"refresh_token_present": s.refreshToken != "",
		"device_id_present":     s.deviceID != "",
		"store":                 s.store,
		"product_id_present":    s.productID != "",
		"service_id_present":    s.serviceID != "",
		"payment_method":        s.paymentMethod,
		"invoice_email_present": s.invoiceEmail != "",
		"target_phone_present":  s.paymentPhone != "",
		"fee_total":             s.feeTotal,
	}
}

func (s rekberinajaAddBalanceSettings) missingRequiredFields() []string {
	missing := []string{}
	if s.endpointURL == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_ENDPOINT_URL")
	}
	if s.bearerToken == "" && s.refreshToken == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_BEARER_TOKEN or GOPAY_ADD_BALANCE_REKBERINAJA_REFRESH_TOKEN")
	}
	if s.deviceID == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_DEVICE_ID")
	}
	if s.productID == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_PRODUCT_ID")
	}
	if s.serviceID == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_SERVICE_ID")
	}
	if s.invoiceEmail == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_INVOICE_EMAIL")
	}
	if s.paymentPhone == "" {
		missing = append(missing, "target_phone")
	}
	return missing
}

func (s rekberinajaAddBalanceSettings) newAPIClient(ctx context.Context, server *Server, apiBaseURL string) *rekberinajaAPIClient {
	return &rekberinajaAPIClient{
		httpClient:   &http.Client{Timeout: 45 * time.Second},
		endpointURL:  s.endpointURL,
		apiBaseURL:   apiBaseURL,
		accessToken:  s.bearerToken,
		refreshToken: s.refreshToken,
		deviceID:     s.deviceID,
		store:        s.store,
		userAgent:    s.userAgent,
		origin:       s.origin,
		referer:      s.referer,
		onTokenRefresh: func(accessToken, refreshToken string) error {
			return server.saveRekberinajaTokens(ctx, accessToken, refreshToken)
		},
	}
}

func (s rekberinajaAddBalanceSettings) checkoutPayload() map[string]any {
	return map[string]any{
		"product_id":     s.productID,
		"promo_code":     s.promoCode,
		"use_poin":       s.usePoin,
		"data":           s.paymentPhone,
		"payment_method": s.paymentMethod,
		"invoice_email":  s.invoiceEmail,
		"service_id":     s.serviceID,
	}
}

func (s rekberinajaAddBalanceSettings) pollTimeoutSeconds() int {
	return int(s.pollTimeout / time.Second)
}

func (s rekberinajaAddBalanceSettings) pollIntervalSeconds() int {
	return int(s.pollInterval / time.Second)
}
