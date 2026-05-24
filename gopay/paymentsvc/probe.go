package paymentsvc

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type tierProbe struct {
	Checked      bool
	PlusActive   bool
	PlanType     string
	Tier         string
	Source       string
	ErrorMessage string
}

type trialProbe struct {
	Checked           bool
	PlusTrialEligible bool
	PlusActive        bool
	PlanType          string
	Amount            int64
	Currency          string
	Source            string
	CheckoutURL       string
	CheckoutSessionID string
	ErrorMessage      string
}

func (s *Server) probeTier(ctx context.Context, cred credential) (tierProbe, error) {
	if cred.accessToken != "" {
		return s.probeTierAccessToken(ctx, cred.accessToken), nil
	}
	return s.probePlusActiveSessionToken(ctx, cred.sessionToken), nil
}

func (s *Server) probePlusTrial(ctx context.Context, cred credential) (trialProbe, error) {
	var sessionProbe tierProbe
	if cred.sessionToken != "" {
		sessionProbe = s.probePlusActiveSessionToken(ctx, cred.sessionToken)
		if sessionProbe.Checked && sessionProbe.PlusActive {
			return trialProbe{Checked: true, PlusTrialEligible: false, PlusActive: true, Source: sessionProbe.Source, PlanType: firstNonEmpty(sessionProbe.Tier, sessionProbe.PlanType)}, nil
		}
		if sessionProbe.ErrorMessage != "" && cred.accessToken == "" {
			return trialProbe{Checked: sessionProbe.Checked, PlusActive: false, Source: sessionProbe.Source, PlanType: firstNonEmpty(sessionProbe.Tier, sessionProbe.PlanType), ErrorMessage: sessionProbe.ErrorMessage}, nil
		}
	}
	ch, err := s.newCharger(ctx, cred, "", "", "", defaultTokenization)
	if err != nil {
		return trialProbe{}, err
	}
	defer ch.close()
	result, err := ch.probePlusTrialCheckout(ctx)
	if err != nil {
		return trialProbe{}, err
	}
	result.PlusActive = sessionProbe.PlusActive
	result.PlanType = firstNonEmpty(sessionProbe.Tier, sessionProbe.PlanType)
	return result, nil
}

