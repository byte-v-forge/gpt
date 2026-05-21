package paymentsvc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (c *charger) gopayHeaders(locale string) http.Header {
	headers := http.Header{
		"Accept":  []string{"application/json, text/plain, */*"},
		"Origin":  []string{"https://merchants-gws-app.gopayapi.com"},
		"Referer": []string{"https://merchants-gws-app.gopayapi.com/"},
	}
	headers.Set("Content-Type", "application/json")
	if locale != "" {
		headers.Set("x-user-locale", locale)
	}
	return headers
}

func (c *charger) gopayPINHeaders(linking bool) http.Header {
	headers := c.gopayHeaders("")
	if linking {
		headers.Set("Origin", "https://pin-web-client.gopayapi.com")
		headers.Set("Referer", "https://pin-web-client.gopayapi.com/")
		headers.Set("x-user-locale", c.cfg.PINLocale)
		headers.Set("x-appversion", "1.0.0")
		headers.Set("x-correlation-id", uuid.NewString())
		headers.Set("x-is-mobile", "false")
		headers.Set("x-platform", c.cfg.BrowserPlatform)
	}
	headers.Set("x-request-id", uuid.NewString())
	return headers
}

func (c *charger) startLinkingUntilOTP(ctx context.Context, snapToken, csID, stripePK, otpChannel string) (map[string]any, error) {
	otpChannel = normalizeOTPChannel(otpChannel)
	if err := c.midtransLoadTransaction(ctx, snapToken); err != nil {
		return nil, err
	}
	referenceID, err := c.midtransInitLinking(ctx, snapToken)
	if err != nil {
		return nil, err
	}
	if err := c.gopayValidateReference(ctx, referenceID); err != nil {
		return nil, err
	}
	issued := time.Now().Unix()
	consent, err := c.gopayUserConsent(ctx, referenceID)
	if err != nil {
		return nil, err
	}
	otpRequired := linkingConsentRequiresOTP(consent)
	if otpRequired && otpChannel == "sms" {
		_, _ = c.gopayResendOTP(ctx, referenceID)
	}
	state := map[string]any{
		"cs_id":             csID,
		"checkout_url":      c.checkoutURL,
		"stripe_pk":         stripePK,
		"snap_token":        snapToken,
		"reference_id":      referenceID,
		"issued_after_unix": issued,
		"otp_channel":       otpChannel,
		"otp_required":      otpRequired,
	}
	if !otpRequired {
		chargeData, err := c.midtransCreateChargeData(ctx, snapToken)
		if err != nil {
			return nil, err
		}
		for key, value := range chargeData {
			state[key] = value
		}
		state["state"] = "awaiting_manual_confirmation"
	}
	return state, nil
}

func (c *charger) resendLinkingOTP(ctx context.Context, state map[string]any) (map[string]any, error) {
	referenceID := stringAt(state, "reference_id")
	if referenceID == "" {
		return nil, fmt.Errorf("prepared payment is missing reference_id")
	}
	if _, err := c.gopayResendOTP(ctx, referenceID); err != nil {
		return nil, err
	}
	next := cloneMap(state)
	next["issued_after_unix"] = time.Now().Unix()
	next["otp_required"] = true
	next["otp_resend_count"] = intAt(state, "otp_resend_count") + 1
	return next, nil
}

func (c *charger) gopayValidateReference(ctx context.Context, referenceID string) error {
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/validate-reference", requestOptions{
		jsonBody: map[string]any{"reference_id": referenceID},
		headers:  c.gopayHeaders(""),
	})
	if err != nil {
		return err
	}
	if err := resp.require(http.StatusOK, "validate-reference"); err != nil {
		return err
	}
	if !boolAt(resp.json, "success") {
		return fmt.Errorf("validate-reference failed: %s", resp.excerpt(500))
	}
	return nil
}

func (c *charger) gopayUserConsent(ctx context.Context, referenceID string) (map[string]any, error) {
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/user-consent", requestOptions{
		jsonBody: map[string]any{"reference_id": referenceID},
		headers:  c.gopayHeaders(c.cfg.BrowserLocale),
	})
	if err != nil {
		return nil, err
	}
	if err := resp.require(http.StatusOK, "user-consent"); err != nil {
		return nil, err
	}
	if !boolAt(resp.json, "success") {
		return nil, fmt.Errorf("user-consent failed: %s", resp.excerpt(500))
	}
	return resp.json, nil
}

