package appsvc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/gpt/gopay/protocol"
	gopayapp "github.com/byte-v-forge/gpt/gopay/protocol/app"
)

var (
	loginStateKeys = []string{
		"_login_phone", "_login_country_code", "_login_verification_id",
		"_login_verification_method", "_login_otp_token", "_login_2fa_token",
		"_login_started_at", "_login_otp_sent_at", "_login_otp_expires_at",
	}
	signupAccountStateKeys = []string{"_signup_phone", "_signup_country_code", "_signup_name", "_signup_email"}
	signupOTPStateKeys     = []string{"_signup_verification_id", "_signup_verification_method", "_signup_otp_token", "_signup_started_at", "_signup_otp_sent_at", "_signup_otp_expires_at"}
	signupPINStateKeys     = []string{"_signup_pin_verification_id", "_signup_pin_verification_method", "_signup_pin_otp_token", "_signup_pin_challenge_id", "_signup_pin_client_id", "_signup_pin_otp_sent_at", "_signup_pin_otp_expires_at"}
	activeTokenKeys        = []string{"token", "refresh_token", "token_expires_at"}
	activeTokenMetaKeys    = []string{"last_token_refresh_at", "last_token_refresh_error", "last_token_refresh_failed_at"}
	tmpTokenKeys           = []string{"_tmp_token", "_tmp_refresh_token", "_tmp_token_expires_at"}
	tmpTokenMetaKeys       = []string{"_tmp_phone", "_tmp_token_migrated_at"}
)

func (s *Server) checkPhoneByLoginMethods(ctx context.Context, phone, countryCode string) map[string]any {
	cc := phoneCountryCode(s.cfg, countryCode)
	normalized := normalizePhoneWithConfig(s.cfg, phone, cc)
	proxyState := stateMap{}
	attempts := s.proxyAttemptLimit()
	for attempt := 1; attempt <= attempts; attempt++ {
		proxyURL, _, proxyCount, err := s.proxyForAttempt(attempt, proxyState)
		if err != nil {
			return map[string]any{"success": false, "available": false, "status": "rate_limited", "error": err.Error(), "attempts": attempt - 1}
		}
		device, _, err := s.newLogonDevice()
		if err != nil {
			return map[string]any{"success": false, "available": false, "status": "error", "error": err.Error()}
		}
		client, err := s.newClient(ctx, "", proxyURL, device)
		if err != nil {
			return map[string]any{"success": false, "available": false, "status": "error", "error": err.Error()}
		}
		resp, err := client.Post(ctx, gotoAuthBaseURL+"/goto-auth/login/methods", s.authBody(map[string]any{
			"country_code":                 cc,
			"device_verification_token_id": "",
			"email":                        "",
			"phone_number":                 normalized,
		}))
		if err != nil {
			if attempt < attempts && retryableGoPayTransportError(err) {
				time.Sleep(loginMethodsBackoff(attempt))
				continue
			}
			return map[string]any{"success": false, "available": false, "status": "error", "error": err.Error(), "attempts": attempt}
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			return map[string]any{"success": true, "available": false, "status": "registered", "methods": resp.Data()["methods"], "attempts": attempt}
		}
		if loginMethodsInvalidUser(resp) {
			return map[string]any{"success": true, "available": true, "status": "available", "attempts": attempt}
		}
		if isRateLimited(resp) && attempt < attempts {
			time.Sleep(loginMethodsBackoff(attempt))
			continue
		}
		if isRateLimited(resp) {
			return map[string]any{"success": false, "available": false, "status": "rate_limited", "error": loginMethodsRateLimitedError(attempts, proxyCount), "attempts": attempt, "proxy_pool_size": proxyCount}
		}
		return map[string]any{"success": false, "available": false, "status": "error", "error": apiError("login methods failed", resp), "attempts": attempt}
	}
	return map[string]any{"success": false, "available": false, "status": "error", "error": "login methods attempts exhausted"}
}

