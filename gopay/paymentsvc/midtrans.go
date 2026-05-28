package paymentsvc

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/byte-v-forge/common-lib/stringx"
	"net/http"
	"strings"
	"time"
)

const (
	linkRetryLimit           = 2
	linkRetrySleep           = 12 * time.Second
	statusPollLimit          = 12
	qrisStatusPollLimit      = 300
	midtransChargeRetryLimit = 3
)

func (c *charger) prepareUntilLinking(ctx context.Context, checkoutSessionID, checkoutURL string) (map[string]any, error) {
	csID, state, err := c.prepareCheckout(ctx, checkoutSessionID, checkoutURL, 1)
	if err != nil {
		return nil, err
	}
	if stringAt(state, "checkout_supplied") == "true" {
		return c.prepareCheckoutSessionUntilLinking(ctx, csID)
	}
	var lastErr error
	for attempt := int(intAt(state, "checkout_attempt")); attempt <= 2; attempt++ {
		if attempt > int(intAt(state, "checkout_attempt")) {
			var refreshErr error
			csID, state, refreshErr = c.prepareCheckout(ctx, "", "", attempt)
			if refreshErr != nil {
				return nil, refreshErr
			}
		}
		prepared, err := c.prepareCheckoutSessionUntilLinking(ctx, csID)
		if err == nil {
			prepared["checkout_attempt"] = attempt
			return prepared, nil
		}
		lastErr = err
		if !isChatGPTApproveBlocked(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(2+attempt) * time.Second):
		}
	}
	return nil, lastErr
}

func (c *charger) prepareCheckout(ctx context.Context, checkoutSessionID, checkoutURL string, attempt int) (string, map[string]any, error) {
	checkoutURL = strings.TrimSpace(checkoutURL)
	csID := strings.TrimSpace(checkoutSessionID)
	if csID == "" && checkoutURL != "" {
		csID = extractCheckoutSessionID(map[string]any{"url": checkoutURL})
	}
	if checkoutURL != "" && csID == "" {
		return "", nil, fmt.Errorf("checkout_url does not contain checkout_session_id")
	}
	if csID != "" {
		if !strings.HasPrefix(csID, "cs_") {
			return "", nil, fmt.Errorf("invalid checkout_session_id: %s", csID)
		}
		c.checkoutURL = stringx.FirstNonEmpty(checkoutURL, "https://checkout.stripe.com/c/pay/"+csID)
		c.processorEntity = stringx.FirstNonEmpty(extractProcessorEntityFromURL(checkoutURL), c.processorEntity, "openai_llc")
		return csID, map[string]any{
			"state":             "checkout",
			"cs_id":             csID,
			"processor_entity":  c.processorEntityOrDefault(),
			"checkout_url":      c.checkoutURL,
			"stripe_pk":         c.cfg.StripePublishableKey,
			"checkout_attempt":  attempt,
			"checkout_supplied": "true",
		}, nil
	}

	csID, err := c.createCheckout(ctx)
	if err != nil {
		return "", nil, err
	}
	return csID, map[string]any{
		"state":             "checkout",
		"cs_id":             csID,
		"processor_entity":  c.processorEntityOrDefault(),
		"checkout_url":      c.checkoutURL,
		"stripe_pk":         c.cfg.StripePublishableKey,
		"checkout_attempt":  attempt,
		"checkout_supplied": "false",
	}, nil
}

func (c *charger) prepareCheckoutSessionUntilLinking(ctx context.Context, csID string) (map[string]any, error) {
	pmID, err := c.stripeCreatePaymentMethod(ctx, csID)
	if err != nil {
		return nil, err
	}
	confirmData, err := c.stripeConfirm(ctx, csID, pmID)
	if err != nil {
		return nil, err
	}
	redirectURL := extractRedirectToURL(confirmData)
	var snapToken string
	if redirectURL != "" {
		snapToken, err = c.fetchPMRedirectSnapToken(ctx, redirectURL)
	} else {
		if err = c.chatGPTApprove(ctx, csID); err != nil {
			return nil, err
		}
		snapToken, err = c.followRedirectToMidtrans(ctx, csID)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"state":            "prepared",
		"cs_id":            csID,
		"processor_entity": c.processorEntityOrDefault(),
		"checkout_url":     c.checkoutURL,
		"stripe_pk":        c.cfg.StripePublishableKey,
		"snap_token":       snapToken,
	}, nil
}

