package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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
