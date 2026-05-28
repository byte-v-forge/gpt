package stripe

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/browserfingerprint"
	"github.com/byte-v-forge/common-lib/fingerprinthttp"
)

const ChatGPTBaseURL = "https://chatgpt.com"

type GptClient struct {
	client  *fingerprinthttp.Client
	profile fingerprinthttp.Profile
}

type GptClientConfig struct {
	Profile  fingerprinthttp.Profile
	ProxyURL string
	Timeout  time.Duration
}

func NewGptClient(cfg GptClientConfig) (*GptClient, error) {
	profile := cfg.Profile.WithDefaults(DefaultGptProfile())
	if cfg.ProxyURL != "" {
		profile.ProxyURL = strings.TrimSpace(cfg.ProxyURL)
	}
	if strings.TrimSpace(profile.DeviceID) == "" {
		return nil, fmt.Errorf("gpt client device_id is required")
	}
	client, err := fingerprinthttp.New(fingerprinthttp.Config{
		Timeout:      cfg.Timeout,
		ProxyURL:     profile.ProxyURL,
		Profile:      profile,
		DisableHTTP3: true,
		RetryMax:     3,
	})
	if err != nil {
		return nil, err
	}
	return &GptClient{client: client, profile: profile}, nil
}

func DefaultGptProfile() fingerprinthttp.Profile {
	candidate := browserfingerprint.DefaultChromiumCandidates()[0]
	fp := browserfingerprint.BuildChromium(candidate, "en-US", "")
	return fingerprinthttp.Profile{
		TLSProfileName: fp.TLSProfileName,
		UserAgent:      fp.UserAgent,
		SecCHUA:        fp.SecCHUA,
		SecCHPlatform:  fp.SecCHPlatform,
		AcceptLanguage: "en-US,en;q=0.9",
		Language:       "en-US",
	}
}

func (c *GptClient) Request(ctx context.Context, method, rawURL string, opts fingerprinthttp.RequestOptions) (*fingerprinthttp.Response, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("gpt client is nil")
	}
	if err := requireGptOpenAIURL(rawURL); err != nil {
		return nil, err
	}
	headers := http.Header{}
	mergeHeader(headers, opts.Headers)
	c.profile.ApplyBrowserHeaders(headers)
	if headers.Get("Accept") == "" {
		headers.Set("Accept", "*/*")
	}
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("oai-language", "en-US")
	headers.Set("oai-device-id", c.profile.DeviceID)
	opts.Headers = headers
	return c.client.Request(ctx, method, rawURL, opts)
}

func (c *GptClient) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}

func requireGptOpenAIURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "chatgpt.com" || strings.HasSuffix(host, ".chatgpt.com") || host == "openai.com" || strings.HasSuffix(host, ".openai.com") {
		return nil
	}
	return fmt.Errorf("gpt client refuses non gpt/openai url: %s", host)
}

func mergeHeader(dst http.Header, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
