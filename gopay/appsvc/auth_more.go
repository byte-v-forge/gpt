package appsvc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) completeLogin(ctx context.Context, state stateMap, otp string) error {
	device, err := s.ensureDevice(state)
	if err != nil {
		return err
	}
	client, err := s.newClient(ctx, "", s.proxyForState(state), device)
	if err != nil {
		return err
	}
	verificationID := stateString(state, "_login_verification_id")
	otpToken := stateString(state, "_login_otp_token")
	method := firstNonEmpty(stateString(state, "_login_verification_method"), "otp_wa")
	twoFAToken := stateString(state, "_login_2fa_token")
	if verificationID == "" || otpToken == "" || twoFAToken == "" {
		return fmt.Errorf("login 2fa state missing")
	}
	verifyResp, err := client.Post(ctx, gotoAuthBaseURL+"/cvs/v1/verify", s.authBody(map[string]any{
		"data":                map[string]any{"otp": strings.TrimSpace(otp), "otp_token": otpToken},
		"flow":                "login_2fa",
		"verification_id":     verificationID,
		"verification_method": method,
	}))
	if err != nil {
		return err
	}
	if verifyResp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", apiError("2fa verify failed", verifyResp))
	}
	verificationToken := verificationTokenFrom(verifyResp.Data())
	if verificationToken == "" {
		return fmt.Errorf("2fa verification_token missing")
	}
	tokenResp, err := client.Request(ctx, http.MethodPost, gotoAuthBaseURL+"/goto-auth/token", s.authBody(map[string]any{
		"ext_user_token": nil,
		"grant_type":     "challenge",
		"token":          twoFAToken,
	}), http.Header{"Verification-Token": []string{"Bearer " + verificationToken}})
	if err != nil {
		return err
	}
	if tokenResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%s", apiError("challenge token failed", tokenResp))
	}
	s.persistLoginReady(state, tokenResp.Data(), stateString(state, "_login_phone"))
	return nil
}

func (s *Server) startSignup(ctx context.Context, state stateMap, phone, name, email, countryCode, otpChannel string) map[string]any {
	cc := phoneCountryCode(s.cfg, countryCode)
	normalized := normalizePhoneWithConfig(s.cfg, phone, cc)
	if normalized == "" {
		return map[string]any{"success": false, "error": "signup phone missing"}
	}
	name, email = s.signupProfile(normalized, name, email)
	if name == "" {
		return map[string]any{"success": false, "error": "signup name missing"}
	}
	s.clearSignupState(state, "")
	s.clearLoginState(state, "")
	device, rawDevice, err := s.newLogonDevice()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	state["device"] = rawDevice
	deleteKeys(state, activeTokenKeys...)
	deleteKeys(state, activeTokenMetaKeys...)
	deleteKeys(state, tmpTokenKeys...)
	deleteKeys(state, tmpTokenMetaKeys...)
	state["_signup_phone"] = normalized
	state["_signup_country_code"] = cc
	state["_signup_name"] = name
	state["_signup_email"] = email
	state["_signup_started_at"] = time.Now().Unix()
	state["stage"] = "signup"
	delete(state, "last_error")
	client, err := s.newClient(ctx, "", s.proxyForState(state), device)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	methodsResp, err := client.Post(ctx, gotoAuthBaseURL+"/cvs/v1/methods", s.authBody(map[string]any{
		"country_code":                 cc,
		"device_verification_token_id": nil,
		"email_address":                nil,
		"flow":                         "signup",
		"phone_number":                 normalized,
	}))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if methodsResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("signup methods failed", methodsResp), "raw_json": safeJSON(methodsResp.Payload)}
	}
	methodsData := methodsResp.Data()
	verificationID := verificationIDFrom(methodsData)
	if verificationID == "" {
		shape := responseShape(methodsResp)
		return map[string]any{"success": false, "error": "signup verification_id missing: " + safeJSON(shape), "response_shape": shape}
	}
	methods := methodsFrom(methodsData)
	method := chooseOTPMethod(methods, otpChannel, "otp_sms")
	if method == "" {
		return map[string]any{"success": false, "error": fmt.Sprintf("otp method unavailable: %v", methods), "response_shape": responseShape(methodsResp)}
	}
	initResp, err := client.Post(ctx, gotoAuthBaseURL+"/cvs/v1/initiate", s.authBody(map[string]any{
		"country_code":                 cc,
		"device_verification_token_id": nil,
		"email_address":                nil,
		"flow":                         "signup",
		"is_multiple_method":           nil,
		"phone_number":                 normalized,
		"verification_id":              verificationID,
		"verification_method":          method,
	}))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if initResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("signup otp initiate failed", initResp), "raw_json": safeJSON(initResp.Payload)}
	}
	otpToken := otpTokenFrom(initResp.Data())
	if otpToken == "" {
		return map[string]any{"success": false, "error": "signup otp_token missing", "raw_json": safeJSON(initResp.Payload)}
	}
	s.persistSignupOTP(state, verificationID, method, otpToken)
	return map[string]any{
		"success": true, "otp_sent": true, "verification_id": verificationID,
		"method": method, "retry_timer_seconds": initResp.Data()["retry_timer_in_seconds"],
		"raw_json": safeJSON(initResp.Payload),
	}
}

