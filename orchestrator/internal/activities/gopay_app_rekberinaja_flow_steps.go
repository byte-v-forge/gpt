package activities

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func refreshRekberinajaAccessTokenIfNeeded(ctx context.Context, step activityStep, client *rekberinajaAPIClient, rekData map[string]any) error {
	if client.accessToken != "" || client.refreshToken == "" {
		return nil
	}
	step.progress("refreshing rekberinaja access token", map[string]any{"refresh_token_present": true})
	if err := client.refreshAccessToken(ctx); err != nil {
		return fmt.Errorf("rekberinaja refresh token: %w", err)
	}
	rekData["access_token_refreshed"] = true
	return nil
}

func calculateRekberinajaProductFee(ctx context.Context, step activityStep, client *rekberinajaAPIClient, apiBaseURL string, settings rekberinajaAddBalanceSettings, rekData map[string]any) error {
	if settings.feeTotal <= 0 {
		return nil
	}
	step.progress("calculating rekberinaja product fee", map[string]any{"fee_total": settings.feeTotal})
	feeResp, err := client.doJSON(ctx, http.MethodPost, rekberinajaJoinURL(apiBaseURL, "/fee/calculate"), map[string]any{
		"type":  "Product",
		"total": settings.feeTotal,
	}, true, true)
	if err != nil {
		return fmt.Errorf("rekberinaja fee calculate: %w", err)
	}
	feeData := rekberinajaDataObject(feeResp.body)
	rekData["fee"] = map[string]any{
		"http_status": feeResp.httpStatus,
		"total":       rekberinajaInt64(feeData["total"]),
	}
	return nil
}

func submitRekberinajaCheckout(ctx context.Context, step activityStep, client *rekberinajaAPIClient, apiBaseURL string, settings rekberinajaAddBalanceSettings, output *GoPayAppAddBalanceOutput, data map[string]any, rekData map[string]any) (string, error) {
	step.progress("submitting rekberinaja add_balance checkout", map[string]any{
		"endpoint_host":         rekData["endpoint_host"],
		"product_id_present":    true,
		"service_id_present":    true,
		"invoice_email_present": true,
		"target_phone_present":  true,
	})
	checkoutResp, err := client.doJSON(ctx, http.MethodPost, settings.endpointURL, settings.checkoutPayload(), true, true)
	if err != nil {
		return "", fmt.Errorf("rekberinaja checkout: %w", err)
	}
	transactionID := rekberinajaStringAt(checkoutResp.body, "data", "transaction_id")
	rekData["checkout"] = map[string]any{
		"http_status":            checkoutResp.httpStatus,
		"transaction_id_present": transactionID != "",
	}
	data["status"] = "checkout_submitted"
	output.Status = "checkout_submitted"
	if transactionID == "" {
		return "", fmt.Errorf("rekberinaja checkout did not return transaction_id")
	}
	return rekberinajaJoinURL(apiBaseURL, "/transaction/"+url.PathEscape(transactionID)), nil
}

func fetchRekberinajaTransaction(ctx context.Context, client *rekberinajaAPIClient, transactionURL string, rekData map[string]any) error {
	transactionResp, err := client.doJSON(ctx, http.MethodGet, transactionURL, nil, true, true)
	if err != nil {
		return fmt.Errorf("rekberinaja transaction detail: %w", err)
	}
	transactionData := rekberinajaDataObject(transactionResp.body)
	rekData["transaction"] = map[string]any{
		"http_status": transactionResp.httpStatus,
		"status":      rekberinajaString(transactionData["status"]),
		"total":       rekberinajaInt64(transactionData["total"]),
	}
	return nil
}

func payRekberinajaTransaction(ctx context.Context, step activityStep, client *rekberinajaAPIClient, transactionURL string, output *GoPayAppAddBalanceOutput, data map[string]any, rekData map[string]any) error {
	step.progress("paying rekberinaja transaction from saldo", map[string]any{"transaction_id_present": true})
	payResp, err := client.doJSON(ctx, http.MethodGet, transactionURL+"/pay", nil, true, true)
	if err != nil {
		return fmt.Errorf("rekberinaja transaction pay: %w", err)
	}
	rekData["pay"] = map[string]any{
		"http_status": payResp.httpStatus,
		"message":     rekberinajaString(payResp.body["message"]),
	}
	data["status"] = "pay_submitted"
	output.Status = "pay_submitted"
	return nil
}
