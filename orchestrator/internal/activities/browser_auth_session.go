package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (f *browserAuthFlow) startSession(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	ctx, cancel := context.WithTimeout(f.ctx, cfg.CommandTimeout)
	defer cancel()
	resp, err := client.StartBrowserSession(ctx, &browserautomationv1.StartBrowserSessionRequest{
		RequestId: "gpt-browser-auth-" + f.flowID,
		Profile: &browserautomationv1.BrowserProfile{
			BrowserKind: browserautomationv1.BrowserKind_BROWSER_KIND_FIREFOX,
			Locale:      cfg.Locale,
			Timezone:    cfg.Timezone,
			UserAgent:   cfg.UserAgent,
			Viewport: &browserautomationv1.BrowserViewport{
				Width:  int32(cfg.WindowWidth),
				Height: int32(cfg.WindowHeight),
			},
			ProxyRef:         cfg.ProxyRef,
			ExtraHttpHeaders: browserAuthHeaders(cfg),
			InitScripts:      []string{browserAuthLanguageOverrideScript(cfg.Locale)},
		},
		Labels: map[string]string{
			"domain":                  "gpt",
			"workflow":                "browser_auth",
			"mode":                    f.mode,
			"job_id":                  f.jobID,
			"fingerprint.device_id":   cfg.DeviceID,
			"fingerprint.tls_profile": cfg.TLSProfileName,
		},
		Ttl: durationpb.New(cfg.SessionTTL),
	})
	if err != nil {
		return err
	}
	if resp.GetError() != nil {
		return errors.New(resp.GetError().GetMessage())
	}
	sessionID := resp.GetSession().GetSessionId()
	if sessionID == "" {
		return fmt.Errorf("browser automation returned empty session_id")
	}
	f.mu.Lock()
	f.sessionID = sessionID
	f.mu.Unlock()
	return nil
}

func browserAuthHeaders(cfg BrowserAuthConfig) map[string]string {
	headers := map[string]string{"Accept-Language": cfg.AcceptLanguage}
	if cfg.SecCHUA != "" {
		headers["sec-ch-ua"] = cfg.SecCHUA
		headers["sec-ch-ua-mobile"] = "?0"
	}
	if cfg.SecCHPlatform != "" {
		headers["sec-ch-ua-platform"] = cfg.SecCHPlatform
	}
	return headers
}

func (f *browserAuthFlow) stopSession(client browserautomationv1.BrowserAutomationServiceClient) {
	sessionID := f.getSessionID()
	if sessionID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = client.StopBrowserSession(ctx, &browserautomationv1.StopBrowserSessionRequest{
		SessionId: sessionID,
		Reason:    "gpt browser auth finished",
	})
}
