package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/protojsonx"
	"google.golang.org/protobuf/proto"
)

type n8nWebhookClient struct {
	name       string
	webhookURL string
	httpClient *http.Client
}

func newN8NWebhookClient(name string, baseURL string, path string) *n8nWebhookClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil
	}
	path = strings.Trim(path, "/")
	url := strings.TrimRight(baseURL, "/")
	if path != "" {
		url += "/" + path
	}
	return &n8nWebhookClient{
		name:       strings.TrimSpace(name),
		webhookURL: url,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *n8nWebhookClient) trigger(ctx context.Context, payload any) error {
	if c == nil || c.webhookURL == "" {
		return fmt.Errorf("n8n %s workflow is not configured", c.workflowName())
	}
	body, err := marshalN8NWebhookPayload(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("trigger n8n %s workflow: %w", c.workflowName(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("n8n %s workflow returned HTTP %d", c.workflowName(), resp.StatusCode)
	}
	return nil
}

func (c *n8nWebhookClient) workflowName() string {
	if c == nil || strings.TrimSpace(c.name) == "" {
		return "webhook"
	}
	return c.name
}

func marshalN8NWebhookPayload(payload any) ([]byte, error) {
	if message, ok := payload.(proto.Message); ok {
		return protojsonx.Marshal(message)
	}
	return json.Marshal(payload)
}
