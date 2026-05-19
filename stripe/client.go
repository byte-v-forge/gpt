package stripe

import (
	"context"
	"net/http"
	"time"

	"github.com/byte-v-forge/gpt/stripe/internal/protocol"
)

const (
	ChatGPTBaseURL  = "https://chatgpt.com"
	StripeBaseURL   = "https://api.stripe.com"
	MidtransBaseURL = "https://app.midtrans.com"
	GatewayBaseURL  = "https://gwa.gopayapi.com"
	DefaultStripePK = "pk_live_51HOrSwC6h1nxGoI3lTAgRjYVrz4dU3fVOabyCcKR3pbEJguCVAlqCxdxCUvoRh1XWwRacViovU3kLKvpkjh7IqkW00iXQsjo3n"
	DefaultTimeout  = 30 * time.Second
)

type ClientSet struct {
	ChatGPT  *protocol.Client
	Stripe   *protocol.Client
	Midtrans *protocol.Client
	GoPayGWA *protocol.Client
}

type ClientSetConfig struct {
	HTTPClient *http.Client
	ProxyURL   string
	Timeout    time.Duration
	Retry      protocol.RetryPolicy
	Logger     protocol.Logger
}

func NewClientSet(cfg ClientSetConfig) (*ClientSet, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		var err error
		httpClient, err = protocol.NewHTTPClient(cfg.Timeout, cfg.ProxyURL)
		if err != nil {
			return nil, err
		}
	}
	opts := []protocol.Option{
		protocol.WithHTTPClient(httpClient),
		protocol.WithRetry(cfg.Retry),
		protocol.WithLogger(cfg.Logger),
	}
	chatgpt, err := protocol.NewClient(ChatGPTBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	stripe, err := protocol.NewClient(StripeBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	midtrans, err := protocol.NewClient(MidtransBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	gwa, err := protocol.NewClient(GatewayBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	return &ClientSet{ChatGPT: chatgpt, Stripe: stripe, Midtrans: midtrans, GoPayGWA: gwa}, nil
}

func ProbeTierFromAccessToken(ctx context.Context, client *protocol.Client, accessToken string) (map[string]any, error) {
	headers := http.Header{
		"Authorization": []string{"Bearer " + accessToken},
		"Accept":        []string{"application/json"},
		"User-Agent":    []string{"codex-cli"},
	}
	if accountID := AccessTokenAccountID(accessToken); accountID != "" {
		headers.Set("ChatGPT-Account-Id", accountID)
	}
	resp, err := client.Do(ctx, protocol.Request{
		Method:       http.MethodGet,
		Path:         "/backend-api/wham/usage",
		Headers:      headers,
		Operation:    "chatgpt-wham-usage",
		ExpectStatus: []int{http.StatusOK},
	})
	if err != nil {
		return nil, err
	}
	return map[string]any(resp.Payload), nil
}
