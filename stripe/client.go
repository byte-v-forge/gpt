package stripe

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/byte-v-forge/common-lib/fingerprinthttp"
	"github.com/byte-v-forge/common-lib/httpclient"
	"github.com/byte-v-forge/common-lib/httpjson"
)

const (
	StripeBaseURL   = "https://api.stripe.com"
	MidtransBaseURL = "https://app.midtrans.com"
	DefaultStripePK = "pk_live_51HOrSwC6h1nxGoI3lTAgRjYVrz4dU3fVOabyCcKR3pbEJguCVAlqCxdxCUvoRh1XWwRacViovU3kLKvpkjh7IqkW00iXQsjo3n"
	DefaultTimeout  = 30 * time.Second
)

type ClientSet struct {
	Stripe   *httpjson.Client
	Midtrans *httpjson.Client
}

type ClientSetConfig struct {
	HTTPClient *http.Client
	ProxyURL   string
	Timeout    time.Duration
	Retry      httpjson.RetryPolicy
	Logger     httpjson.Logger
}

func NewClientSet(cfg ClientSetConfig) (*ClientSet, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		var err error
		httpClient, err = httpclient.NewWithSchemes(cfg.Timeout, cfg.ProxyURL, httpclient.HTTPProxySchemes...)
		if err != nil {
			return nil, err
		}
	}
	opts := []httpjson.Option{
		httpjson.WithHTTPClient(httpClient),
		httpjson.WithRetry(cfg.Retry),
		httpjson.WithLogger(cfg.Logger),
	}
	stripe, err := httpjson.NewClient(StripeBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	midtrans, err := httpjson.NewClient(MidtransBaseURL, opts...)
	if err != nil {
		return nil, err
	}
	return &ClientSet{Stripe: stripe, Midtrans: midtrans}, nil
}

func ProbeTierFromAccessToken(ctx context.Context, client *GptClient, accessToken string) (map[string]any, error) {
	headers := http.Header{
		"Authorization": []string{"Bearer " + accessToken},
		"Accept":        []string{"application/json"},
	}
	if accountID := AccessTokenAccountID(accessToken); accountID != "" {
		headers.Set("ChatGPT-Account-Id", accountID)
	}
	resp, err := client.Request(ctx, http.MethodGet, ChatGPTBaseURL+"/backend-api/wham/usage", fingerprinthttp.RequestOptions{Headers: headers})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatgpt wham usage returned status %d: %s", resp.StatusCode, resp.Excerpt(300))
	}
	return resp.JSON, nil
}
