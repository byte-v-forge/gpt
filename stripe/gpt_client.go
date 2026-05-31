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
	profile := cleanGptProfile(cfg.Profile.WithDefaults(DefaultGptProfile()))
	if cfg.ProxyURL != "" {
		profile.ProxyURL = strings.TrimSpace(cfg.ProxyURL)
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
	candidate := defaultGptChromiumCandidate("")
	fp := browserfingerprint.BuildChromium(candidate, "en-US", "")
	return fingerprinthttp.Profile{
		TLSProfileName: fp.TLSProfileName,
		UserAgent:      gptUserAgent(candidate),
		AcceptLanguage: "en-US,en;q=0.9",
		Language:       "en-US",
	}
}

func cleanGptProfile(profile fingerprinthttp.Profile) fingerprinthttp.Profile {
	fallback := DefaultGptProfile()
	profile = profile.WithDefaults(fallback)
	candidate := defaultGptChromiumCandidate(profile.TLSProfileName)
	profile.TLSProfileName = candidate.ProfileName
	profile.UserAgent = gptUserAgent(candidate)
	profile.SecCHUA = ""
	profile.SecCHPlatform = ""
	profile.AcceptLanguage = "en-US,en;q=0.9"
	profile.Language = "en-US"
	profile.DeviceID = ""
	return profile
}

func defaultGptChromiumCandidate(tlsProfile string) browserfingerprint.ChromiumCandidate {
	profileName := browserfingerprint.ResolveTLSProfileName(tlsProfile, browserfingerprint.DefaultTLSProfileName)
	for _, candidate := range browserfingerprint.DefaultChromiumCandidates() {
		if strings.EqualFold(candidate.ProfileName, profileName) && browserfingerprint.OSAlias(candidate) == "mac" {
			return candidate
		}
	}
	for _, candidate := range browserfingerprint.DefaultChromiumCandidates() {
		if strings.EqualFold(candidate.ProfileName, browserfingerprint.DefaultTLSProfileName) && browserfingerprint.OSAlias(candidate) == "mac" {
			return candidate
		}
	}
	return browserfingerprint.DefaultChromiumCandidates()[0]
}

func gptUserAgent(candidate browserfingerprint.ChromiumCandidate) string {
	major := strings.TrimSpace(candidate.MajorVersion)
	if major == "" {
		major = "146"
	}
	return fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36", major)
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
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", c.profile.UserAgent)
	}
	if headers.Get("Accept") == "" {
		headers.Set("Accept", "*/*")
	}
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Del("sec-ch-ua")
	headers.Del("sec-ch-ua-mobile")
	headers.Del("sec-ch-ua-platform")
	headers.Del("oai-language")
	headers.Del("oai-device-id")
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