func (s *Server) probeTierAccessToken(ctx context.Context, accessToken string) tierProbe {
	if strings.TrimSpace(accessToken) == "" {
		return tierProbe{Source: "wham_usage", ErrorMessage: "access_token is required"}
	}
	fingerprint := s.cfg.CheckoutProfile.fingerprint()
	session, err := newHTTPSession(s.cfg.CheckoutProfile.ProxyURL, fingerprint)
	if err != nil {
		return tierProbe{Source: "wham_usage", ErrorMessage: err.Error()}
	}
	defer session.close()
	setChatGPTBrowserHeaders(session, fingerprint.DeviceID, fingerprint)
	session.headers.Set("Authorization", "Bearer "+accessToken)
	session.headers.Set("Accept", "application/json")
	if accountID := accessTokenAccountID(accessToken); accountID != "" {
		session.headers.Set("ChatGPT-Account-Id", accountID)
	}
	resp, err := session.request(ctx, http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", requestOptions{})
	if err != nil {
		return tierProbe{Source: "wham_usage", ErrorMessage: "wham usage probe failed: " + err.Error()}
	}
	if resp.status != http.StatusOK {
		return tierProbe{Source: "wham_usage", ErrorMessage: fmt.Sprintf("wham usage returned status %d", resp.status)}
	}
	if result := tierResult(firstNonEmpty(stringAt(resp.json, "plan_type"), stringAt(resp.json, "planType")), "wham_usage.plan_type"); result.Checked {
		return result
	}
	return tierProbe{Checked: true, Source: "wham_usage", ErrorMessage: "wham usage returned no plan_type"}
}

func (s *Server) probePlusActiveSessionToken(ctx context.Context, sessionToken string) tierProbe {
	if strings.TrimSpace(sessionToken) == "" {
		return tierProbe{Source: "auth_session", ErrorMessage: "session_token is required"}
	}
	fingerprint := s.cfg.CheckoutProfile.fingerprint()
	deviceID := fingerprint.DeviceID
	fingerprint.DeviceID = deviceID
	session, err := newHTTPSession(s.cfg.CheckoutProfile.ProxyURL, fingerprint)
	if err != nil {
		return tierProbe{Source: "auth_session", ErrorMessage: err.Error()}
	}
	defer session.close()
	setChatGPTBrowserHeaders(session, deviceID, fingerprint)
	session.headers.Set("Accept", "application/json")
	session.headers.Set("Cookie", chatGPTCookieHeader(sessionToken, deviceID))
	resp, err := session.request(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", requestOptions{})
	if err != nil {
		return tierProbe{Source: "auth_session", ErrorMessage: "auth session probe failed: " + err.Error()}
	}
	if resp.status != http.StatusOK {
		return tierProbe{Source: "auth_session", ErrorMessage: fmt.Sprintf("auth session returned status %d", resp.status)}
	}
	accessToken := stringAt(resp.json, "accessToken")
	if accessToken != "" {
		wham := s.probeTierAccessToken(ctx, accessToken)
		if wham.Checked && firstNonEmpty(wham.Tier, wham.PlanType) != "" {
			return wham
		}
		if result := accessTokenTier(accessToken, "accessToken.auth"); result.Checked {
			return result
		}
	}
	result := detectPlusActiveFromSessionPayload(resp.json)
	if !result.Checked {
		result.ErrorMessage = "auth session returned no authenticated user"
	}
	return result
}

func tierResult(value any, source string) tierProbe {
	tier := normalizeTier(value)
	if tier == "" {
		return tierProbe{}
	}
	return tierProbe{Checked: true, PlusActive: tier != "free", PlanType: tier, Tier: tier, Source: source}
}

func normalizeTier(value any) string {
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if text == "" || text == "<nil>" {
		return ""
	}
	for _, tier := range []string{"plus", "pro", "team", "business", "enterprise", "free"} {
		if text == tier {
			return tier
		}
	}
	paidRE := regexp.MustCompile(`(^|[_:/\-\s])(chatgpt[_:/\-\s]*)?(plus|pro|team|business|enterprise)([_:/\-\s]|$)`)
	if match := paidRE.FindStringSubmatch(text); len(match) > 3 {
		return match[3]
	}
	freeRE := regexp.MustCompile(`(^|[_:/\-\s])(free|none|anonymous|unauthenticated)([_:/\-\s]|$)`)
	if freeRE.MatchString(text) {
		return "free"
	}
	return ""
}

func detectPlusActiveFromSessionPayload(payload map[string]any) tierProbe {
	if payload == nil {
		return tierProbe{Source: "auth_session"}
	}
	if result := accessTokenTier(stringAt(payload, "accessToken"), "accessToken.auth"); result.Checked {
		return result
	}
	if account, ok := payload["account"].(map[string]any); ok {
		for _, key := range []string{"planType", "plan_type", "accountPlan", "account_plan", "tier", "plan"} {
			if result := tierResult(account[key], "account."+key); result.Checked {
				return result
			}
		}
	}
	if walkPaidMarker(payload) {
		return tierProbe{Checked: true, PlusActive: true, PlanType: "paid", Tier: "paid", Source: "auth_session:paid_marker"}
	}
	checked := payload["user"] != nil || payload["accessToken"] != nil
	return tierProbe{Checked: checked, PlusActive: false, PlanType: "free", Tier: "free", Source: "auth_session:no_paid_marker"}
}

func walkPaidMarker(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			clean := strings.ToLower(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(key, ""))
			if b, ok := item.(bool); ok && b {
				for _, marker := range []string{"haspaid", "haspaidsubscription", "hasactivepaidsubscription", "isplus", "ispaid", "subscribed"} {
					if clean == marker {
						return true
					}
				}
			}
			if result := tierResult(item, key); result.Checked && result.PlusActive {
				return true
			}
			if walkPaidMarker(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if walkPaidMarker(item) {
				return true
			}
		}
	}
	return false
}
