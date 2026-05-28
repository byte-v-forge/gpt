package paymentsvc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/browserfingerprint"
	"github.com/byte-v-forge/common-lib/stringx"
	"github.com/google/uuid"
)

type charger struct {
	cfg              Config
	checkoutProfile  requestProfile
	paymentProfile   requestProfile
	cs               *GptClient
	ext              *httpSession
	countryCode      string
	phone            string
	pin              string
	tokenization     string
	checkoutURL      string
	processorEntity  string
	midtransMerchant string
}

const stripeCheckoutVersion = "2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1"

func (s *Server) newCharger(ctx context.Context, cred credential, phone, countryCode, pin, tokenization string) (*charger, error) {
	checkoutProfile := cred.chatGPTProfile(s.cfg.CheckoutProfile)
	paymentProfile := s.cfg.PaymentProfile
	paymentFingerprint := paymentProfile.fingerprint()
	cs, err := s.newGptClient(ctx, cred, checkoutProfile)
	if err != nil {
		return nil, err
	}
	ext, err := newHTTPSession(paymentProfile.ProxyURL, paymentFingerprint)
	if err != nil {
		cs.close()
		return nil, err
	}
	paymentFingerprint.applyBrowserHeaders(ext.headers)
	return &charger{
		cfg:             s.cfg,
		checkoutProfile: checkoutProfile,
		paymentProfile:  paymentProfile,
		cs:              cs,
		ext:             ext,
		countryCode:     normalizeCountryCode(countryCode),
		phone:           normalizeDigits(phone),
		pin:             strings.TrimSpace(pin),
		tokenization:    normalizeTokenization(tokenization),
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
	return requiresManualPaymentConfirmation(c.tokenization)
}

func requiresManualPaymentConfirmation(tokenization string) bool {
	value := strings.ToLower(strings.TrimSpace(tokenization))
	return value == "false" || value == "qris"
}

func isQRISTokenization(tokenization string) bool {
	return strings.EqualFold(strings.TrimSpace(tokenization), "qris")
}

func (c *charger) createCheckout(ctx context.Context) (string, error) {
	if err := c.cs.setProxy(c.checkoutProfile.ProxyURL); err != nil {
		return "", fmt.Errorf("checkout proxy switch: %w", err)
	}
	c.chatGPTWarmup(ctx)
	plan := c.cfg.CheckoutPlan
	checkoutMode := stringx.FirstNonEmpty(plan["checkout_ui_mode"], "hosted")
	body := map[string]any{
		"entry_point": stringx.FirstNonEmpty(plan["entry_point"], "all_plans_pricing_modal"),
		"plan_name":   stringx.FirstNonEmpty(plan["plan_name"], "chatgptplusplan"),
		"billing_details": map[string]any{
			"country":  stringx.FirstNonEmpty(plan["billing_country"], "ID"),
			"currency": stringx.FirstNonEmpty(plan["billing_currency"], "IDR"),
		},
		"checkout_ui_mode": checkoutMode,
	}
	body["cancel_url"] = stringx.FirstNonEmpty(plan["cancel_url"], "https://chatgpt.com/#pricing")
	if promo := stringx.FirstNonEmpty(plan["promo_campaign_id"], "plus-1-month-free"); promo != "" {
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
	c.processorEntity = stringx.FirstNonEmpty(extractProcessorEntity(resp.json), c.processorEntity, "openai_llc")
	return csID, nil
}

func (c *charger) chatGPTWarmup(ctx context.Context) {
	warmups := []struct {
		method string
		url    string
		accept string
		body   any
	}{
		{http.MethodGet, "https://chatgpt.com/api/auth/session", "application/json", nil},
	}
	for _, item := range warmups {
		select {
		case <-ctx.Done():
			return
		default:
		}
		resp, _ := c.cs.request(ctx, item.method, item.url, requestOptions{
			headers:  http.Header{"Accept": []string{item.accept}},
			jsonBody: item.body,
		})
		if resp != nil && item.url == "https://chatgpt.com/api/auth/session" && resp.status == http.StatusOK {
			if accessToken := stringAt(resp.json, "accessToken"); accessToken != "" {
				c.cs.setHeader("Authorization", "Bearer "+accessToken)
			}
		}
	}
	if cookie := mergeCookieHeaders(c.cs.header("Cookie"), c.cs.cookieHeader("https://chatgpt.com/")); cookie != "" {
		c.cs.setHeader("Cookie", cookie)
	}
}

func (c *charger) processorEntityOrDefault() string {
	return stringx.FirstNonEmpty(c.processorEntity, extractProcessorEntityFromURL(c.checkoutURL), "openai_llc")
}

func (c *charger) checkoutApprovalURL(csID string) string {
	return "https://chatgpt.com/checkout/" + c.processorEntityOrDefault() + "/" + csID
}

func (c *charger) stripeCreatePaymentMethod(ctx context.Context, csID string) (string, error) {
	billing := c.cfg.Billing
	runtimeVersion := stringx.FirstNonEmpty(c.cfg.Runtime["version"], "fed52f3bc6")
	clientSessionID := uuid.NewString()
	form := url.Values{
		"billing_details[name]":                                                    {stringx.FirstNonEmpty(billing["name"], "John Doe")},
		"billing_details[email]":                                                   {stringx.FirstNonEmpty(billing["email"], "buyer@example.com")},
		"billing_details[address][country]":                                        {stringx.FirstNonEmpty(billing["country"], "US")},
		"billing_details[address][line1]":                                          {stringx.FirstNonEmpty(billing["line1"], "3110 Sunset Boulevard")},
		"billing_details[address][city]":                                           {stringx.FirstNonEmpty(billing["city"], "Los Angeles")},
		"billing_details[address][postal_code]":                                    {stringx.FirstNonEmpty(billing["postal_code"], "90026")},
		"billing_details[address][state]":                                          {stringx.FirstNonEmpty(billing["state"], "CA")},
		"type":                                                                     {"gopay"},
		"payment_user_agent":                                                       {fmt.Sprintf("stripe.js/%s; stripe-js-v3/%s; payment-element; deferred-intent", runtimeVersion, runtimeVersion)},
		"referrer":                                                                 {"https://chatgpt.com"},
		"time_on_page":                                                             {fmt.Sprintf("%d", 25000+browserfingerprint.RandomIndex(30001))},
		"client_attribution_metadata[client_session_id]":                           {clientSessionID},
		"client_attribution_metadata[checkout_session_id]":                         {csID},
		"client_attribution_metadata[elements_session_id]":                         {newElementsSessionID()},
		"client_attribution_metadata[elements_session_config_id]":                  {uuid.NewString()},
		"client_attribution_metadata[merchant_integration_source]":                 {"elements"},
		"client_attribution_metadata[merchant_integration_subtype]":                {"payment-element"},
		"client_attribution_metadata[merchant_integration_version]":                {"2021"},
		"client_attribution_metadata[payment_intent_creation_flow]":                {"deferred"},
		"client_attribution_metadata[payment_method_selection_flow]":               {"automatic"},
		"client_attribution_metadata[merchant_integration_additional_elements][0]": {"payment"},
		"client_attribution_metadata[merchant_integration_additional_elements][1]": {"address"},
		"guid":            {uuidHex()},
		"muid":            {uuidHex()},
		"sid":             {uuidHex()},
		"key":             {c.cfg.StripePublishableKey},
		"_stripe_version": {stripeCheckoutVersion},
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
		"key":             {c.cfg.StripePublishableKey},
		"_stripe_version": {stripeCheckoutVersion},
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
	chatGPTReturn := "https://chatgpt.com/checkout/verify?stripe_session_id=" + csID + "&processor_entity=" + c.processorEntityOrDefault() + "&plan_type=plus"
	returnURL := "https://checkout.stripe.com/c/pay/" + csID + "?returned_from_redirect=true&ui_mode=custom&return_url=" + url.QueryEscape(chatGPTReturn)
	clientSessionID := uuid.NewString()
	form := url.Values{
		"guid":                                   {uuidHex()},
		"muid":                                   {uuidHex()},
		"sid":                                    {uuidHex()},
		"payment_method":                         {pmID},
		"init_checksum":                          {stringAt(initData, "init_checksum")},
		"version":                                {stringx.FirstNonEmpty(c.cfg.Runtime["version"], "fed52f3bc6")},
		"expected_amount":                        {expectedAmount},
		"expected_payment_method_type":           {"gopay"},
		"return_url":                             {returnURL},
		"elements_session_client[session_id]":    {newElementsSessionID()},
		"elements_session_client[locale]":        {"en"},
		"elements_session_client[referrer_host]": {"chatgpt.com"},
		"elements_session_client[is_aggregation_expected]":                         {"false"},
		"elements_session_client[elements_init_source]":                            {"custom_checkout"},
		"elements_session_client[client_betas][0]":                                 {"custom_checkout_server_updates_1"},
		"elements_session_client[client_betas][1]":                                 {"custom_checkout_manual_approval_1"},
		"client_attribution_metadata[client_session_id]":                           {clientSessionID},
		"client_attribution_metadata[checkout_session_id]":                         {csID},
		"client_attribution_metadata[merchant_integration_source]":                 {"checkout"},
		"client_attribution_metadata[merchant_integration_subtype]":                {"payment-element"},
		"client_attribution_metadata[merchant_integration_version]":                {"custom"},
		"client_attribution_metadata[payment_intent_creation_flow]":                {"deferred"},
		"client_attribution_metadata[payment_method_selection_flow]":               {"automatic"},
		"client_attribution_metadata[merchant_integration_additional_elements][0]": {"payment"},
		"client_attribution_metadata[merchant_integration_additional_elements][1]": {"address"},
		"consent[terms_of_service]":                                                {"accepted"},
		"key":                                                                      {c.cfg.StripePublishableKey},
		"_stripe_version":                                                          {stripeCheckoutVersion},
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
	if err := c.cs.setProxy(c.paymentProfile.ProxyURL); err != nil {
		return fmt.Errorf("chatgpt approve proxy switch: %w", err)
	}
	headers := c.chatGPTApproveHeaders(csID)
	c.chatGPTSentinelPing(ctx, c.cs)

	var lastStatus int
	var lastBody string
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := c.cs.request(ctx, http.MethodPost, "https://chatgpt.com/backend-api/payments/checkout/approve", requestOptions{
			jsonBody: map[string]any{"checkout_session_id": csID, "processor_entity": c.processorEntityOrDefault()},
			headers:  headers,
		})
		if err != nil {
			return err
		}
		lastStatus = resp.status
		lastBody = resp.excerpt(500)
		result := stringAt(resp.json, "result")
		if resp.status == http.StatusOK && result == "approved" {
			return nil
		}
		if result == "blocked" && attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(2+attempt) * time.Second):
			}
			c.chatGPTSentinelPing(ctx, c.cs)
			continue
		}
		if resp.status == http.StatusForbidden && strings.Contains(strings.ToLower(lastBody), "<html") {
			return fmt.Errorf("chatgpt approve cloudflare challenge 403: %s", lastBody)
		}
		if resp.status != http.StatusOK {
			return fmt.Errorf("chatgpt approve %d: %s", resp.status, lastBody)
		}
		if result == "blocked" {
			return chatGPTApproveBlockedError{status: lastStatus, body: lastBody}
		}
		return fmt.Errorf("chatgpt approve: result=%q body=%s", result, lastBody)
	}
	return chatGPTApproveBlockedError{status: lastStatus, body: lastBody}
}