func (s *Server) retrySignupOTP(ctx context.Context, state stateMap) map[string]any {
	if stateString(state, "stage") != "signup_otp_pending" {
		return map[string]any{"success": false, "error": fmt.Sprintf("not waiting for signup otp: %s", firstNonEmpty(stateString(state, "stage"), "idle"))}
	}
	otpToken := stateString(state, "_signup_otp_token")
	method := firstNonEmpty(stateString(state, "_signup_verification_method"), "otp_sms")
	if otpToken == "" {
		return map[string]any{"success": false, "error": "signup otp state missing"}
	}
	device, err := s.ensureDevice(state)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	client, err := s.newClient(ctx, "", s.proxyForState(state), device)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	resp, err := client.Post(ctx, gotoAuthBaseURL+"/cvs/v1/retry", s.authBody(map[string]any{
		"flow":                "signup",
		"verification_method": method,
		"data":                map[string]any{"otp_token": otpToken},
	}))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("signup otp retry failed", resp), "raw_json": safeJSON(resp.Payload)}
	}
	if newToken := otpTokenFrom(resp.Data()); newToken != "" {
		state["_signup_otp_token"] = newToken
	}
	now := time.Now().Unix()
	state["_signup_otp_sent_at"] = now
	state["_signup_otp_expires_at"] = now + int64(s.cfg.OTPTimeout.Seconds())
	state["stage"] = "signup_otp_pending"
	delete(state, "last_error")
	return map[string]any{"success": true, "otp_sent": true, "raw_json": safeJSON(resp.Payload)}
}

func (s *Server) completeSignup(ctx context.Context, state stateMap, otp string) map[string]any {
	if stateString(state, "stage") != "signup_otp_pending" {
		return map[string]any{"success": false, "error": fmt.Sprintf("not waiting for signup otp: %s", firstNonEmpty(stateString(state, "stage"), "idle"))}
	}
	otp = strings.TrimSpace(otp)
	if otp == "" {
		return map[string]any{"success": false, "error": "signup otp required"}
	}
	phone := stateString(state, "_signup_phone")
	cc := firstNonEmpty(stateString(state, "_signup_country_code"), phoneCountryCode(s.cfg, ""))
	name := stateString(state, "_signup_name")
	email := stateString(state, "_signup_email")
	verificationID := stateString(state, "_signup_verification_id")
	method := firstNonEmpty(stateString(state, "_signup_verification_method"), "otp_sms")
	otpToken := stateString(state, "_signup_otp_token")
	if phone == "" || verificationID == "" || otpToken == "" {
		return map[string]any{"success": false, "error": "signup otp state missing"}
	}
	device, err := s.ensureDevice(state)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	client, err := s.newClient(ctx, "", s.proxyForState(state), device)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	verifyResp, err := client.Post(ctx, gotoAuthBaseURL+"/cvs/v1/verify", s.authBody(map[string]any{
		"data":                map[string]any{"otp": otp, "otp_token": otpToken},
		"flow":                "signup",
		"verification_id":     verificationID,
		"verification_method": method,
	}))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if verifyResp.StatusCode != http.StatusOK {
		return map[string]any{"success": false, "error": apiError("signup otp verify failed", verifyResp), "raw_json": safeJSON(verifyResp.Payload)}
	}
	verificationToken := verificationTokenFrom(verifyResp.Data())
	if verificationToken == "" {
		return map[string]any{"success": false, "error": "signup verification_token missing", "raw_json": safeJSON(verifyResp.Payload)}
	}
	signupResp, err := client.Request(ctx, http.MethodPost, gojekBaseURL+"/v7/customers/signup", map[string]any{
		"client_name":   s.cfg.GotoClientID,
		"client_secret": s.cfg.GotoClientSecret,
		"data": map[string]any{
			"name":               name,
			"phone":              cc + phone,
			"email":              email,
			"signed_up_country":  cc,
			"onboarding_partner": "gopay_consumer_app",
		},
	}, http.Header{
		"Authorization":      []string{s.signupBasicAuthorization()},
		"Verification-Token": []string{"Bearer " + verificationToken},
	})
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}
	if signupResp.StatusCode != http.StatusCreated {
		return map[string]any{"success": false, "error": apiError("customer signup failed", signupResp), "raw_json": safeJSON(signupResp.Payload)}
	}
	s.storeTokenResponse(state, signupResp.Data(), false)
	state["phone"] = phone
	state["name"] = name
	state["email"] = email
	state["stage"] = "signup_pin_required"
	delete(state, "last_error")
	deleteKeys(state, signupOTPStateKeys...)
	refresh := s.ensureAccessToken(ctx, state, 0, true)
	if !anyBool(refresh["success"]) {
		state["last_error"] = anyString(refresh["error"])
		return map[string]any{"success": false, "error": stateString(state, "last_error"), "raw_json": safeJSON(signupResp.Payload)}
	}
	state["stage"] = "signup_pin_required"
	return map[string]any{"success": true, "phone": phone, "pin_setup_required": true, "raw_json": safeJSON(signupResp.Payload)}
}
