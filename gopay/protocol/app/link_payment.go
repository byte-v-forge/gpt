package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/byte-v-forge/gpt/gopay/protocol"
)

const (
	DefaultCurrency                   = "IDR"
	defaultServiceID                  = "1001"
	midtransMerchantTransferServiceID = "1002"
)

type LinkPaymentOptions struct {
	PaymentLink    string
	PIN            string
	AmountValue    int64
	AmountCurrency string
	BodyLimit      int
}

type StepResult struct {
	Label        string
	StatusCode   int
	ResponseText string
	ErrorMessage string
}

type LinkPaymentResult struct {
	Success      bool
	ErrorMessage string
	PaymentID    string
	Status       string
	Steps        []StepResult
}

func RunLinkPayment(ctx context.Context, client *Client, options LinkPaymentOptions) (LinkPaymentResult, error) {
	if client == nil {
		err := fmt.Errorf("gopay app client is nil")
		return LinkPaymentResult{ErrorMessage: err.Error()}, err
	}
	if strings.TrimSpace(options.PIN) == "" {
		err := fmt.Errorf("pin is required")
		return LinkPaymentResult{ErrorMessage: err.Error()}, err
	}
	paymentRef, err := ExtractMidtransPaymentRef(options.PaymentLink)
	if err != nil {
		return LinkPaymentResult{ErrorMessage: err.Error()}, err
	}
	recorder := stepRecorder{limit: normalizeBodyLimit(options.BodyLimit)}
	detail, err := recorder.call("payment_detail", func() (*protocol.Response, error) {
		return client.Get(ctx, CustomerBaseURL+"/customers/v1/payments/"+paymentRef+"?fetch_promotion_details=false", http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentRef, "", err), err
	}
	order, err := ExtractPaymentOrder(detail)
	if err != nil {
		return recorder.result(paymentRef, "", err), err
	}
	order["payment_id"] = firstNonEmpty(protocol.StringAt(order, "payment_id"), paymentRef)
	return RunPaymentOrder(ctx, client, order, options.PIN, recorder)
}

func RunPaymentOrder(ctx context.Context, client *Client, order map[string]any, pin string, recorder stepRecorder) (LinkPaymentResult, error) {
	paymentID := strings.TrimSpace(protocol.StringAt(order, "payment_id"))
	if paymentID == "" {
		err := fmt.Errorf("payment_id is required")
		return recorder.result("", "", err), err
	}
	checkout, err := recorder.call("checkout_list", func() (*protocol.Response, error) {
		return client.Post(ctx, CustomerBaseURL+"/v2/customer/payment-options/checkout/list", BuildCheckoutBody(order, midtransMerchantTransferServiceID), http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	paymentToken, err := ExtractPaymentToken(checkout)
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	_, err = recorder.call("promotions_evaluate", func() (*protocol.Response, error) {
		return client.Post(ctx, CustomerBaseURL+"/v1/promotions/evaluate", BuildPromotionsEvaluateBody(order, paymentToken), http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	capturePaymentToken, err := RandomizePaymentOptionToken(paymentToken)
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	capture1, err := recorder.call("capture1", func() (*protocol.Response, error) {
		headers := http.Header{"Idempotency-Key": []string{uuid.NewString()}}
		return client.Request(ctx, http.MethodPatch, CustomerBaseURL+"/v3/payments/"+paymentID+"/capture", BuildCaptureBody(order, capturePaymentToken, "", "", ""), headers, http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	challengeID, clientID, err := ExtractChallenge(capture1)
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	_, err = recorder.call("last_used", func() (*protocol.Response, error) {
		return client.Put(ctx, CustomerBaseURL+"/v1/customer/payment-options/settings/last-used", map[string]any{"token": paymentToken}, http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	_, _ = recorder.call("pin_page", func() (*protocol.Response, error) {
		return client.Get(ctx, CustomerBaseURL+"/api/v2/challenges/"+challengeID+"/pin-page")
	})
	pinResp, err := recorder.call("pin_tokens", func() (*protocol.Response, error) {
		return client.Post(ctx, CustomerBaseURL+"/api/v1/users/pin/tokens", map[string]any{
			"pin":          pin,
			"client_id":    clientID,
			"challenge_id": challengeID,
		}, http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	pinToken, err := ExtractPinToken(pinResp)
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	capture2, err := recorder.call("capture2", func() (*protocol.Response, error) {
		headers := http.Header{"Idempotency-Key": []string{uuid.NewString()}}
		return client.Request(ctx, http.MethodPatch, CustomerBaseURL+"/v3/payments/"+paymentID+"/capture", BuildCaptureBody(order, capturePaymentToken, pinToken, challengeID, clientID), headers, http.StatusOK)
	})
	if err != nil {
		return recorder.result(paymentID, "", err), err
	}
	status := strings.ToUpper(protocol.StringAt(capture2.Data(), "status"))
	if status != "PAID" {
		err := fmt.Errorf("payment not paid: status=%s", status)
		return recorder.result(paymentID, status, err), err
	}
	return LinkPaymentResult{Success: true, PaymentID: paymentID, Status: status, Steps: recorder.steps}, nil
}

var midtransPaymentRefRE = regexp.MustCompile(`A[0-9]{12,}[A-Za-z0-9]+ID`)

func ExtractMidtransPaymentRef(paymentLink string) (string, error) {
	match := midtransPaymentRefRE.FindString(strings.TrimSpace(paymentLink))
	if match == "" {
		return "", fmt.Errorf("midtrans payment reference missing from payment_link")
	}
	return match, nil
}

func BuildCheckoutBody(order map[string]any, serviceID string) map[string]any {
	merchantID := OrderMerchantID(order)
	return map[string]any{
		"intent": firstNonEmpty(protocol.StringAt(order, "payment_intent"), protocol.StringAt(order, "intent"), "EWALLET_QR"),
		"order_pricing": map[string]any{
			"payment_method_specific_pricing": []any{},
			"default_amount": map[string]any{
				"amount": OrderAmount(order),
			},
		},
		"selected_options_tokens": []any{},
		"merchant_id":             merchantID,
		"frontend_overrides": map[string]any{
			"offline_methods":        []any{},
			"payment_method_rollout": []any{},
			"exclude_paylater":       false,
		},
		"service_id": firstNonEmpty(serviceID, OrderServiceID(order), defaultServiceID),
		"metadata":   map[string]any{"merchant_id": merchantID},
	}
}

func BuildPromotionsEvaluateBody(order map[string]any, paymentToken string) map[string]any {
	return map[string]any{
		"order_id": protocol.StringAt(order, "payment_id"),
		"payment_instructions": []any{map[string]any{
			"token": paymentToken,
			"amount": map[string]any{
				"value":    OrderAmount(order),
				"currency": OrderCurrency(order),
			},
		}},
		"transaction_type": "MERCHANT_TRANSACTION",
	}
}

func BuildCaptureBody(order map[string]any, paymentToken string, pinToken string, challengeID string, clientID string) map[string]any {
	var challenge any
	if pinToken != "" {
		challenge = map[string]any{
			"action": nil,
			"value":  map[string]any{"pin_token": pinToken},
			"type":   "GOPAY_PIN_CHALLENGE",
			"metadata": map[string]any{
				"challenge_id": challengeID,
				"client_id":    clientID,
			},
		}
	}
	return map[string]any{
		"payment_instructions": []any{map[string]any{
			"token":           paymentToken,
			"amount":          map[string]any{"value": OrderAmount(order), "currency": OrderCurrency(order)},
			"admin_fee_token": nil,
		}},
		"applied_promo_code": []string{"NO_PROMO_APPLIED"},
		"description":        nil,
		"payment_method":     nil,
		"channel_type":       nil,
		"additional_data":    nil,
		"challenge":          challenge,
		"metadata":           nil,
		"checksum":           nil,
		"order_signature":    nil,
	}
}

func ExtractPaymentOrder(response *protocol.Response) (map[string]any, error) {
	if response == nil {
		return nil, fmt.Errorf("payment detail response is nil")
	}
	data := response.Data()
	if len(data) == 0 {
		return nil, fmt.Errorf("payment detail missing order data")
	}
	return map[string]any(data), nil
}

func ExtractPaymentToken(response *protocol.Response) (string, error) {
	for _, item := range paymentOptionItems(response) {
		if token := strings.TrimSpace(protocol.StringAt(item, "token")); token != "" {
			return token, nil
		}
	}
	return "", fmt.Errorf("payment option token missing")
}

func RandomizePaymentOptionToken(token string) (string, error) {
	payload, err := DecodePaymentOptionToken(token)
	if err != nil {
		return "", err
	}
	payload["payment_option_id"] = uuid.NewString()
	raw, err := protocol.CompactJSON(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func DecodePaymentOptionToken(token string) (map[string]any, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("payment option token missing")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(token)
	}
	if err != nil {
		return nil, fmt.Errorf("payment option token is not decodable JSON: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("payment option token is not JSON: %w", err)
	}
	if strings.TrimSpace(protocol.StringAt(payload, "payment_option_id")) == "" {
		return nil, fmt.Errorf("payment option token missing payment_option_id")
	}
	return payload, nil
}

func ExtractChallenge(response *protocol.Response) (string, string, error) {
	data := response.Data()
	challengeID := protocol.StringAt(data, "challenge", "action", "value", "challenge_id")
	clientID := protocol.StringAt(data, "challenge", "action", "value", "client_id")
	if challengeID == "" {
		challengeID = protocol.StringAt(data, "challenge", "metadata", "challenge_id")
	}
	if clientID == "" {
		clientID = protocol.StringAt(data, "challenge", "metadata", "client_id")
	}
	if challengeID == "" || clientID == "" {
		return "", "", fmt.Errorf("capture challenge missing")
	}
	return challengeID, clientID, nil
}

func ExtractPinToken(response *protocol.Response) (string, error) {
	data := response.Data()
	token := firstNonEmpty(protocol.StringAt(data, "token"), protocol.StringAt(data, "pin_token"))
	if token == "" {
		token = protocol.StringAt(response.Payload, "token")
	}
	if token == "" {
		return "", fmt.Errorf("pin token missing")
	}
	return token, nil
}

func OrderAmount(order map[string]any) int64 {
	if value := protocol.IntAt(order, "amount", "value"); value > 0 {
		return value
	}
	return protocol.IntAt(order, "additional_data", "aspiqr_information_v2", "transaction_details", "amount", "value")
}

func OrderCurrency(order map[string]any) string {
	return firstNonEmpty(protocol.StringAt(order, "amount", "currency"), DefaultCurrency)
}

func OrderServiceID(order map[string]any) string {
	return firstNonEmpty(
		protocol.StringAt(order, "service_id"),
		protocol.StringAt(order, "payment_widget_metadata", "service_id"),
		protocol.StringAt(order, "metadata", "service_id"),
		defaultServiceID,
	)
}

func OrderMerchantID(order map[string]any) string {
	return firstNonEmpty(
		protocol.StringAt(order, "payment_widget_metadata", "merchant_id"),
		protocol.StringAt(order, "merchant_information", "merchant_id"),
		protocol.StringAt(order, "merchant_information", "id"),
		protocol.StringAt(order, "additional_data", "merchant_information", "merchant_id"),
		protocol.StringAt(order, "additional_data", "merchant_information", "id"),
		protocol.StringAt(order, "additional_data", "aspiqr_information", "merchant_id"),
	)
}

func paymentOptionItems(response *protocol.Response) []map[string]any {
	if response == nil {
		return nil
	}
	var out []map[string]any
	collectItems := func(value any) {
		items, ok := value.([]any)
		if !ok {
			return
		}
		for _, item := range items {
			if obj, ok := item.(map[string]any); ok {
				out = append(out, obj)
			}
		}
	}
	data := response.Data()
	collectItems(data["selected_options"])
	collectItems(data["payment_options"])
	collectItems(response.Payload["selected_options"])
	collectItems(response.Payload["payment_options"])
	return out
}

type stepRecorder struct {
	steps []StepResult
	limit int
}

func (r *stepRecorder) call(label string, fn func() (*protocol.Response, error)) (*protocol.Response, error) {
	resp, err := fn()
	r.steps = append(r.steps, stepResult(label, resp, err, r.limit))
	return resp, err
}

func (r stepRecorder) result(paymentID string, status string, err error) LinkPaymentResult {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return LinkPaymentResult{
		Success:      err == nil,
		ErrorMessage: message,
		PaymentID:    paymentID,
		Status:       status,
		Steps:        r.steps,
	}
}

func stepResult(label string, response *protocol.Response, err error, limit int) StepResult {
	result := StepResult{Label: label}
	if response != nil {
		result.StatusCode = response.StatusCode
		result.ResponseText = protocol.Snippet(protocol.RedactText(string(response.Body)), limit)
	}
	if err != nil {
		result.ErrorMessage = protocol.RedactText(err.Error())
	}
	return result
}

func normalizeBodyLimit(limit int) int {
	if limit <= 0 {
		return 1200
	}
	return limit
}