func (c *charger) chatGPTSentinelPing(ctx context.Context, session *GptClient) {
	if session == nil {
		return
	}
	_, _ = session.request(ctx, http.MethodPost, "https://chatgpt.com/backend-api/sentinel/ping", requestOptions{
		jsonBody: map[string]any{},
		headers:  c.chatGPTAuthHeaders("https://chatgpt.com/"),
	})
}

func (c *charger) chatGPTAuthHeaders(referer string) http.Header {
	fingerprint := c.chatGPTFingerprint()
	headers := http.Header{
		"Accept":       []string{"*/*"},
		"Content-Type": []string{"application/json"},
		"Origin":       []string{"https://chatgpt.com"},
		"Referer":      []string{stringx.FirstNonEmpty(referer, "https://chatgpt.com/")},
	}
	fingerprint.applyBrowserHeaders(headers)
	headers.Set("Accept-Language", gptAcceptLanguage)
	if value := c.cs.header("Authorization"); value != "" {
		headers.Set("Authorization", value)
	}
	headers.Set("oai-device-id", fingerprint.DeviceID)
	headers.Set("oai-language", gptOAILanguage)
	if cookie := mergeCookieHeaders(c.cs.header("Cookie"), c.cs.cookieHeader("https://chatgpt.com/")); cookie != "" {
		cookie = cookieHeaderWithDeviceID(splitCookieHeader(cookie), fingerprint.DeviceID)
		headers.Set("Cookie", cookie)
	}
	return headers
}

