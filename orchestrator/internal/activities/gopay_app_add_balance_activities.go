package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm/clause"
	"orchestrator/db"
	pb "orchestrator/pb"
)

const (
	goPayEnvelopeLinkSecretKey       = "gopay_add_balance_envelope_link"
	rekberinajaAccessTokenSecretKey  = "gopay_add_balance_rekberinaja_access_token"
	rekberinajaRefreshTokenSecretKey = "gopay_add_balance_rekberinaja_refresh_token"
)

func (s *Server) GoPayAppAddBalanceActivity(ctx context.Context, input GoPayAppAddBalanceInput) (GoPayAppAddBalanceOutput, error) {
	output := GoPayAppAddBalanceOutput{StateJson: normalizeGoPayWorkflowStateJSON(input.GetStateJson())}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppAddBalance, false, true)
	_, err := step.run(func() (any, error) {
		return s.runGoPayAddBalance(ctx, step, input, &output, data)
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) runGoPayAddBalance(ctx context.Context, step activityStep, input GoPayAppAddBalanceInput, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	addBalance := input.GetAddBalance()
	if addBalance == nil {
		err := fmt.Errorf("add_balance is required")
		data["error_message"] = err.Error()
		return data, err
	}
	switch {
	case addBalance.GetManualTransfer() != nil:
		return s.prepareManualTransferAddBalance(ctx, step, addBalance.GetManualTransfer(), output, data)
	case addBalance.GetEnvelope() != nil:
		return s.claimEnvelopeAddBalance(ctx, step, addBalance.GetEnvelope(), output, data)
	case addBalance.GetRekberinaja() != nil:
		return s.submitRekberinajaAddBalance(ctx, step, addBalance.GetRekberinaja(), input.GetTargetPhone(), output, data)
	default:
		err := fmt.Errorf("add_balance method is required")
		data["error_message"] = err.Error()
		return data, err
	}
}

func (s *Server) prepareManualTransferAddBalance(ctx context.Context, step activityStep, transfer *pb.GoPayManualTransferAddBalance, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	if s.gopayClient == nil {
		err := fmt.Errorf("gopay-app client not configured")
		data["error_message"] = err.Error()
		return data, err
	}

	data["method"] = "manual_transfer"
	data["status"] = "awaiting_manual_confirmation"
	data["manual_confirmation_required"] = true
	output.Method = "manual_transfer"
	output.Status = "awaiting_manual_confirmation"
	output.Success = true

	step.progress("fetching qr_id from gopay-app", nil)
	resp, err := s.gopayClient.GetQrId(ctx, &pb.GetQrIdRequest{StateJson: output.GetStateJson()})
	if err != nil {
		err = fmt.Errorf("GetQrId: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	if !resp.GetSuccess() {
		err = fmt.Errorf("GetQrId: %s", resp.GetErrorMessage())
		data["error_message"] = err.Error()
		return data, err
	}

	qrPayload := fmt.Sprintf(`{"qr_id":"%s"}`, resp.GetQrId())
	data["manual_transfer"] = map[string]any{
		"configured":         true,
		"qr_payload":         qrPayload,
		"qr_payload_present": true,
		"qr_image_present":   false,
		"instructions":       strings.TrimSpace(transfer.GetInstructions()),
		"amount":             transfer.GetAmount(),
		"currency":           strings.TrimSpace(transfer.GetCurrency()),
	}
	step.progress("waiting for manual gopay transfer confirmation", map[string]any{
		"qr_payload_present": true,
	})
	return data, nil
}

func (s *Server) submitRekberinajaAddBalance(ctx context.Context, step activityStep, cfg *pb.GoPayRekberinajaAddBalance, targetPhone string, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	endpointURL := strings.TrimSpace(cfg.GetEndpointUrl())
	bearerToken := strings.TrimSpace(cfg.GetBearerToken())
	refreshToken := strings.TrimSpace(cfg.GetRefreshToken())
	bearerToken, refreshToken = s.loadRekberinajaTokens(ctx, bearerToken, refreshToken)
	deviceID := strings.TrimSpace(cfg.GetDeviceId())
	store := strings.TrimSpace(cfg.GetStore())
	productID := strings.TrimSpace(cfg.GetProductId())
	serviceID := strings.TrimSpace(cfg.GetServiceId())
	paymentMethod := strings.TrimSpace(cfg.GetPaymentMethod())
	invoiceEmail := strings.TrimSpace(cfg.GetInvoiceEmail())
	paymentPhone := rekberinajaPaymentPhone(targetPhone)
	if paymentMethod == "" {
		paymentMethod = "saldo"
	}
	if store == "" {
		store = "rekberinaja"
	}

	data["method"] = "rekberinaja"
	data["status"] = "checkout_submitting"
	output.Method = "rekberinaja"
	output.Status = "checkout_submitting"
	rekData := map[string]any{
		"endpoint_present":      endpointURL != "",
		"bearer_token_present":  bearerToken != "",
		"refresh_token_present": refreshToken != "",
		"device_id_present":     deviceID != "",
		"store":                 store,
		"product_id_present":    productID != "",
		"service_id_present":    serviceID != "",
		"payment_method":        paymentMethod,
		"invoice_email_present": invoiceEmail != "",
		"target_phone_present":  paymentPhone != "",
		"fee_total":             cfg.GetFeeTotal(),
	}
	data["rekberinaja"] = rekData

	missing := []string{}
	if endpointURL == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_ENDPOINT_URL")
	}
	if bearerToken == "" && refreshToken == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_BEARER_TOKEN or GOPAY_ADD_BALANCE_REKBERINAJA_REFRESH_TOKEN")
	}
	if deviceID == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_DEVICE_ID")
	}
	if productID == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_PRODUCT_ID")
	}
	if serviceID == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_SERVICE_ID")
	}
	if invoiceEmail == "" {
		missing = append(missing, "GOPAY_ADD_BALANCE_REKBERINAJA_INVOICE_EMAIL")
	}
	if paymentPhone == "" {
		missing = append(missing, "target_phone")
	}
	if len(missing) > 0 {
		err := fmt.Errorf("rekberinaja add_balance config is incomplete: %s", strings.Join(missing, ", "))
		data["error_message"] = err.Error()
		return data, err
	}

	apiBaseURL, err := rekberinajaAPIBaseURL(endpointURL)
	if err != nil {
		err = fmt.Errorf("rekberinaja api base url: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	if parsed, parseErr := url.Parse(apiBaseURL); parseErr == nil {
		rekData["endpoint_host"] = parsed.Host
	}

	client := &rekberinajaAPIClient{
		httpClient:   &http.Client{Timeout: 45 * time.Second},
		endpointURL:  endpointURL,
		apiBaseURL:   apiBaseURL,
		accessToken:  bearerToken,
		refreshToken: refreshToken,
		deviceID:     deviceID,
		store:        store,
		userAgent:    strings.TrimSpace(cfg.GetUserAgent()),
		origin:       strings.TrimSpace(cfg.GetOrigin()),
		referer:      strings.TrimSpace(cfg.GetReferer()),
		onTokenRefresh: func(accessToken, refreshToken string) error {
			return s.saveRekberinajaTokens(ctx, accessToken, refreshToken)
		},
	}

	if client.accessToken == "" && client.refreshToken != "" {
		step.progress("refreshing rekberinaja access token", map[string]any{"refresh_token_present": true})
		if err := client.refreshAccessToken(ctx); err != nil {
			err = fmt.Errorf("rekberinaja refresh token: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		rekData["access_token_refreshed"] = true
	}

	feeTotal := cfg.GetFeeTotal()
	if feeTotal > 0 {
		step.progress("calculating rekberinaja product fee", map[string]any{"fee_total": feeTotal})
		feeResp, err := client.doJSON(ctx, http.MethodPost, rekberinajaJoinURL(apiBaseURL, "/fee/calculate"), map[string]any{
			"type":  "Product",
			"total": feeTotal,
		}, true, true)
		if err != nil {
			err = fmt.Errorf("rekberinaja fee calculate: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
		feeData := rekberinajaDataObject(feeResp.body)
		rekData["fee"] = map[string]any{
			"http_status": feeResp.httpStatus,
			"total":       rekberinajaInt64(feeData["total"]),
		}
	}

	payload := map[string]any{
		"product_id":     productID,
		"promo_code":     strings.TrimSpace(cfg.GetPromoCode()),
		"use_poin":       cfg.GetUsePoin(),
		"data":           paymentPhone,
		"payment_method": paymentMethod,
		"invoice_email":  invoiceEmail,
		"service_id":     serviceID,
	}
	step.progress("submitting rekberinaja add_balance checkout", map[string]any{
		"endpoint_host":         rekData["endpoint_host"],
		"product_id_present":    true,
		"service_id_present":    true,
		"invoice_email_present": true,
		"target_phone_present":  true,
	})
	checkoutResp, err := client.doJSON(ctx, http.MethodPost, endpointURL, payload, true, true)
	if err != nil {
		err = fmt.Errorf("rekberinaja checkout: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	transactionID := rekberinajaStringAt(checkoutResp.body, "data", "transaction_id")
	rekData["checkout"] = map[string]any{
		"http_status":            checkoutResp.httpStatus,
		"transaction_id_present": transactionID != "",
	}
	data["status"] = "checkout_submitted"
	output.Status = "checkout_submitted"
	if transactionID == "" {
		err = fmt.Errorf("rekberinaja checkout did not return transaction_id")
		data["error_message"] = err.Error()
		return data, err
	}

	transactionURL := rekberinajaJoinURL(apiBaseURL, "/transaction/"+url.PathEscape(transactionID))
	transactionResp, err := client.doJSON(ctx, http.MethodGet, transactionURL, nil, true, true)
	if err != nil {
		err = fmt.Errorf("rekberinaja transaction detail: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	transactionData := rekberinajaDataObject(transactionResp.body)
	rekData["transaction"] = map[string]any{
		"http_status": transactionResp.httpStatus,
		"status":      rekberinajaString(transactionData["status"]),
		"total":       rekberinajaInt64(transactionData["total"]),
	}

	step.progress("paying rekberinaja transaction from saldo", map[string]any{"transaction_id_present": true})
	payResp, err := client.doJSON(ctx, http.MethodGet, transactionURL+"/pay", nil, true, true)
	if err != nil {
		err = fmt.Errorf("rekberinaja transaction pay: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	rekData["pay"] = map[string]any{
		"http_status": payResp.httpStatus,
		"message":     rekberinajaString(payResp.body["message"]),
	}
	data["status"] = "pay_submitted"
	output.Status = "pay_submitted"

	pollTimeout := time.Duration(cfg.GetPollTimeoutSeconds()) * time.Second
	if pollTimeout <= 0 {
		pollTimeout = 180 * time.Second
	}
	pollInterval := time.Duration(cfg.GetPollIntervalSeconds()) * time.Second
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	deadline := time.Now().Add(pollTimeout)
	orderURL := transactionURL + "/order-product"
	for attempt := 1; ; attempt++ {
		step.progress("polling rekberinaja order product", map[string]any{
			"attempt":                attempt,
			"poll_timeout_seconds":   int(pollTimeout / time.Second),
			"poll_interval_seconds":  int(pollInterval / time.Second),
			"transaction_id_present": true,
		})
		orderResp, err := client.doJSON(ctx, http.MethodGet, orderURL, nil, true, true)
		if err != nil {
			err = fmt.Errorf("rekberinaja order product: %w", err)
			data["error_message"] = err.Error()
			return data, err
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
		if orderStatus == "completed" && orderStatusCode == "00" {
			output.Success = true
			output.Status = "completed"
			data["status"] = "completed"
			data["add_balance_complete"] = true
			rekData["success"] = true
			step.progress("rekberinaja add_balance completed", map[string]any{
				"status":      orderStatus,
				"status_code": orderStatusCode,
			})
			return data, nil
		}
		if orderStatus == "failed" || orderStatus == "canceled" || (orderStatusCode != "" && orderStatusCode != "00") {
			err = fmt.Errorf("rekberinaja order product failed: status=%s status_code=%s", orderStatus, orderStatusCode)
			data["error_message"] = err.Error()
			output.ErrorMessage = err.Error()
			return data, err
		}
		if time.Now().After(deadline) {
			err = fmt.Errorf("rekberinaja order product did not complete before timeout: status=%s status_code=%s", orderStatus, orderStatusCode)
			data["error_message"] = err.Error()
			output.ErrorMessage = err.Error()
			return data, err
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			err = ctx.Err()
			data["error_message"] = err.Error()
			return data, err
		case <-timer.C:
		}
	}
}

func (s *Server) claimEnvelopeAddBalance(ctx context.Context, step activityStep, envelope *pb.GoPayEnvelopeAddBalance, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	if s.gopayClient == nil {
		err := fmt.Errorf("gopay-app client not configured")
		data["error_message"] = err.Error()
		return data, err
	}

	envelopeLink := strings.TrimSpace(envelope.GetLink())
	if envelopeLink == "" {
		envelopeLink = s.loadRuntimeSecret(ctx, goPayEnvelopeLinkSecretKey)
	}
	envelopeRequestID := strings.TrimSpace(envelope.GetEnvelopeRequestId())
	data["method"] = "envelope"
	data["status"] = "claiming"
	data["envelope_link_present"] = envelopeLink != ""
	data["envelope_request_id_present"] = envelopeRequestID != ""
	if envelopeLink == "" && envelopeRequestID == "" {
		err := fmt.Errorf("GOPAY_ADD_BALANCE_ENVELOPE_LINK or envelope_request_id is required")
		data["error_message"] = err.Error()
		return data, err
	}
	if envelopeLink != "" {
		if err := s.saveRuntimeSecret(ctx, goPayEnvelopeLinkSecretKey, envelopeLink); err != nil {
			err = fmt.Errorf("store gopay envelope link: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
	}

	step.progress("claiming gopay envelope", map[string]any{
		"envelope_link_present":       envelopeLink != "",
		"envelope_request_id_present": envelopeRequestID != "",
	})
	resp, err := s.gopayClient.ClaimEnvelope(ctx, &pb.ClaimEnvelopeRequest{
		EnvelopeRequestId: envelopeRequestID,
		Link:              envelopeLink,
		StateJson:         output.GetStateJson(),
	})
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(resp))
	data["envelope"] = claimEnvelopeData(resp)
	if err != nil {
		err = fmt.Errorf("ClaimEnvelope: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	if resp == nil {
		err := fmt.Errorf("ClaimEnvelope returned empty response")
		data["error_message"] = err.Error()
		return data, err
	}
	output.Success = resp.GetSuccess()
	output.Method = "envelope"
	output.Status = resp.GetStatus()
	data["status"] = resp.GetStatus()
	if !resp.GetSuccess() {
		message := strings.TrimSpace(resp.GetErrorMessage())
		if message == "" {
			message = "claim envelope failed"
		}
		output.ErrorMessage = message
		err := fmt.Errorf("ClaimEnvelope: %s", message)
		data["error_message"] = err.Error()
		return data, err
	}
	data["add_balance_complete"] = true
	return data, nil
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

func (s *Server) loadRekberinajaTokens(ctx context.Context, fallbackAccessToken string, fallbackRefreshToken string) (string, string) {
	if s == nil || s.db == nil {
		return fallbackAccessToken, fallbackRefreshToken
	}
	accessToken := fallbackAccessToken
	refreshToken := fallbackRefreshToken
	var rows []db.RuntimeSecret
	if err := s.db.WithContext(ctx).Where("key IN ?", []string{rekberinajaAccessTokenSecretKey, rekberinajaRefreshTokenSecretKey}).Find(&rows).Error; err != nil {
		return accessToken, refreshToken
	}
	for _, row := range rows {
		switch row.Key {
		case rekberinajaAccessTokenSecretKey:
			if value := strings.TrimSpace(row.Value); value != "" {
				accessToken = value
			}
		case rekberinajaRefreshTokenSecretKey:
			if value := strings.TrimSpace(row.Value); value != "" {
				refreshToken = value
			}
		}
	}
	return accessToken, refreshToken
}

func (s *Server) loadRuntimeSecret(ctx context.Context, key string) string {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	var row db.RuntimeSecret
	if err := s.db.WithContext(ctx).First(&row, "key = ?", strings.TrimSpace(key)).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(row.Value)
}

func (s *Server) saveRekberinajaTokens(ctx context.Context, accessToken string, refreshToken string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if strings.TrimSpace(accessToken) != "" {
		if err := s.saveRuntimeSecret(ctx, rekberinajaAccessTokenSecretKey, accessToken); err != nil {
			return err
		}
	}
	if strings.TrimSpace(refreshToken) != "" {
		if err := s.saveRuntimeSecret(ctx, rekberinajaRefreshTokenSecretKey, refreshToken); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) saveRuntimeSecret(ctx context.Context, key string, value string) error {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	row := db.RuntimeSecret{Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

type rekberinajaAPIClient struct {
	httpClient     *http.Client
	endpointURL    string
	apiBaseURL     string
	accessToken    string
	refreshToken   string
	deviceID       string
	store          string
	userAgent      string
	origin         string
	referer        string
	onTokenRefresh func(accessToken string, refreshToken string) error
}

type rekberinajaAPIResponse struct {
	httpStatus int
	body       map[string]any
	raw        string
}

func (c *rekberinajaAPIClient) doJSON(ctx context.Context, method string, endpoint string, payload any, includeStore bool, cacheBuster bool) (rekberinajaAPIResponse, error) {
	resp, err := c.doJSONOnce(ctx, method, endpoint, payload, includeStore, cacheBuster)
	if resp.httpStatus == http.StatusUnauthorized && c.refreshToken != "" {
		if refreshErr := c.refreshAccessToken(ctx); refreshErr != nil {
			return resp, refreshErr
		}
		return c.doJSONOnce(ctx, method, endpoint, payload, includeStore, cacheBuster)
	}
	if err != nil {
		return resp, err
	}
	return resp, nil
}

func (c *rekberinajaAPIClient) doJSONOnce(ctx context.Context, method string, endpoint string, payload any, includeStore bool, cacheBuster bool) (rekberinajaAPIResponse, error) {
	requestURL := strings.TrimSpace(endpoint)
	if cacheBuster {
		var err error
		requestURL, err = rekberinajaURLWithCacheBuster(requestURL, time.Now())
		if err != nil {
			return rekberinajaAPIResponse{}, err
		}
	}

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return rekberinajaAPIResponse{}, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return rekberinajaAPIResponse{}, err
	}
	c.setHeaders(req, includeStore, payload != nil)
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	httpResp, err := httpClient.Do(req)
	if err != nil {
		return rekberinajaAPIResponse{}, err
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 65536))
	if err != nil {
		return rekberinajaAPIResponse{httpStatus: httpResp.StatusCode}, err
	}
	response := rekberinajaAPIResponse{httpStatus: httpResp.StatusCode, raw: string(raw)}
	if strings.TrimSpace(response.raw) != "" {
		_ = json.Unmarshal(raw, &response.body)
	}
	if response.body == nil {
		response.body = map[string]any{}
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return response, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, rekberinajaResponseMessage(response.body, response.raw))
	}
	if rekberinajaBoolFalse(response.body, "success") || rekberinajaBoolFalse(response.body, "status") {
		message := rekberinajaResponseMessage(response.body, response.raw)
		if message == "" {
			message = "request rejected"
		}
		return response, fmt.Errorf("%s", message)
	}
	return response, nil
}

func (c *rekberinajaAPIClient) setHeaders(req *http.Request, includeStore bool, hasBody bool) {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("DNT", "1")
	req.Header.Set("Origin", c.origin)
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", c.referer)
	req.Header.Set("Sec-CH-UA", `"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Device-Id", c.deviceID)
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	if includeStore && c.store != "" {
		req.Header.Set("X-Store", c.store)
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
}

func (c *rekberinajaAPIClient) refreshAccessToken(ctx context.Context) error {
	if c.refreshToken == "" {
		return fmt.Errorf("refresh token is required")
	}
	previousAccessToken := c.accessToken
	c.accessToken = ""
	resp, err := c.doJSONOnce(ctx, http.MethodPost, rekberinajaJoinURL(c.apiBaseURL, "/auth/refresh-token"), map[string]any{
		"refresh_token": c.refreshToken,
	}, false, false)
	if err != nil {
		c.accessToken = previousAccessToken
		return err
	}
	accessToken := strings.TrimSpace(rekberinajaStringAt(resp.body, "data", "access_token"))
	if accessToken == "" {
		c.accessToken = previousAccessToken
		return fmt.Errorf("refresh-token response missing access_token")
	}
	c.accessToken = accessToken
	if nextRefresh := strings.TrimSpace(rekberinajaStringAt(resp.body, "data", "refresh_token")); nextRefresh != "" {
		c.refreshToken = nextRefresh
	}
	if c.onTokenRefresh != nil {
		if err := c.onTokenRefresh(c.accessToken, c.refreshToken); err != nil {
			return fmt.Errorf("store refreshed token: %w", err)
		}
	}
	return nil
}

func rekberinajaAPIBaseURL(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("absolute URL is required")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if index := strings.Index(parsed.Path, "/api/"); index >= 0 {
		parsed.Path = parsed.Path[:index+len("/api")]
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	return "", fmt.Errorf("endpoint path must contain /api/")
}

func rekberinajaJoinURL(base string, suffix string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

func rekberinajaURLWithCacheBuster(endpoint string, now time.Time) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("absolute URL is required")
	}
	query := parsed.Query()
	if query.Get("_") == "" {
		query.Set("_", strconv.FormatInt(now.UnixMilli(), 10))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func rekberinajaDataObject(response map[string]any) map[string]any {
	if response == nil {
		return map[string]any{}
	}
	data, ok := response["data"].(map[string]any)
	if !ok || data == nil {
		return map[string]any{}
	}
	return data
}

func rekberinajaStringAt(response map[string]any, path ...string) string {
	var current any = response
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[key]
	}
	return rekberinajaString(current)
}

func rekberinajaString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func rekberinajaInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func rekberinajaBoolFalse(response map[string]any, key string) bool {
	if response == nil {
		return false
	}
	value, ok := response[key].(bool)
	return ok && !value
}

func rekberinajaResponseMessage(response map[string]any, raw string) string {
	for _, key := range []string{"error_message", "message", "error"} {
		if value := strings.TrimSpace(fmt.Sprint(response[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return limitStepText(strings.TrimSpace(raw), 500)
}

func claimEnvelopeData(resp *pb.ClaimEnvelopeResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":             true,
		"success":                      resp.GetSuccess(),
		"error_message":                resp.GetErrorMessage(),
		"envelope_request_id":          resp.GetEnvelopeRequestId(),
		"response_envelope_request_id": resp.GetResponseEnvelopeRequestId(),
		"status":                       resp.GetStatus(),
		"http_status":                  resp.GetHttpStatus(),
		"raw_json":                     limitStepText(resp.GetRawJson(), 2000),
	}
}

func limitStepText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...<truncated>"
}