func (c *charger) startUntilOTP(ctx context.Context, checkoutSessionID, checkoutURL, otpChannel string) (map[string]any, error) {
	prepared, err := c.prepareUntilLinking(ctx, checkoutSessionID, checkoutURL)
	if err != nil {
		return nil, err
	}
	if c.requiresManualConfirmation() {
		return c.startPreparedQRISToPaymentCharge(ctx, prepared)
	}
	return c.startPreparedLinkingUntilOTP(ctx, prepared, otpChannel)
}

func (c *charger) startPreparedLinkingUntilOTP(ctx context.Context, state map[string]any, otpChannel string) (map[string]any, error) {
	snapToken := stringAt(state, "snap_token")
	if snapToken == "" {
		return nil, fmt.Errorf("prepared payment is missing snap_token")
	}
	if checkoutURL := stringAt(state, "checkout_url"); checkoutURL != "" {
		c.checkoutURL = checkoutURL
	}
	if c.phone == "" {
		return nil, fmt.Errorf("gopay_phone is required before linking")
	}
	if c.countryCode == "" {
		return nil, fmt.Errorf("gopay_country_code is required before linking")
	}
	return c.startLinkingUntilOTP(ctx, snapToken, stringAt(state, "cs_id"), stringAt(state, "stripe_pk"), otpChannel)
}

func (c *charger) midtransLoadTransaction(ctx context.Context, snapToken string) error {
	_, _ = c.ext.request(ctx, http.MethodGet, midtransRedirectionURL(snapToken), requestOptions{
		headers: http.Header{
			"Accept":  []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
			"Referer": []string{"https://pay.openai.com/"},
		},
	})
	resp, err := c.ext.request(ctx, http.MethodGet, "https://app.midtrans.com/snap/v1/transactions/"+snapToken, requestOptions{
		headers: c.midtransHeaders(snapToken, midtransHeaderOptions{source: true}),
	})
	if err != nil {
		return err
	}
	if err := resp.require(http.StatusOK, "midtrans transaction"); err != nil {
		return err
	}
	merchantID := stringx.FirstNonEmpty(stringAt(resp.json, "merchant", "merchant_id"), stringAt(resp.json, "merchant", "id"))
	if merchantID != "" {
		c.midtransMerchant = merchantID
	}
	_ = c.midtransWarmSnap(ctx, snapToken)
	return nil
}

func (c *charger) midtransWarmSnap(ctx context.Context, snapToken string) error {
	_, _ = c.ext.request(ctx, http.MethodPost, "https://app.midtrans.com/snap/v1/promos/"+snapToken+"/search", requestOptions{
		headers: c.midtransHeaders(snapToken, midtransHeaderOptions{source: true, origin: true}),
	})
	_, _ = c.ext.request(ctx, http.MethodGet, "https://app.midtrans.com/snap/v3/experiment", requestOptions{
		query:   mapValues("id", snapToken),
		headers: c.midtransHeaders(snapToken, midtransHeaderOptions{source: true}),
	})
	return nil
}