func (c *charger) gopayResendOTP(ctx context.Context, referenceID string) (map[string]any, error) {
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/resend-otp", requestOptions{
		jsonBody: map[string]any{"reference_id": referenceID},
		headers:  c.gopayHeaders(c.cfg.BrowserLocale),
	})
	if err != nil {
		return nil, err
	}
	if resp.status < 200 || resp.status >= 300 {
		return nil, fmt.Errorf("resend-otp %d: %s", resp.status, resp.excerpt(300))
	}
	return resp.json, nil
}

func (c *charger) gopayValidateOTP(ctx context.Context, referenceID, otp string) (string, string, error) {
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/validate-otp", requestOptions{
		jsonBody: map[string]any{"reference_id": referenceID, "otp": strings.TrimSpace(otp)},
		headers:  c.gopayHeaders(c.cfg.BrowserLocale),
	})
	if err != nil {
		return "", "", err
	}
	if resp.status != http.StatusOK {
		return "", "", fmt.Errorf("validate-otp %d: %s", resp.status, resp.excerpt(400))
	}
	if !boolAt(resp.json, "success") {
		return "", "", fmt.Errorf("validate-otp failed: %s", resp.excerpt(400))
	}
	challengeID := stringAt(resp.json, "data", "challenge", "action", "value", "challenge_id")
	clientID := stringAt(resp.json, "data", "challenge", "action", "value", "client_id")
	if challengeID == "" || clientID == "" {
		return "", "", fmt.Errorf("validate-otp: missing challenge details")
	}
	return challengeID, clientID, nil
}

func (c *charger) tokenizePIN(ctx context.Context, challengeID, clientID string, linking bool) (string, error) {
	if strings.TrimSpace(c.pin) == "" {
		return "", fmt.Errorf("pin is required")
	}
	resp, err := c.ext.request(ctx, http.MethodPost, "https://customer.gopayapi.com/api/v1/users/pin/tokens/nb", requestOptions{
		jsonBody: map[string]any{"pin": c.pin, "challenge_id": challengeID, "client_id": clientID},
		headers:  c.gopayPINHeaders(linking),
	})
	if err != nil {
		return "", err
	}
	if resp.status == http.StatusBadRequest || resp.status == http.StatusUnauthorized || resp.status == http.StatusForbidden {
		return "", fmt.Errorf("PIN rejected: %s", resp.excerpt(300))
	}
	if resp.status >= 400 {
		return "", fmt.Errorf("pin tokenize %d: %s", resp.status, resp.excerpt(500))
	}
	token := stringAt(resp.data(), "token")
	if token == "" {
		return "", fmt.Errorf("pin tokenize: no token in response")
	}
	return token, nil
}

func (c *charger) gopayValidatePIN(ctx context.Context, referenceID, pinToken string) error {
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/validate-pin", requestOptions{
		jsonBody: map[string]any{"reference_id": referenceID, "token": pinToken},
		headers:  c.gopayHeaders(c.cfg.BrowserLocale),
	})
	if err != nil {
		return err
	}
	if err := resp.require(http.StatusOK, "validate-pin"); err != nil {
		return err
	}
	if !boolAt(resp.json, "success") {
		return fmt.Errorf("validate-pin failed: %s", resp.excerpt(500))
	}
	return nil
}

func (c *charger) midtransCreateChargeData(ctx context.Context, snapToken string) (map[string]any, error) {
	resp, err := c.ext.request(ctx, http.MethodPost, "https://app.midtrans.com/snap/v2/transactions/"+snapToken+"/charge", requestOptions{
		jsonBody: map[string]any{"payment_type": "gopay", "tokenization": c.tokenization, "promo_details": nil},
		headers:  c.midtransHeaders(snapToken, midtransHeaderOptions{jsonBody: true, source: true}),
	})
	if err != nil {
		return nil, err
	}
	if err := resp.require(http.StatusOK, "midtrans charge"); err != nil {
		return nil, err
	}
	if message := midtransChargeDenial(resp.json); message != "" {
		return nil, fmt.Errorf("%s", message)
	}
	chargeRef := extractMidtransChargeReference(resp.json)
	if chargeRef == "" {
		return nil, fmt.Errorf("midtrans charge: no reference in response")
	}
	data := map[string]any{"charge_ref": chargeRef, "snap_token": snapToken}
	for key, value := range midtransChargeURLs(resp.json) {
		data[key] = value
	}
	return data, nil
}

func (c *charger) gopayPaymentValidate(ctx context.Context, chargeRef string) error {
	query := url.Values{"reference_id": []string{chargeRef}}
	var last *httpResult
	for range 8 {
		resp, err := c.ext.request(ctx, http.MethodGet, "https://gwa.gopayapi.com/v1/payment/validate", requestOptions{
			query:   query,
			headers: c.gopayHeaders(""),
		})
		if err != nil {
			return err
		}
		last = resp
		if resp.status == http.StatusOK && boolAt(resp.json, "success") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}
	return fmt.Errorf("payment/validate failed after retries: %d %s", last.status, last.excerpt(250))
}

