package activities

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) submitRekberinajaAddBalance(ctx context.Context, step activityStep, cfg *pb.GoPayRekberinajaAddBalance, targetPhone string, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	settings := s.newRekberinajaAddBalanceSettings(ctx, cfg, targetPhone)
	data["method"] = "rekberinaja"
	data["status"] = "checkout_submitting"
	output.Method = "rekberinaja"
	output.Status = "checkout_submitting"

	rekData := settings.metadata()
	data["rekberinaja"] = rekData

	if missing := settings.missingRequiredFields(); len(missing) > 0 {
		err := fmt.Errorf("rekberinaja add_balance config is incomplete: %s", strings.Join(missing, ", "))
		return data, setRekberinajaAddBalanceError(data, err)
	}

	apiBaseURL, err := rekberinajaAPIBaseURL(settings.endpointURL)
	if err != nil {
		err = fmt.Errorf("rekberinaja api base url: %w", err)
		return data, setRekberinajaAddBalanceError(data, err)
	}
	if parsed, parseErr := url.Parse(apiBaseURL); parseErr == nil {
		rekData["endpoint_host"] = parsed.Host
	}

	client := settings.newAPIClient(ctx, s, apiBaseURL)
	if err := refreshRekberinajaAccessTokenIfNeeded(ctx, step, client, rekData); err != nil {
		return data, setRekberinajaAddBalanceError(data, err)
	}
	if err := calculateRekberinajaProductFee(ctx, step, client, apiBaseURL, settings, rekData); err != nil {
		return data, setRekberinajaAddBalanceError(data, err)
	}

	transactionURL, err := submitRekberinajaCheckout(ctx, step, client, apiBaseURL, settings, output, data, rekData)
	if err != nil {
		return data, setRekberinajaAddBalanceError(data, err)
	}
	if err := fetchRekberinajaTransaction(ctx, client, transactionURL, rekData); err != nil {
		return data, setRekberinajaAddBalanceError(data, err)
	}
	if err := payRekberinajaTransaction(ctx, step, client, transactionURL, output, data, rekData); err != nil {
		return data, setRekberinajaAddBalanceError(data, err)
	}
	if err := s.pollRekberinajaOrderProduct(ctx, step, client, transactionURL+"/order-product", settings, output, data, rekData); err != nil {
		return data, err
	}
	return data, nil
}

func setRekberinajaAddBalanceError(data map[string]any, err error) error {
	data["error_message"] = err.Error()
	return err
}

func rekberinajaPaymentPhone(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if strings.HasPrefix(digits, "0062") {
		digits = strings.TrimPrefix(digits, "0062")
	} else if strings.HasPrefix(digits, "62") {
		digits = strings.TrimPrefix(digits, "62")
	}
	if digits == "" {
		return ""
	}
	if strings.HasPrefix(digits, "0") {
		return digits
	}
	return "0" + digits
}