func (c *charger) chatGPTApproveHeaders(csID string) http.Header {
	// nb-register style: keep the same ChatGPT browser identity that created the
	// checkout, then move the session to the payment proxy only for approve.
	// Switching to a fresh payment-profile device can make approve return blocked.
	headers := c.chatGPTAuthHeaders(c.checkoutApprovalURL(csID))
	headers.Set("x-openai-target-path", "/backend-api/payments/checkout/approve")
	headers.Set("x-openai-target-route", "/backend-api/payments/checkout/approve")
	return headers
}

func (c *charger) chatGPTFingerprint() browserFingerprint {
	if c != nil && c.cs != nil {
		return c.cs.fingerprint()
	}
	return defaultRequestProfile("checkout").fingerprint()
}

type chatGPTApproveBlockedError struct {
	status int
	body   string
}

func (e chatGPTApproveBlockedError) Error() string {
	return fmt.Sprintf("chatgpt approve: result=\"blocked\" body=%s", e.body)
}

func isChatGPTApproveBlocked(err error) bool {
	if err == nil {
		return false
	}
	var blocked chatGPTApproveBlockedError
	if errors.As(err, &blocked) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "chatgpt approve") && strings.Contains(strings.ToLower(err.Error()), "blocked")
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
			"elements_session_client[session_id]":                             {newElementsSessionID()},
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
			if pmURL := extractRedirectToURL(resp.json); pmURL != "" {
				return c.fetchPMRedirectSnapToken(ctx, pmURL)
			}
			lastErr = fmt.Sprintf("setup_intent status=%q payment_intent status=%q invoice.payment_intent status=%q payment_status=%q status=%q",
				stringAt(resp.json, "setup_intent", "status"),
				stringAt(resp.json, "payment_intent", "status"),
				stringAt(resp.json, "invoice", "payment_intent", "status"),
				stringAt(resp.json, "payment_status"),
				stringAt(resp.json, "status"),
			)
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

func uuidHex() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func newElementsSessionID() string {
	value := uuidHex()
	if len(value) > 11 {
		value = value[:11]
	}
	return "elements_session_" + value
}