func (c *charger) gopayPaymentConfirm(ctx context.Context, chargeRef string) (string, string, error) {
	query := url.Values{"reference_id": []string{chargeRef}}
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/payment/confirm", requestOptions{
		query:    query,
		jsonBody: map[string]any{"payment_instructions": []any{}},
		headers:  c.gopayHeaders(""),
	})
	if err != nil {
		return "", "", err
	}
	if err := resp.require(http.StatusOK, "payment/confirm"); err != nil {
		return "", "", err
	}
	if !boolAt(resp.json, "success") {
		return "", "", fmt.Errorf("payment/confirm failed: %s", resp.excerpt(500))
	}
	challengeID := stringAt(resp.json, "data", "challenge", "action", "value", "challenge_id")
	clientID := stringAt(resp.json, "data", "challenge", "action", "value", "client_id")
	if challengeID == "" || clientID == "" {
		return "", "", fmt.Errorf("payment/confirm missing challenge")
	}
	return challengeID, clientID, nil
}

func (c *charger) gopayPaymentProcess(ctx context.Context, chargeRef, pinToken string) error {
	query := url.Values{"reference_id": []string{chargeRef}}
	resp, err := c.ext.request(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/payment/process", requestOptions{
		query: query,
		jsonBody: map[string]any{"challenge": map[string]any{
			"type":  "GOPAY_PIN_CHALLENGE",
			"value": map[string]any{"pin_token": pinToken},
		}},
		headers: c.gopayHeaders(""),
	})
	if err != nil {
		return err
	}
	if err := resp.require(http.StatusOK, "payment/process"); err != nil {
		return err
	}
	if !boolAt(resp.json, "success") || stringAt(resp.json, "data", "next_action") != "payment-success" {
		return fmt.Errorf("payment/process failed: %s", resp.excerpt(500))
	}
	return nil
}

