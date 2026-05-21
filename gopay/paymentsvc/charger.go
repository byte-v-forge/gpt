package paymentsvc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type charger struct {
	cfg              Config
	cs               *httpSession
	ext              *httpSession
	countryCode      string
	phone            string
	pin              string
	tokenization     string
	checkoutURL      string
	midtransMerchant string
}

func (s *Server) newCharger(ctx context.Context, cred credential, phone, countryCode, pin, tokenization string) (*charger, error) {
	cs, err := s.newChatGPTSession(ctx, cred)
	if err != nil {
		return nil, err
	}
	ext, err := newHTTPSession(s.cfg.PaymentProxyURL)
	if err != nil {
		cs.close()
		return nil, err
	}
	ext.headers.Set("User-Agent", firstNonEmpty(cs.headers.Get("User-Agent"), "Mozilla/5.0 (Macintosh; Intel Mac OS X 12_2_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"))
	if strings.HasPrefix(strings.ToLower(s.cfg.BrowserLocale), "zh") {
		ext.headers.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	} else {
		ext.headers.Set("Accept-Language", "en-US,en;q=0.9")
	}
	return &charger{
		cfg:          s.cfg,
		cs:           cs,
		ext:          ext,
		countryCode:  normalizeCountryCode(countryCode),
		phone:        normalizeDigits(phone),
		pin:          strings.TrimSpace(pin),
		tokenization: normalizeTokenization(tokenization),
	}, nil
}

func (c *charger) close() {
	if c == nil {
		return
	}
	if c.cs != nil {
		c.cs.close()
	}
	if c.ext != nil {
		c.ext.close()
	}
}

func (c *charger) requiresManualConfirmation() bool {
	return strings.EqualFold(strings.TrimSpace(c.tokenization), "false")
}

func (c *charger) createCheckout(ctx context.Context) (string, error) {
	plan := c.cfg.CheckoutPlan
	body := map[string]any{
		"entry_point": firstNonEmpty(plan["entry_point"], "all_plans_pricing_modal"),
		"plan_name":   firstNonEmpty(plan["plan_name"], "chatgptplusplan"),
		"billing_details": map[string]any{
			"country":  firstNonEmpty(plan["billing_country"], "ID"),
			"currency": firstNonEmpty(plan["billing_currency"], "IDR"),
		},
		"checkout_ui_mode": firstNonEmpty(plan["checkout_ui_mode"], "hosted"),
	}
	if cancelURL := firstNonEmpty(plan["cancel_url"], "https://chatgpt.com/#pricing"); cancelURL != "" {
		body["cancel_url"] = cancelURL
	}
	if promo := firstNonEmpty(plan["promo_campaign_id"], "plus-1-month-free"); promo != "" {
		body["promo_campaign"] = map[string]any{"promo_campaign_id": promo, "is_coupon_from_query_param": false}
	}
	resp, err := c.cs.request(ctx, http.MethodPost, "https://chatgpt.com/backend-api/payments/checkout", requestOptions{jsonBody: body})
	if err != nil {
		return "", err
	}
	if resp.status >= 400 {
		return "", fmt.Errorf("checkout create failed: status=%d %s", resp.status, resp.excerpt(500))
	}
	csID := extractCheckoutSessionID(resp.json)
	if csID == "" {
		return "", fmt.Errorf("checkout create: bad response %s", jsonExcerpt(resp.json, 500))
	}
	c.checkoutURL = checkoutURLFromResponse(resp.json, csID)
	return csID, nil
}

