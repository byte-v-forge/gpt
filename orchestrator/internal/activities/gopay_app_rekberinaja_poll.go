package activities

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) pollRekberinajaOrderProduct(ctx context.Context, step activityStep, client *rekberinajaAPIClient, orderURL string, settings rekberinajaAddBalanceSettings, output *GoPayAppAddBalanceOutput, data map[string]any, rekData map[string]any) error {
	deadline := time.Now().Add(settings.pollTimeout)
	for attempt := 1; ; attempt++ {
		if s.applyGoPayAddBalanceReadyFromToken(ctx, output, data, "rekberinaja_order_poll") {
			step.progress("gopay balance ready while polling rekberinaja", map[string]any{
				"attempt":        attempt,
				"balance_amount": data["balance_amount"],
				"currency":       data["balance_currency"],
			})
			return nil
		}
		orderStatus, orderStatusCode, err := pollRekberinajaOrderProductOnce(ctx, step, client, orderURL, settings, attempt, rekData)
		if err != nil {
			return setRekberinajaAddBalanceError(data, err)
		}
		if completedRekberinajaOrderProduct(orderStatus, orderStatusCode, step, output, data, rekData) {
			return nil
		}
		if failedRekberinajaOrderProduct(orderStatus, orderStatusCode) {
			err := fmt.Errorf("rekberinaja order product failed: status=%s status_code=%s", orderStatus, orderStatusCode)
			output.ErrorMessage = err.Error()
			return setRekberinajaAddBalanceError(data, err)
		}
		if time.Now().After(deadline) {
			err := fmt.Errorf("rekberinaja order product did not complete before timeout: status=%s status_code=%s", orderStatus, orderStatusCode)
			output.ErrorMessage = err.Error()
			return setRekberinajaAddBalanceError(data, err)
		}
		if err := waitRekberinajaOrderProductPoll(ctx, settings.pollInterval); err != nil {
			return setRekberinajaAddBalanceError(data, err)
		}
	}
}

func pollRekberinajaOrderProductOnce(ctx context.Context, step activityStep, client *rekberinajaAPIClient, orderURL string, settings rekberinajaAddBalanceSettings, attempt int, rekData map[string]any) (string, string, error) {
	step.progress("polling rekberinaja order product", map[string]any{
		"attempt":                attempt,
		"poll_timeout_seconds":   settings.pollTimeoutSeconds(),
		"poll_interval_seconds":  settings.pollIntervalSeconds(),
		"transaction_id_present": true,
	})
	orderResp, err := client.doJSON(ctx, http.MethodGet, orderURL, nil, true, true)
	if err != nil {
		return "", "", fmt.Errorf("rekberinaja order product: %w", err)
	}
	orderData := rekberinajaDataObject(orderResp.body)
	orderStatus := strings.ToLower(rekberinajaString(orderData["status"]))
	orderStatusCode := rekberinajaString(orderData["status_code"])
	rekData["order_product"] = map[string]any{
		"http_status": orderResp.httpStatus,
		"status":      orderStatus,
		"status_code": orderStatusCode,
		"title":       rekberinajaString(orderData["title"]),
		"trx_id":      rekberinajaString(orderData["trx_id"]),
	}
	return orderStatus, orderStatusCode, nil
}

func completedRekberinajaOrderProduct(orderStatus string, orderStatusCode string, step activityStep, output *GoPayAppAddBalanceOutput, data map[string]any, rekData map[string]any) bool {
	if orderStatus != "completed" || orderStatusCode != "00" {
		return false
	}
	output.Success = true
	output.Status = "completed"
	data["status"] = "completed"
	data["add_balance_complete"] = true
	rekData["success"] = true
	step.progress("rekberinaja add_balance completed", map[string]any{
		"status":      orderStatus,
		"status_code": orderStatusCode,
	})
	return true
}

func failedRekberinajaOrderProduct(orderStatus string, orderStatusCode string) bool {
	return orderStatus == "failed" || orderStatus == "canceled" || (orderStatusCode != "" && orderStatusCode != "00")
}

func waitRekberinajaOrderProductPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
