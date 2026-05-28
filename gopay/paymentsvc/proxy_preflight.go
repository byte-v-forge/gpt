package paymentsvc

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/hashx"
)

type paymentProxyProbeResult struct {
	Labels  []string
	IP      string
	Country string
}

func (s *Server) ensurePaymentProxyAvailable(ctx context.Context, fingerprint browserFingerprint) (paymentProxyProbeResult, error) {
	proxies := paymentProxyLabels(s.cfg)
	if len(proxies) == 0 {
		return paymentProxyProbeResult{}, nil
	}
	var combined paymentProxyProbeResult
	for proxyURL, labels := range proxies {
		probe, err := probePaymentProxy(ctx, proxyURL, labels, fingerprint)
		if err != nil {
			return combined, err
		}
		if combined.IP == "" {
			combined.IP = probe.IP
			combined.Country = probe.Country
		}
		combined.Labels = append(combined.Labels, labels...)
		log.Printf(
			"[gopay-payment] proxy probe ok labels=%s exit_ip=%s exit_country=%s proxy=%s",
			strings.Join(labels, ","),
			probe.IP,
			probe.Country,
			paymentProxyHash(proxyURL),
		)
	}
	return combined, nil
}

func paymentProxyLabels(cfg Config) map[string][]string {
	out := map[string][]string{}
	add := func(label string, proxyURL string) {
		proxyURL = strings.TrimSpace(proxyURL)
		if proxyURL == "" {
			return
		}
		out[proxyURL] = append(out[proxyURL], label)
	}
	add("checkout", cfg.CheckoutProfile.ProxyURL)
	add("payment", cfg.PaymentProfile.ProxyURL)
	return out
}

func probePaymentProxy(ctx context.Context, proxyURL string, labels []string, fingerprint browserFingerprint) (paymentProxyProbeResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	session, err := newHTTPSession(proxyURL, fingerprint)
	if err != nil {
		return paymentProxyProbeResult{}, fmt.Errorf("payment proxy %s init failed: %w", strings.Join(labels, ","), err)
	}
	defer session.close()
	fingerprint.applyBrowserHeaders(session.headers)

	probe := probePaymentProxyExitIP(probeCtx, session)
	if strings.TrimSpace(probe.IP) == "" {
		return probe, fmt.Errorf("payment proxy %s exit ip probe missing", strings.Join(labels, ","))
	}

	gptClient, err := newGptClientWithFingerprint(proxyURL, fingerprint)
	if err != nil {
		return probe, fmt.Errorf("payment proxy %s chatgpt client init failed: %w", strings.Join(labels, ","), err)
	}
	defer gptClient.close()
	resp, err := gptClient.request(probeCtx, http.MethodGet, "https://chatgpt.com/api/auth/csrf", requestOptions{
		headers: http.Header{
			"Accept":  []string{"application/json"},
			"Referer": []string{"https://chatgpt.com/"},
		},
	})
	if err != nil {
		return probe, fmt.Errorf("payment proxy %s csrf probe failed: %w", strings.Join(labels, ","), err)
	}
	if resp.status != http.StatusOK {
		return probe, fmt.Errorf("payment proxy %s csrf probe status %d: %s", strings.Join(labels, ","), resp.status, resp.excerpt(300))
	}
	if strings.TrimSpace(stringAt(resp.json, "csrfToken")) == "" {
		return probe, fmt.Errorf("payment proxy %s csrf probe token missing", strings.Join(labels, ","))
	}
	return probe, nil
}

func probePaymentProxyExitIP(ctx context.Context, session *httpSession) paymentProxyProbeResult {
	if session == nil {
		return paymentProxyProbeResult{}
	}
	if resp, err := session.request(ctx, http.MethodGet, "https://cloudflare.com/cdn-cgi/trace", requestOptions{
		headers: http.Header{"Accept": []string{"text/plain,*/*"}},
	}); err == nil && resp != nil && resp.status == http.StatusOK {
		probe := parsePaymentCloudflareTrace(string(resp.body))
		if strings.TrimSpace(probe.IP) != "" {
			return probe
		}
	}
	if resp, err := session.request(ctx, http.MethodGet, "https://api.ipify.org?format=json", requestOptions{
		headers: http.Header{"Accept": []string{"application/json"}},
	}); err == nil && resp != nil && resp.status == http.StatusOK {
		ip := strings.TrimSpace(stringAt(resp.json, "ip"))
		if ip != "" {
			return paymentProxyProbeResult{IP: ip}
		}
	}
	return paymentProxyProbeResult{}
}

func parsePaymentCloudflareTrace(body string) paymentProxyProbeResult {
	var probe paymentProxyProbeResult
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "ip":
			probe.IP = strings.TrimSpace(value)
		case "loc":
			probe.Country = strings.TrimSpace(value)
		}
	}
	return probe
}

func paymentProxyHash(value string) string {
	return hashx.ShortSHA256(value, 12)
}