func (c *charger) midtransPollStatus(ctx context.Context, snapToken string) (map[string]any, error) {
	var last string
	for range statusPollLimit {
		resp, err := c.ext.request(ctx, http.MethodGet, "https://app.midtrans.com/snap/v1/transactions/"+snapToken+"/status", requestOptions{
			headers: c.midtransHeaders(snapToken, midtransHeaderOptions{source: true}),
		})
		if err != nil {
			last = err.Error()
		} else if resp.status == http.StatusOK {
			status := stringAt(resp.json, "transaction_status")
			statusCode := stringAt(resp.json, "status_code")
			if status == "settlement" || status == "capture" || statusCode == "200" {
				return resp.json, nil
			}
			if status == "deny" || status == "cancel" || status == "expire" || status == "failure" {
				return nil, fmt.Errorf("midtrans transaction failed: %s", resp.excerpt(500))
			}
			last = fmt.Sprintf("status=%q status_code=%q", status, statusCode)
		} else {
			last = fmt.Sprintf("http %d: %s", resp.status, resp.excerpt(150))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return map[string]any{}, fmt.Errorf("midtrans status poll timeout: %s", last)
}

func (c *charger) followMidtransFinishRedirect(ctx context.Context, state map[string]any, midtransStatus map[string]any) string {
	finishURL := firstNonEmpty(stringAt(midtransStatus, "finish_redirect_url"), stringAt(midtransStatus, "finish_200_redirect_url"), stringAt(state, "finish_redirect_url"), stringAt(state, "finish_200_redirect_url"))
	if finishURL == "" {
		return ""
	}
	_, _ = c.ext.request(ctx, http.MethodGet, finishURL, requestOptions{
		headers: http.Header{
			"Accept":  []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
			"Referer": []string{midtransRedirectionURL(stringAt(state, "snap_token"))},
		},
	})
	return finishURL
}

func (c *charger) chatGPTVerify(ctx context.Context, csID string) map[string]any {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := c.cs.request(ctx, http.MethodGet, "https://chatgpt.com/checkout/verify", requestOptions{
			query: url.Values{"stripe_session_id": []string{csID}, "processor_entity": []string{"openai_llc"}, "plan_type": []string{"plus"}},
		})
		if err == nil && resp.status == http.StatusOK {
			return map[string]any{"state": "succeeded", "cs_id": csID}
		}
		select {
		case <-ctx.Done():
			return map[string]any{"state": "verify_timeout", "cs_id": csID}
		case <-time.After(2 * time.Second):
		}
	}
	return map[string]any{"state": "verify_timeout", "cs_id": csID}
}

func (c *charger) completeAfterOTPUntilManualConfirmation(ctx context.Context, state map[string]any, otp string) (map[string]any, error) {
	referenceID := stringAt(state, "reference_id")
	snapToken := stringAt(state, "snap_token")
	if referenceID == "" || snapToken == "" {
		return nil, fmt.Errorf("payment flow state is missing reference_id/snap_token")
	}
	if strings.TrimSpace(otp) == "" {
		return nil, fmt.Errorf("OTP not provided")
	}
	challengeID, clientID, err := c.gopayValidateOTP(ctx, referenceID, otp)
	if err != nil {
		return nil, err
	}
	pinToken, err := c.tokenizePIN(ctx, challengeID, clientID, true)
	if err != nil {
		return nil, err
	}
	if err := c.gopayValidatePIN(ctx, referenceID, pinToken); err != nil {
		return nil, err
	}
	chargeData, err := c.midtransCreateChargeData(ctx, snapToken)
	if err != nil {
		return nil, err
	}
	next := cloneMap(state)
	for key, value := range chargeData {
		next[key] = value
	}
	next["state"] = "awaiting_manual_confirmation"
	return next, nil
}

func (c *charger) completeAfterManualConfirmation(ctx context.Context, state map[string]any) (map[string]any, error) {
	snapToken := stringAt(state, "snap_token")
	csID := stringAt(state, "cs_id")
	chargeRef := stringAt(state, "charge_ref")
	if chargeRef == "" || snapToken == "" {
		return nil, fmt.Errorf("payment flow state is missing charge_ref/snap_token")
	}
	var midtransStatus map[string]any
	var err error
	if c.requiresManualConfirmation() {
		midtransStatus, err = c.midtransPollStatus(ctx, snapToken)
		if err != nil {
			return nil, err
		}
		c.followMidtransFinishRedirect(ctx, state, midtransStatus)
	} else {
		if err := c.gopayPaymentValidate(ctx, chargeRef); err != nil {
			return nil, err
		}
		challengeID, clientID, err := c.gopayPaymentConfirm(ctx, chargeRef)
		if err != nil {
			return nil, err
		}
		pinToken, err := c.tokenizePIN(ctx, challengeID, clientID, false)
		if err != nil {
			return nil, err
		}
		if err := c.gopayPaymentProcess(ctx, chargeRef, pinToken); err != nil {
			return nil, err
		}
		midtransStatus, err = c.midtransPollStatus(ctx, snapToken)
		if err != nil {
			return nil, err
		}
	}
	result := map[string]any{"state": "succeeded", "snap_token": snapToken, "charge_ref": chargeRef, "midtrans_status": stringAt(midtransStatus, "transaction_status")}
	if csID != "" {
		result = c.chatGPTVerify(ctx, csID)
		result["snap_token"] = snapToken
		result["charge_ref"] = chargeRef
		result["midtrans_status"] = stringAt(midtransStatus, "transaction_status")
	}
	for _, key := range []string{"deeplink_url", "qr_code_url", "finish_redirect_url", "finish_200_redirect_url"} {
		result[key] = stringAt(state, key)
	}
	return result, nil
}

func (c *charger) completeAfterOTP(ctx context.Context, state map[string]any, otp string) (map[string]any, error) {
	next, err := c.completeAfterOTPUntilManualConfirmation(ctx, state, otp)
	if err != nil {
		return nil, err
	}
	return c.completeAfterManualConfirmation(ctx, next)
}

func linkingConsentRequiresOTP(value any) bool {
	found, ok := findBoolField(value, map[string]bool{"otp_required": true, "is_otp_required": true, "requires_otp": true, "need_otp": true, "needs_otp": true})
	if ok {
		return found
	}
	return true
}

func findBoolField(value any, names map[string]bool) (bool, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if names[strings.ToLower(key)] {
				if b, ok := item.(bool); ok {
					return b, true
				}
			}
			if found, ok := findBoolField(item, names); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range typed {
			if found, ok := findBoolField(item, names); ok {
				return found, true
			}
		}
	}
	return false, false
}

func midtransChargeDenial(data map[string]any) string {
	status := stringAt(data, "transaction_status")
	fraud := stringAt(data, "fraud_status")
	if status != "deny" && status != "cancel" && status != "expire" && status != "failure" && fraud != "deny" {
		return ""
	}
	return "midtrans charge denied: " + jsonExcerpt(data, 500)
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