func (c *charger) stripeCreatePaymentMethod(ctx context.Context, csID string) (string, error) {
	billing := c.cfg.Billing
	form := url.Values{
		"billing_details[name]":                            {firstNonEmpty(billing["name"], "John Doe")},
		"billing_details[email]":                           {firstNonEmpty(billing["email"], "buyer@example.com")},
		"billing_details[address][country]":                {firstNonEmpty(billing["country"], "US")},
		"billing_details[address][line1]":                  {firstNonEmpty(billing["line1"], "3110 Sunset Boulevard")},
		"billing_details[address][city]":                   {firstNonEmpty(billing["city"], "Los Angeles")},
		"billing_details[address][postal_code]":            {firstNonEmpty(billing["postal_code"], "90026")},
		"billing_details[address][state]":                  {firstNonEmpty(billing["state"], "CA")},
		"type":                                             {"gopay"},
		"client_attribution_metadata[checkout_session_id]": {csID},
		"key": {c.cfg.StripePublishableKey},
	}
	resp, err := c.ext.request(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_methods", requestOptions{formBody: form})
	if err != nil {
		return "", err
	}
	if err := resp.require(http.StatusOK, "stripe payment_methods"); err != nil {
		return "", err
	}
	pmID := stringAt(resp.json, "id")
	if !strings.HasPrefix(pmID, "pm_") {
		return "", fmt.Errorf("stripe payment_methods: bad response %s", resp.excerpt(300))
	}
	return pmID, nil
}

func (c *charger) stripeInit(ctx context.Context, csID string) (map[string]any, error) {
	form := url.Values{
		"browser_locale":                                   {"en-US"},
		"browser_timezone":                                 {"Asia/Shanghai"},
		"elements_session_client[client_betas][0]":         {"custom_checkout_server_updates_1"},
		"elements_session_client[client_betas][1]":         {"custom_checkout_manual_approval_1"},
		"elements_session_client[elements_init_source]":    {"custom_checkout"},
		"elements_session_client[referrer_host]":           {"chatgpt.com"},
		"elements_session_client[stripe_js_id]":            {uuid.NewString()},
		"elements_session_client[locale]":                  {"en"},
		"elements_session_client[is_aggregation_expected]": {"false"},
		"key": {c.cfg.StripePublishableKey},
	}
	resp, err := c.ext.request(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_pages/"+csID+"/init", requestOptions{formBody: form})
	if err != nil {
		return nil, err
	}
	if err := resp.require(http.StatusOK, "stripe init"); err != nil {
		return nil, err
	}
	if !containsString(resp.json["payment_method_types"], "gopay") {
		return nil, fmt.Errorf("checkout does not support GoPay: currency=%s payment_method_types=%v", stringAt(resp.json, "currency"), resp.json["payment_method_types"])
	}
	if stringAt(resp.json, "init_checksum") == "" {
		return nil, fmt.Errorf("stripe init: no init_checksum %s", resp.excerpt(300))
	}
	return resp.json, nil
}

func (c *charger) stripeConfirm(ctx context.Context, csID, pmID string) (map[string]any, error) {
	initData, err := c.stripeInit(ctx, csID)
	if err != nil {
		return nil, err
	}
	expectedAmount, _, err := c.resolveExpectedAmount(initData)
	if err != nil {
		return nil, err
	}
	chatGPTReturn := "https://chatgpt.com/checkout/verify?stripe_session_id=" + csID + "&processor_entity=openai_llc&plan_type=plus"
	returnURL := "https://checkout.stripe.com/c/pay/" + csID + "?returned_from_redirect=true&ui_mode=custom&return_url=" + url.QueryEscape(chatGPTReturn)
	form := url.Values{
		"guid":                                   {uuid.NewString()},
		"muid":                                   {uuid.NewString()},
		"sid":                                    {uuid.NewString()},
		"payment_method":                         {pmID},
		"init_checksum":                          {stringAt(initData, "init_checksum")},
		"version":                                {firstNonEmpty(c.cfg.Runtime["version"], "fed52f3bc6")},
		"expected_amount":                        {expectedAmount},
		"expected_payment_method_type":           {"gopay"},
		"return_url":                             {returnURL},
		"elements_session_client[session_id]":    {"elements_session_" + uuid.NewString()[:11]},
		"elements_session_client[locale]":        {"en"},
		"elements_session_client[referrer_host]": {"chatgpt.com"},
		"elements_session_client[is_aggregation_expected]":          {"false"},
		"client_attribution_metadata[client_session_id]":            {uuid.NewString()},
		"client_attribution_metadata[merchant_integration_source]":  {"elements"},
		"client_attribution_metadata[merchant_integration_subtype]": {"payment-element"},
		"client_attribution_metadata[payment_intent_creation_flow]": {"deferred"},
		"key": {c.cfg.StripePublishableKey},
	}
	if value := strings.TrimSpace(c.cfg.Runtime["js_checksum"]); value != "" {
		form.Set("js_checksum", value)
	}
	if value := strings.TrimSpace(c.cfg.Runtime["rv_timestamp"]); value != "" {
		form.Set("rv_timestamp", value)
	}
	resp, err := c.ext.request(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_pages/"+csID+"/confirm", requestOptions{formBody: form})
	if err != nil {
		return nil, err
	}
	if resp.status == http.StatusBadRequest && strings.Contains(strings.ToLower(string(resp.body)), "terms of service") {
		form.Set("consent[terms_of_service]", "accepted")
		resp, err = c.ext.request(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_pages/"+csID+"/confirm", requestOptions{formBody: form})
		if err != nil {
			return nil, err
		}
	}
	if resp.status != http.StatusOK {
		return nil, fmt.Errorf("stripe confirm %d: %s", resp.status, resp.excerpt(500))
	}
	return resp.json, nil
}

func (c *charger) chatGPTApprove(ctx context.Context, csID string) error {
	_, _ = c.cs.request(ctx, http.MethodPost, "https://chatgpt.com/backend-api/sentinel/ping", requestOptions{jsonBody: map[string]any{}})
	resp, err := c.cs.request(ctx, http.MethodPost, "https://chatgpt.com/backend-api/payments/checkout/approve", requestOptions{
		jsonBody: map[string]any{"checkout_session_id": csID, "processor_entity": "openai_llc"},
		headers: http.Header{
			"x-openai-target-path":  []string{"/backend-api/payments/checkout/approve"},
			"x-openai-target-route": []string{"/backend-api/payments/checkout/approve"},
		},
	})
	if err != nil {
		return err
	}
	if err := resp.require(http.StatusOK, "chatgpt approve"); err != nil {
		return err
	}
	if stringAt(resp.json, "result") != "approved" {
		return fmt.Errorf("chatgpt approve: result=%q", stringAt(resp.json, "result"))
	}
	return nil
}

func (c *charger) fetchPMRedirectSnapToken(ctx context.Context, pmURL string) (string, error) {
	if token := regexpSnapToken(pmURL); token != "" {
		return token, nil
	}
	resp, err := c.ext.request(ctx, http.MethodGet, pmURL, requestOptions{noRedirect: true})
	if err != nil {
		return "", err
	}
	if resp.status < 300 || resp.status > 399 {
		return "", fmt.Errorf("pm-redirects: expected redirect, got %d", resp.status)
	}
	location := resp.headers.Get("Location")
	if token := regexpSnapToken(location); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("pm-redirects: no midtrans token in redirect")
}

func (c *charger) followRedirectToMidtrans(ctx context.Context, csID string) (string, error) {
	deadline := time.Now().Add(60 * time.Second)
	lastErr := ""
	for time.Now().Before(deadline) {
		query := url.Values{
			"elements_session_client[client_betas][0]":                        {"custom_checkout_server_updates_1"},
			"elements_session_client[client_betas][1]":                        {"custom_checkout_manual_approval_1"},
			"elements_session_client[elements_init_source]":                   {"custom_checkout"},
			"elements_session_client[referrer_host]":                          {"chatgpt.com"},
			"elements_session_client[session_id]":                             {"elements_session_" + uuid.NewString()[:11]},
			"elements_session_client[stripe_js_id]":                           {uuid.NewString()},
			"elements_session_client[locale]":                                 {"en"},
			"elements_session_client[is_aggregation_expected]":                {"false"},
			"elements_options_client[saved_payment_method][enable_save]":      {"never"},
			"elements_options_client[saved_payment_method][enable_redisplay]": {"never"},
			"key":             {c.cfg.StripePublishableKey},
			"_stripe_version": {"2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1"},
		}
		resp, err := c.ext.request(ctx, http.MethodGet, "https://api.stripe.com/v1/payment_pages/"+csID, requestOptions{query: query})
		if err != nil {
			lastErr = err.Error()
		} else if resp.status == http.StatusOK {
			if stringAt(resp.json, "setup_intent", "status") == "requires_action" {
				pmURL := stringAt(resp.json, "setup_intent", "next_action", "redirect_to_url", "url")
				if pmURL != "" {
					return c.fetchPMRedirectSnapToken(ctx, pmURL)
				}
			}
			lastErr = fmt.Sprintf("setup_intent status=%q payment_status=%q status=%q", stringAt(resp.json, "setup_intent", "status"), stringAt(resp.json, "payment_status"), stringAt(resp.json, "status"))
		} else {
			lastErr = fmt.Sprintf("http %d: %s", resp.status, resp.excerpt(150))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return "", fmt.Errorf("snap_token resolution timeout: %s", lastErr)
}

func regexpSnapToken(value string) string {
	match := regexp.MustCompile(`app\.midtrans\.com/snap/v[14]/redirection/([a-f0-9-]{36})`).FindStringSubmatch(value)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func normalizeTokenization(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultTokenization
	}
	return value
}

func containsString(value any, needle string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if stringAt(map[string]any{"value": item}, "value") == needle {
			return true
		}
	}
	return false
}