func (s *Server) startLogin(ctx context.Context, state stateMap, phone, pin, countryCode, otpChannel string) map[string]any {
	cc := phoneCountryCode(s.cfg, countryCode)
	normalized := normalizePhoneWithConfig(s.cfg, phone, cc)
	attempts := s.proxyAttemptLimit()
	var resp *protocol.Response
	var client *gopayapp.Client
	for attempt := 1; attempt <= attempts; attempt++ {
		proxyURL, _, _, err := s.proxyForAttempt(attempt, state)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		device, rawDevice, err := s.newLogonDevice()
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		state["device"] = rawDevice
		state["_login_phone"] = normalized
		state["_login_started_at"] = time.Now().Unix()
		state["stage"] = "login"
		delete(state, "last_error")
		c, err := s.newClient(ctx, "", proxyURL, device)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		resp, err = c.Post(ctx, gotoAuthBaseURL+"/goto-auth/login/methods", s.authBody(map[string]any{
			"country_code":                 cc,
			"device_verification_token_id": "",
			"email":                        "",
			"phone_number":                 normalized,
		}))
		if err != nil {
			if attempt < attempts && retryableGoPayTransportError(err) {
				time.Sleep(loginMethodsBackoff(attempt))
				continue
			}
			return map[string]any{"success": false, "error": err.Error()}
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			client = c
			break
		}
		if isRateLimited(resp) && attempt < attempts {
			time.Sleep(loginMethodsBackoff(attempt))
			continue
		}
		if isRateLimited(resp) {
			return map[string]any{"success": false, "error": loginMethodsRateLimitedError(attempts, len(s.cfg.ProxyPool))}
		}
		if loginMethodsInvalidUser(resp) {
			return map[string]any{"success": false, "not_registered": true, "error": apiError("login methods failed", resp)}
		}
		return map[string]any{"success": false, "error": apiError("login methods failed", resp)}
	}
	if resp == nil || client == nil {
		return map[string]any{"success": false, "error": "login methods failed"}
	}
	data := resp.Data()
	methods := methodsFrom(data)
	verificationID := verificationIDFrom(data)
	if verificationID == "" {
		shape := responseShape(resp)
		return map[string]any{"success": false, "error": "verification_id missing: " + safeJSON(shape), "response_shape": shape}
	}
	if !contains(methods, "goto_pin") {
		return map[string]any{"success": false, "error": fmt.Sprintf("goto_pin unavailable: %v", methods)}
	}
	if strings.TrimSpace(pin) == "" {
		return map[string]any{"success": false, "error": "gopay pin missing"}
	}
	c := client
	initResp, err := c.Request(ctx, http.MethodPost, gotoAuthBaseURL+"/cvs/v1/initiate", s.authBody(map[string]any{
		"country_code":                 cc,
		"device_verification_token_id": nil,
		"email_address":                nil,
		"flow":                         "login_1fa",
		"is_multiple_method":           true,
		"phone_number":                 normalized,
		"verification_id":              verificationID,
		"verification_method":          "goto_pin",
	}), http.Header{"Authorization": []string{""}})
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if initResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("login pin initiate failed", initResp)}
	}
	challengeID := challengeIDFrom(initResp.Data())
	if challengeID == "" {
		shape := responseShape(initResp)
		return map[string]any{"success": false, "error": "pin challenge_id missing: " + safeJSON(shape), "response_shape": shape}
	}
	if pinPage, err := c.Get(ctx, customerBaseURL+"/api/v2/challenges/"+challengeID+"/pin-page/nb"); err != nil || pinPage.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("pin page failed", pinPage)}
	}
	pinResp, err := c.Post(ctx, customerBaseURL+"/api/v1/users/pin/tokens/nb", map[string]any{
		"challenge_id": challengeID,
		"client_id":    s.cfg.PINClientID,
		"pin":          pin,
	})
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if pinResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("pin token failed", pinResp)}
	}
	validationJWT := stringForAnyKey(pinResp.Data(), "token")
	if validationJWT == "" {
		return map[string]any{"success": false, "error": "pin validation token missing"}
	}
	verifyResp, err := c.Post(ctx, gotoAuthBaseURL+"/cvs/v1/verify", s.authBody(map[string]any{
		"data":                map[string]any{"challenge_id": challengeID, "validation_jwt": validationJWT},
		"flow":                "login_1fa",
		"verification_id":     verificationID,
		"verification_method": "goto_pin",
	}))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if verifyResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("login pin verify failed", verifyResp)}
	}
	verificationToken := verificationTokenFrom(verifyResp.Data())
	if verificationToken == "" {
		return map[string]any{"success": false, "error": "1fa verification_token missing"}
	}
	accountResp, err := c.Request(ctx, http.MethodPost, gotoAuthBaseURL+"/goto-auth/accountlist", s.authBody(map[string]any{}), http.Header{"Verification-Token": []string{"Bearer " + verificationToken}})
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if accountResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("accountlist failed", accountResp)}
	}
	accountID := firstAccountID(accountListFrom(accountResp.Data()))
	oneFAToken := oneFATokenFrom(accountResp.Data())
	if accountID == "" || oneFAToken == "" {
		return map[string]any{"success": false, "error": "account_id or 1fa_token missing"}
	}
	tokenResp, err := c.Post(ctx, gotoAuthBaseURL+"/goto-auth/token", s.authBody(map[string]any{
		"account_id":     accountID,
		"ext_user_token": nil,
		"grant_type":     "cvs",
		"token":          oneFAToken,
	}))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if tokenResp.StatusCode == http.StatusCreated {
		s.persistLoginReady(state, tokenResp.Data(), normalized)
		return map[string]any{"success": true, "ready": true, "otp_sent": false}
	}
	twoFAToken := twoFATokenFrom(tokenResp.Data())
	verificationID = verificationIDFrom(tokenResp.Data())
	if tokenResp.StatusCode != http.StatusForbidden || twoFAToken == "" || verificationID == "" {
		return map[string]any{"success": false, "error": apiError("token exchange failed", tokenResp)}
	}
	otpMethods := methodsFrom(tokenResp.Data())
	method := chooseOTPMethod(otpMethods, otpChannel, "otp_wa")
	if method == "" {
		return map[string]any{"success": false, "error": fmt.Sprintf("otp method unavailable: %v", otpMethods), "response_shape": responseShape(tokenResp)}
	}
	otpResp, err := c.Request(ctx, http.MethodPost, gotoAuthBaseURL+"/cvs/v1/initiate", s.authBody(map[string]any{
		"country_code":                 cc,
		"device_verification_token_id": nil,
		"email_address":                nil,
		"flow":                         "login_2fa",
		"is_multiple_method":           nil,
		"phone_number":                 normalized,
		"verification_id":              verificationID,
		"verification_method":          method,
	}), http.Header{"Authorization": []string{""}})
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if otpResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("2fa otp initiate failed", otpResp)}
	}
	otpToken := otpTokenFrom(otpResp.Data())
	if otpToken == "" {
		return map[string]any{"success": false, "error": "2fa otp_token missing"}
	}
	s.persistLoginOTP(state, normalized, cc, verificationID, method, otpToken, twoFAToken)
	return map[string]any{"success": true, "ready": false, "otp_sent": true, "verification_id": verificationID, "method": method}
}