func (c *charger) midtransInitLinking(ctx context.Context, snapToken string) (string, error) {
	url := "https://app.midtrans.com/snap/v3/accounts/" + snapToken + "/linking"
	body := map[string]any{"type": "gopay", "country_code": c.countryCode, "phone_number": c.phone}
	baseHeaders := c.midtransHeaders(snapToken, midtransHeaderOptions{jsonBody: true})
	authHeaders := c.midtransHeaders(snapToken, midtransHeaderOptions{jsonBody: true, auth: true})
	lastErr := ""
	bypassTried := false
	for range linkRetryLimit + 1 {
		headers, err := c.paymentAttemptHeaders(authHeaders)
		if err != nil {
			return "", err
		}
		resp, err := c.ext.request(ctx, http.MethodPost, url, requestOptions{jsonBody: body, headers: headers})
		if err != nil {
			return "", err
		}
		if ref := parseLinkingReference(resp); ref != "" {
			return ref, nil
		}
		if resp.status == http.StatusNotAcceptable {
			lastErr = resp.excerpt(200)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(linkRetrySleep):
			}
			continue
		}
		if !bypassTried && linkingRateLimited(resp) {
			bypassTried = true
			bypassHeaders, err := c.paymentAttemptHeaders(baseHeaders)
			if err != nil {
				return "", err
			}
			bypassResp, err := c.ext.request(ctx, http.MethodPost, url, requestOptions{jsonBody: body, headers: bypassHeaders})
			if err != nil {
				return "", err
			}
			if ref := parseLinkingReference(bypassResp); ref != "" {
				return ref, nil
			}
			return "", fmt.Errorf("midtrans linking bypass failed status=%d body=%s", bypassResp.status, bypassResp.excerpt(300))
		}
		return "", fmt.Errorf("midtrans linking unexpected status=%d body=%s", resp.status, resp.excerpt(300))
	}
	return "", fmt.Errorf("midtrans linking exhausted retries: %s", lastErr)
}

func parseLinkingReference(resp *httpResult) string {
	if resp == nil || resp.status != http.StatusCreated {
		return ""
	}
	match := regexpReference.FindStringSubmatch(stringAt(resp.json, "activation_link_url"))
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

var regexpReference = regexpMust(`reference=([a-f0-9-]{36})`)

func linkingRateLimited(resp *httpResult) bool {
	if resp == nil {
		return false
	}
	if resp.status == http.StatusTooManyRequests {
		return true
	}
	text := strings.ToLower(string(resp.body))
	return strings.Contains(text, "technical error") || strings.Contains(text, "too many") || strings.Contains(text, "rate limit") || strings.Contains(text, "rate_limit")
}

type midtransHeaderOptions struct {
	jsonBody bool
	source   bool
	auth     bool
	origin   bool
}

func (c *charger) midtransHeaders(snapToken string, opts midtransHeaderOptions) http.Header {
	headers := http.Header{
		"Accept":  []string{"application/json"},
		"Referer": []string{midtransRedirectionURL(snapToken)},
	}
	if opts.jsonBody {
		headers.Set("Content-Type", "application/json")
		opts.origin = true
	}
	if opts.origin {
		headers.Set("Origin", "https://app.midtrans.com")
	}
	if opts.source {
		headers.Set("x-source", "snap")
		headers.Set("x-source-app-type", "redirection")
		headers.Set("x-source-version", "2.3.0")
	}
	if opts.auth {
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.cfg.MidtransClientID+":")))
	}
	return headers
}

func midtransRedirectionURL(snapToken string) string {
	return "https://app.midtrans.com/snap/v4/redirection/" + snapToken
}

func extractRedirectToURL(payload map[string]any) string {
	if value := redirectToURLFromAction(objectAt(payload, "next_action")); value != "" {
		return value
	}
	if value := redirectToURLFromIntent(objectAt(payload, "setup_intent")); value != "" {
		return value
	}
	if value := redirectToURLFromIntent(objectAt(payload, "payment_intent")); value != "" {
		return value
	}
	if value := redirectToURLFromIntent(objectAt(payload, "invoice", "payment_intent")); value != "" {
		return value
	}
	return ""
}

func redirectToURLFromIntent(intent map[string]any) string {
	return redirectToURLFromAction(objectAt(intent, "next_action"))
}

func redirectToURLFromAction(action map[string]any) string {
	if stringAt(action, "type") != "redirect_to_url" {
		return ""
	}
	return stringAt(action, "redirect_to_url", "url")
}

func objectAt(value map[string]any, path ...string) map[string]any {
	current := value
	for _, key := range path {
		if current == nil {
			return nil
		}
		next, _ := current[key].(map[string]any)
		current = next
	}
	return current
}
