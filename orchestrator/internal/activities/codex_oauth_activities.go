package activities

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
	smsv1 "github.com/byte-v-forge/sms/gen/go/byte/v/forge/contracts/sms/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"orchestrator/db"
	"orchestrator/pb"
)

const (
	codexOAuthLeaseAvailable = "available"
	codexOAuthLeaseInUse     = "in_use"
	codexOAuthLeaseExhausted = "exhausted"
	codexOAuthLeaseFailed    = "failed"
	codexOAuthLeaseExpired   = "expired"

	codexOAuthAuthSecretPrefix = "codex_oauth_auth_json:"
)

type codexOAuthPKCE struct {
	verifier  string
	challenge string
}

type codexOAuthTokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) CodexOAuthAcquirePhoneActivity(ctx context.Context, input CodexOAuthAcquirePhoneInput) (*CodexOAuthPhoneLease, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	reuseLimit := input.GetMaxReuseCount()
	if reuseLimit <= 0 {
		reuseLimit = int32(cfg.PhoneMaxReuseCount)
	}
	data := map[string]any{
		"profile_key":          cfg.PhoneProfileKey,
		"country_iso2":         cfg.PhoneCountryISO2,
		"country_calling_code": cfg.PhoneCountryCallingCode,
		"max_reuse_count":      reuseLimit,
		"label":                label,
	}
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthAcquirePhone, false, true)
	var lease *CodexOAuthPhoneLease
	_, err := step.run(func() (any, error) {
		var err error
		lease, err = s.acquireReusableCodexPhone(ctx, input.GetJobId(), input.GetAccountId(), label, reuseLimit, cfg)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		data["activation_id"] = lease.GetActivationId()
		data["phone_reused"] = lease.GetReused()
		data["phone_reuse_count"] = lease.GetReuseCount()
		data["phone_reuse_limit"] = lease.GetReuseLimit()
		data["phone_expires_at_unix"] = lease.GetExpiresAtUnix()
		data["phone_mask"] = maskPhone(lease.GetPhoneE164(), lease.GetPhoneNational())
		return data, nil
	})
	return lease, err
}

func (s *Server) CodexOAuthRunActivity(ctx context.Context, input CodexOAuthRunInput) (CodexOAuthRunOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthRunOutput{
		PhoneLabel:      label,
		PhoneReuseLimit: input.GetPhone().GetReuseLimit(),
	}
	data := map[string]any{
		"label":                 label,
		"profile_key":           input.GetPhone().GetProfileKey(),
		"phone_reused":          input.GetPhone().GetReused(),
		"phone_reuse_count":     input.GetPhone().GetReuseCount(),
		"phone_reuse_limit":     input.GetPhone().GetReuseLimit(),
		"phone_expires_at_unix": input.GetPhone().GetExpiresAtUnix(),
		"phone_activation_id":   input.GetPhone().GetActivationId(),
		"phone_country_iso2":    input.GetPhone().GetCountryIso2(),
		"phone_country_code":    input.GetPhone().GetCountryCallingCode(),
		"phone_mask":            maskPhone(input.GetPhone().GetPhoneE164(), input.GetPhone().GetPhoneNational()),
		"auth_secret_written":   false,
		"account_auth_written":  false,
		"add_phone_confirmed":   false,
		"callback_url_captured": false,
	}
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthBrowser, false, true)
	_, err := step.run(func() (any, error) {
		account, err := s.getAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if strings.TrimSpace(account.GetEmail()) == "" || strings.TrimSpace(account.GetPassword()) == "" {
			err = fmt.Errorf("account email/password is required")
			data["error_message"] = err.Error()
			return data, err
		}
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthBrowser, "running codex oauth browser flow", data)
		defer stopHeartbeat()
		result, err := s.runCodexOAuthBrowser(ctx, account, input.GetJobId(), label, input.GetPhone(), cfg, data)
		if err != nil {
			data["error_message"] = err.Error()
			output.ErrorMessage = err.Error()
			return data, err
		}
		output.Success = true
		output.AuthSecretKey = result.authSecretKey
		output.PhoneLabel = label
		output.PhoneReuseCount = result.phoneReuseCount
		output.PhoneReuseLimit = result.phoneReuseLimit
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}

func (s *Server) CodexOAuthReleasePhoneActivity(ctx context.Context, input CodexOAuthReleasePhoneInput) error {
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthReleasePhone, true, false)
	_, err := step.run(func() (any, error) {
		if strings.TrimSpace(input.GetActivationId()) == "" {
			return map[string]any{"released": false, "reason": "activation_id_missing"}, nil
		}
		data := map[string]any{
			"activation_id": input.GetActivationId(),
			"label":         input.GetLabel(),
			"released":      true,
		}
		if err := s.releaseCodexPhoneAfterFailure(ctx, input.GetActivationId(), input.GetAccountId(), input.GetJobId(), input.GetLabel(), input.GetErrorMessage()); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		return data, nil
	})
	return err
}

func (s *Server) acquireReusableCodexPhone(ctx context.Context, jobID, accountID, label string, reuseLimit int32, cfg CodexOAuthConfig) (*CodexOAuthPhoneLease, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	now := time.Now().Unix()
	minRemaining := codexOAuthPhoneMinRemainingSeconds(cfg)
	reusableAfter := now + int64(minRemaining)
	var reused db.CodexOAuthPhoneLease
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&db.CodexOAuthPhoneLease{}).
			Where("status = ? AND expires_at > 0 AND expires_at <= ?", codexOAuthLeaseAvailable, reusableAfter).
			Updates(map[string]any{
				"status":            codexOAuthLeaseExpired,
				"last_failure_kind": "phone_expired",
				"last_error":        fmt.Sprintf("phone lease expires before required reuse window: expires_at <= %d", reusableAfter),
			}).Error; err != nil {
			return err
		}
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND use_count < max_use_count", codexOAuthLeaseAvailable).
			Where("profile_key = ?", strings.TrimSpace(cfg.PhoneProfileKey)).
			Where("expires_at > ?", reusableAfter).
			Order("updated_at DESC")
		if cfg.PhoneCountryISO2 != "" {
			query = query.Where("country_iso2 = ?", cfg.PhoneCountryISO2)
		}
		if err := query.First(&reused).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		reused.Status = codexOAuthLeaseInUse
		reused.LastJobID = jobID
		reused.LastAccountID = accountID
		reused.LastError = ""
		reused.LastFailureKind = ""
		if strings.TrimSpace(label) != "" {
			reused.Label = label
		}
		return tx.Save(&reused).Error
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reused.ActivationID) != "" {
		return codexOAuthPhoneLeaseFromRow(reused, reused.UseCount > 0), nil
	}

	activation, err := s.acquireCodexSMSActivation(ctx, jobID, accountID, label, reuseLimit, cfg)
	if err != nil {
		return nil, err
	}
	row := db.CodexOAuthPhoneLease{
		ActivationID:       activation.GetActivationId(),
		PhoneE164:          activation.GetPhoneNumber().GetE164Number(),
		PhoneNational:      activation.GetPhoneNumber().GetNationalNumber(),
		CountryISO2:        activation.GetPhoneNumber().GetCountryIso2(),
		CountryCallingCode: activation.GetPhoneNumber().GetCountryCallingCode(),
		ProfileKey:         cfg.PhoneProfileKey,
		Status:             codexOAuthLeaseInUse,
		Label:              label,
		UseCount:           0,
		MaxUseCount:        reuseLimit,
		ExpiresAt:          codexOAuthActivationExpiresAt(activation),
		LastJobID:          jobID,
		LastAccountID:      accountID,
	}
	if row.CountryISO2 == "" {
		row.CountryISO2 = cfg.PhoneCountryISO2
	}
	if row.CountryCallingCode == "" {
		row.CountryCallingCode = cfg.PhoneCountryCallingCode
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return codexOAuthPhoneLeaseFromRow(row, false), nil
}

func (s *Server) acquireCodexSMSActivation(ctx context.Context, jobID, accountID, label string, reuseLimit int32, cfg CodexOAuthConfig) (*smsv1.SmsActivation, error) {
	if s.smsClient == nil {
		return nil, fmt.Errorf("sms client not configured")
	}
	request := &smsv1.AcquireNumberRequest{
		RequestId:     "codex-oauth-" + strings.TrimSpace(jobID),
		ProfileKey:    strings.TrimSpace(cfg.PhoneProfileKey),
		LeaseDuration: durationOrNil(defaultSMSLeaseDuration),
		Target: &smsv1.SmsTarget{
			ApplicationKey:     "openai",
			CountryIso2:        cfg.PhoneCountryISO2,
			CountryCallingCode: cfg.PhoneCountryCallingCode,
			MaxPrice: &smsv1.DecimalMoney{
				CurrencyCode:  "USD",
				AmountDecimal: cfg.PhoneMaxPriceUSD,
			},
		},
		Labels: map[string]string{
			"domain":          "gpt",
			"workflow":        "codex_oauth",
			"action":          actionCodexOAuthAddPhone,
			"job_id":          jobID,
			"account_id":      accountID,
			"label":           label,
			"profile_key":     cfg.PhoneProfileKey,
			"max_reuse_count": fmt.Sprintf("%d", reuseLimit),
		},
	}
	normalizeAcquireNumberRequest(request)
	resp, err := s.smsClient.AcquireNumber(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("AcquireNumber: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("AcquireNumber: empty response")
	}
	if resp.GetError() != nil {
		return nil, fmt.Errorf("AcquireNumber: %s", smsErrorText(resp.GetError()))
	}
	if resp.GetActivation() == nil {
		return nil, fmt.Errorf("AcquireNumber: empty activation")
	}
	return resp.GetActivation(), nil
}

type codexOAuthBrowserResult struct {
	authSecretKey   string
	phoneReuseCount int32
	phoneReuseLimit int32
}

func (s *Server) runCodexOAuthBrowser(ctx context.Context, account *pb.Account, jobID, label string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, data map[string]any) (codexOAuthBrowserResult, error) {
	if s.browserAutomationClient == nil {
		return codexOAuthBrowserResult{}, fmt.Errorf("browser automation client is not configured")
	}
	if err := ensureCodexOAuthPhoneLeaseUsable(phone, cfg); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	pkce, err := newCodexOAuthPKCE()
	if err != nil {
		return codexOAuthBrowserResult{}, err
	}
	state, err := randomURLToken(32)
	if err != nil {
		return codexOAuthBrowserResult{}, err
	}
	authorizeURL := buildCodexOAuthAuthorizeURL(cfg, pkce, state)
	flow := newBrowserAuthFlow("codex_oauth_add_phone", jobID, account)
	if err := flow.startSession(s.browserAutomationClient, s.browserAuthConfig); err != nil {
		return codexOAuthBrowserResult{}, err
	}
	defer flow.stopSession(s.browserAutomationClient)

	phoneUsed := false
	success := false
	failureMessage := "codex oauth browser did not complete"
	defer func() {
		if !success {
			_ = s.releaseCodexPhone(ctx, phone, account.GetAccountId(), jobID, label, phoneUsed, failureMessage)
		}
	}()

	if err := flow.openCodexOAuthEntry(s.browserAutomationClient, s.browserAuthConfig, authorizeURL); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if err := flow.ensureCodexOAuthLoggedIn(ctx, s, account, jobID, data); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	stage, err := flow.detectCodexOAuthStage(s.browserAutomationClient, s.browserAuthConfig)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if stage == "add_phone" {
		phoneUsed = true
		if err := flow.completeCodexOAuthAddPhone(ctx, s, jobID, phone, cfg, data); err != nil {
			failureMessage = err.Error()
			return codexOAuthBrowserResult{}, err
		}
		data["add_phone_confirmed"] = true
		if err := s.markCodexPhoneSuccess(ctx, phone, account.GetAccountId(), jobID, label); err != nil {
			failureMessage = err.Error()
			return codexOAuthBrowserResult{}, err
		}
	} else {
		data["add_phone_confirmed"] = false
		if err := s.releaseCodexPhone(ctx, phone, account.GetAccountId(), jobID, label, false, "add phone not required"); err != nil {
			return codexOAuthBrowserResult{}, err
		}
	}
	callbackURL, err := flow.completeCodexOAuthConsentAndCallback(s.browserAutomationClient, s.browserAuthConfig)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	data["callback_url_captured"] = true
	code, returnedState, err := codexOAuthCodeFromCallback(callbackURL)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if returnedState != state {
		failureMessage = "codex oauth state mismatch"
		return codexOAuthBrowserResult{}, fmt.Errorf("codex oauth state mismatch")
	}
	tokens, err := exchangeCodexOAuthToken(ctx, cfg, code, pkce.verifier)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	authJSON, err := buildCodexAuthJSON(tokens)
	if err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	if err := s.updateAccount(ctx, &pb.Account{
		AccountId:              account.GetAccountId(),
		CodexAuthJson:          string(authJSON),
		CodexAuthUpdatedAtUnix: time.Now().Unix(),
	}); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, fmt.Errorf("save codex auth json to account db: %w", err)
	}
	data["account_auth_written"] = true
	secretKey := codexOAuthAuthSecretPrefix + account.GetAccountId()
	if err := s.saveRuntimeSecret(ctx, secretKey, string(authJSON)); err != nil {
		failureMessage = err.Error()
		return codexOAuthBrowserResult{}, err
	}
	data["auth_secret_key"] = secretKey
	data["auth_secret_written"] = true
	success = true
	return codexOAuthBrowserResult{
		authSecretKey:   secretKey,
		phoneReuseCount: phone.GetReuseCount() + 1,
		phoneReuseLimit: phone.GetReuseLimit(),
	}, nil
}

func (f *browserAuthFlow) openCodexOAuthEntry(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, authorizeURL string) error {
	results, err := f.execute(client, cfg, "codex-oauth-open", []*browserautomationv1.BrowserCommand{
		navigateCommand("open-codex-oauth", authorizeURL, cfg.CommandTimeout),
		clickCommand("reject-cookies", browserAuthRejectCookiesSelector(), 3*time.Second, true),
		waitTimeoutCommand("wait-oauth-entry", 500*time.Millisecond),
		getPageStateCommand("oauth-entry-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	state := browserAuthPageStateData(results, "oauth-entry-state")
	if browserAuthPageHasAny(state, "error", "blocked") {
		return browserAuthStepError(f.mode, "entry", "oauth_entry_error", state)
	}
	return nil
}

func (f *browserAuthFlow) ensureCodexOAuthLoggedIn(ctx context.Context, s *Server, account *pb.Account, jobID string, data map[string]any) error {
	cfg := s.browserAuthConfig
	stage, err := f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
	if err != nil {
		return err
	}
	if stage != "email" {
		data["login_stage"] = stage
		return nil
	}
	if err := f.submitCodexOAuthEmail(s.browserAutomationClient, cfg, account.GetEmail()); err != nil {
		return err
	}
	issuedAfter, err := f.submitCodexOAuthPassword(s.browserAutomationClient, cfg, account.GetPassword())
	if err != nil {
		return err
	}
	stage, err = f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
	if err != nil {
		return err
	}
	if stage == "email_otp" {
		otp, err := s.waitCodexOAuthEmailOTP(ctx, jobID, account.GetEmail(), issuedAfter)
		if err != nil {
			return err
		}
		if err := f.submitCodexOAuthOTP(s.browserAutomationClient, cfg, otp); err != nil {
			return err
		}
		stage, err = f.detectCodexOAuthStage(s.browserAutomationClient, cfg)
		if err != nil {
			return err
		}
	}
	data["login_stage"] = stage
	return nil
}

func (f *browserAuthFlow) submitCodexOAuthEmail(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, email string) error {
	results, err := f.execute(client, cfg, "codex-oauth-email", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-input", browserAuthEmailSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-email", browserAuthEmailSelector(), email, 10*time.Second, false),
		clickCommand("click-email-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-password-or-otp", selectorGroup(45*time.Second, browserAuthLoginPasswordSelector(), browserAuthLoginOTPSelector()), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 45*time.Second, true),
		getPageStateCommand("email-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if browserAuthAnyCommandSucceeded(results, "wait-password-or-otp") {
		return nil
	}
	return browserAuthStepError(f.mode, "email", "next_step_missing", browserAuthPageStateData(results, "email-state"))
}

func (f *browserAuthFlow) submitCodexOAuthPassword(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, password string) (int64, error) {
	issuedAfter := time.Now().Add(-time.Second).Unix()
	results, err := f.execute(client, cfg, "codex-oauth-password", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-password-input", browserAuthLoginPasswordSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-password", browserAuthLoginPasswordSelector(), password, 10*time.Second, false),
		clickCommand("click-password-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-post-password", codexOAuthStageSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("password-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return issuedAfter, err
	}
	if browserAuthCommandSucceeded(results, "wait-post-password") {
		return issuedAfter, nil
	}
	return issuedAfter, browserAuthStepError(f.mode, "password", "next_step_missing", browserAuthPageStateData(results, "password-state"))
}

func (s *Server) waitCodexOAuthEmailOTP(ctx context.Context, _ string, email string, issuedAfter int64) (string, error) {
	wait := s.regOTPTimeout
	if wait <= 0 {
		wait = defaultCodexOAuthPhoneWaitSeconds
	}
	if s.mailboxClient == nil {
		return "", fmt.Errorf("mailbox client not configured")
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(wait+5)*time.Second)
	defer cancel()
	resp, err := s.mailboxClient.WaitForMailboxEmail(reqCtx, &pb.WaitForEmailRequest{
		EmailAddress:    email,
		TimeoutSeconds:  wait,
		IssuedAfterUnix: issuedAfter,
		SignalKind:      pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP,
	})
	if err != nil {
		return "", err
	}
	code := extractOTPFromEmailMessage(resp.GetMessage())
	if !resp.GetFound() || code == "" {
		return "", fmt.Errorf("codex oauth email otp not found")
	}
	return code, nil
}

func (f *browserAuthFlow) submitCodexOAuthOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, otp string) error {
	results, err := f.execute(client, cfg, "codex-oauth-email-otp", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-email-otp", browserAuthLoginOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-email-otp", browserAuthLoginOTPSelector(), otp, 10*time.Second, false),
		clickCommand("click-email-otp-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-post-email-otp", codexOAuthStageSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("email-otp-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	if browserAuthCommandSucceeded(results, "wait-post-email-otp") {
		return nil
	}
	return browserAuthStepError(f.mode, "email_otp", "next_step_missing", browserAuthPageStateData(results, "email-otp-state"))
}

func (f *browserAuthFlow) completeCodexOAuthAddPhone(ctx context.Context, s *Server, jobID string, phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig, data map[string]any) error {
	national := phone.GetPhoneNational()
	if strings.TrimSpace(national) == "" {
		national = strings.TrimPrefix(phone.GetPhoneE164(), "+"+phone.GetCountryCallingCode())
	}
	if strings.TrimSpace(national) == "" {
		return fmt.Errorf("sms phone number is empty")
	}
	results, err := f.execute(s.browserAutomationClient, s.browserAuthConfig, "codex-oauth-add-phone", []*browserautomationv1.BrowserCommand{
		selectOptionGroupCommand("select-phone-country", codexOAuthPhoneCountrySelector(), []string{phone.GetCountryIso2()}, []string{"Thailand (+66)", "Thailand"}, nil, 5*time.Second, true),
		clickCommand("open-phone-country-dropdown", selectorGroup(2*time.Second, roleSelector("button", "United States (+1)", true), roleSelector("button", "Thailand (+66)", true)), 3*time.Second, true),
		clickCommand("click-phone-country-th", selectorGroup(2*time.Second, roleSelector("option", "Thailand (+66)", true), textSelector("Thailand (+66)", true)), 3*time.Second, true),
		waitForSelectorCommand("wait-phone-input", codexOAuthPhoneInputSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-phone", codexOAuthPhoneInputSelector(), national, 10*time.Second, false),
		clickCommand("click-phone-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorCommand("wait-phone-otp", codexOAuthPhoneOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 45*time.Second, true),
		getPageStateCommand("phone-submit-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	submitState := browserAuthPageStateData(results, "phone-submit-state")
	if !browserAuthCommandSucceeded(results, "wait-phone-otp") {
		state := "phone_otp_input_missing"
		if failure := codexOAuthPhonePageFailureState(submitState); failure != "" {
			state = failure
		}
		return browserAuthStepError(f.mode, "add_phone", state, submitState)
	}
	if phone.GetReused() {
		if err := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-additional-"+jobID); err != nil {
			data["sms_request_additional_error"] = err.Error()
		}
	} else if err := s.markSMSMessageSent(ctx, phone.GetActivationId(), "codex-oauth-sent-"+jobID); err != nil {
		data["sms_mark_sent_error"] = err.Error()
	}
	code, err := s.waitSMSCode(ctx, phone.GetActivationId(), cfg.PhoneFirstWaitSeconds)
	if err != nil {
		data["sms_first_wait_error"] = err.Error()
		if resendErr := f.resendCodexOAuthPhoneCode(s.browserAutomationClient, s.browserAuthConfig); resendErr != nil {
			return resendErr
		}
		if addErr := s.requestAdditionalSMSCode(ctx, phone.GetActivationId(), "codex-oauth-resend-"+jobID); addErr != nil {
			data["sms_resend_request_error"] = addErr.Error()
		}
		code, err = s.waitSMSCode(ctx, phone.GetActivationId(), cfg.PhoneResendWaitSeconds)
		if err != nil {
			return fmt.Errorf("phone_sms_timeout: %w", err)
		}
	}
	return f.submitCodexOAuthPhoneOTP(s.browserAutomationClient, s.browserAuthConfig, code)
}

func (f *browserAuthFlow) resendCodexOAuthPhoneCode(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) error {
	_, err := f.execute(client, cfg, "codex-oauth-phone-resend", []*browserautomationv1.BrowserCommand{
		clickCommand("click-resend-phone", selectorGroup(5*time.Second, roleSelector("button", "Resend text message", true), textSelector("Resend text message", true), roleSelector("button", "Resend", true), textSelector("Resend", true)), 10*time.Second, false),
		waitTimeoutCommand("wait-after-phone-resend", 500*time.Millisecond),
	})
	return err
}

func (f *browserAuthFlow) submitCodexOAuthPhoneOTP(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, code string) error {
	results, err := f.execute(client, cfg, "codex-oauth-phone-otp", []*browserautomationv1.BrowserCommand{
		waitForSelectorCommand("wait-phone-code", codexOAuthPhoneOTPSelector(), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 20*time.Second, false),
		fillCommand("fill-phone-code", codexOAuthPhoneOTPSelector(), normalizeOTP(code), 10*time.Second, false),
		clickCommand("click-phone-code-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		waitForSelectorGroupCommand("wait-post-phone", codexOAuthStageSelectorGroup(60*time.Second), browserautomationv1.BrowserSelectorState_BROWSER_SELECTOR_STATE_VISIBLE, 60*time.Second, true),
		getPageStateCommand("phone-otp-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return err
	}
	state := browserAuthPageStateData(results, "phone-otp-state")
	if browserAuthCommandSucceeded(results, "wait-post-phone") {
		return nil
	}
	failure := codexOAuthPhonePageFailureState(state)
	if failure == "" {
		failure = "next_step_missing"
	}
	return browserAuthStepError(f.mode, "phone_otp", failure, state)
}

func (f *browserAuthFlow) completeCodexOAuthConsentAndCallback(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	stage, err := f.detectCodexOAuthStage(client, cfg)
	if err != nil {
		return "", err
	}
	if stage == "consent" {
		if _, err := f.execute(client, cfg, "codex-oauth-consent", []*browserautomationv1.BrowserCommand{
			clickCommand("click-consent-continue", browserAuthEmailSubmitSelector(), 10*time.Second, false),
		}); err != nil {
			return "", err
		}
	}
	results, err := f.execute(client, cfg, "codex-oauth-callback", []*browserautomationv1.BrowserCommand{
		waitForURLCommand("wait-callback-url", "http://localhost:*/auth/callback*", false, 90*time.Second, true),
		getPageStateCommand("callback-state", true, false, false, 5*time.Second),
	})
	if err != nil {
		return "", err
	}
	rawURL := stringMapValue(commandResultMap(results, "callback-state"), "url")
	if rawURL == "" {
		rawURL = stringMapValue(commandResultMap(results, "wait-callback-url"), "current_url")
	}
	if rawURL == "" {
		return "", browserAuthStepError(f.mode, "callback", "callback_url_missing", browserAuthPageStateData(results, "callback-state"))
	}
	return rawURL, nil
}

func (f *browserAuthFlow) detectCodexOAuthStage(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig) (string, error) {
	results, err := f.execute(client, cfg, "codex-oauth-detect-stage", []*browserautomationv1.BrowserCommand{
		countElementsCommand("count-email", browserAuthEmailSelector(), 2*time.Second, true),
		countElementsCommand("count-password", browserAuthLoginPasswordSelector(), 2*time.Second, true),
		countElementsCommand("count-email-otp", browserAuthLoginOTPSelector(), 2*time.Second, true),
		countElementsCommand("count-phone", codexOAuthPhoneInputSelector(), 2*time.Second, true),
		countElementsCommand("count-consent", codexOAuthConsentSignalSelector(), 2*time.Second, true),
		getPageStateCommand("stage-state", true, true, false, 5*time.Second),
	})
	if err != nil {
		return "", err
	}
	rawURL := stringMapValue(commandResultMap(results, "stage-state"), "url")
	if strings.Contains(rawURL, "/auth/callback") {
		return "callback", nil
	}
	if browserAuthMatchedCount(results, "count-phone") > 0 || browserAuthPageHasAny(browserAuthPageStateData(results, "stage-state"), "Add your phone", "Phone number") {
		return "add_phone", nil
	}
	if strings.Contains(rawURL, "/sign-in-with-chatgpt/") ||
		browserAuthMatchedCount(results, "count-consent") > 0 ||
		browserAuthPageHasAny(browserAuthPageStateData(results, "stage-state"), "Codex CLI", "Sign in with ChatGPT") {
		return "consent", nil
	}
	if browserAuthMatchedCount(results, "count-email-otp") > 0 {
		return "email_otp", nil
	}
	if browserAuthMatchedCount(results, "count-password") > 0 {
		return "password", nil
	}
	if browserAuthMatchedCount(results, "count-email") > 0 {
		return "email", nil
	}
	return "unknown", nil
}

func (s *Server) markCodexPhoneSuccess(ctx context.Context, phone *CodexOAuthPhoneLease, accountID, jobID, label string) error {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" || s.db == nil {
		return nil
	}
	var row db.CodexOAuthPhoneLease
	if err := s.db.WithContext(ctx).First(&row, "activation_id = ?", phone.GetActivationId()).Error; err != nil {
		return err
	}
	row.UseCount++
	row.LastJobID = jobID
	row.LastAccountID = accountID
	row.LastError = ""
	row.LastFailureKind = ""
	row.Label = strings.TrimSpace(label)
	if row.Label == "" {
		row.Label = phone.GetLabel()
	}
	if row.UseCount >= row.MaxUseCount {
		row.Status = codexOAuthLeaseExhausted
		_ = s.completeSMSActivation(ctx, row.ActivationID, "codex-oauth-exhausted-"+jobID)
	} else {
		row.Status = codexOAuthLeaseAvailable
	}
	phone.ReuseCount = row.UseCount
	phone.ReuseLimit = row.MaxUseCount
	return s.db.WithContext(ctx).Save(&row).Error
}

func (s *Server) releaseCodexPhoneAfterFailure(ctx context.Context, activationID, accountID, jobID, label, message string) error {
	return s.releaseCodexPhone(ctx, &CodexOAuthPhoneLease{ActivationId: activationID, Label: label}, accountID, jobID, label, codexOAuthFailureLikelyUsedPhone(message), message)
}

func (s *Server) releaseCodexPhone(ctx context.Context, phone *CodexOAuthPhoneLease, accountID, jobID, label string, phoneUsed bool, message string) error {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" || s.db == nil {
		return nil
	}
	var row db.CodexOAuthPhoneLease
	if err := s.db.WithContext(ctx).First(&row, "activation_id = ?", phone.GetActivationId()).Error; err != nil {
		return err
	}
	row.LastJobID = jobID
	row.LastAccountID = accountID
	row.LastError = compactBrowserAuthText(message, 500)
	failureKind, terminalStatus := codexOAuthPhoneFailureDisposition(message)
	row.LastFailureKind = failureKind
	if strings.TrimSpace(label) != "" {
		row.Label = strings.TrimSpace(label)
	}
	if row.Status != codexOAuthLeaseInUse {
		return s.db.WithContext(ctx).Save(&row).Error
	}
	if terminalStatus != "" {
		row.Status = terminalStatus
		if terminalStatus == codexOAuthLeaseExhausted {
			_ = s.completeSMSActivation(ctx, row.ActivationID, "codex-oauth-page-exhausted-"+jobID)
		}
		return s.db.WithContext(ctx).Save(&row).Error
	}
	if row.ExpiresAt > 0 && row.ExpiresAt <= time.Now().Unix()+int64(codexOAuthPhoneMinRemainingSeconds(s.codexOAuthConfig.withDefaults())) {
		row.Status = codexOAuthLeaseExpired
		row.LastFailureKind = codexOAuthFirstNonEmpty(row.LastFailureKind, "phone_expired")
		return s.db.WithContext(ctx).Save(&row).Error
	}
	switch {
	case !phoneUsed:
		row.Status = codexOAuthLeaseAvailable
	case row.UseCount > 0 && row.UseCount < row.MaxUseCount:
		row.Status = codexOAuthLeaseAvailable
	default:
		if row.UseCount >= row.MaxUseCount {
			row.Status = codexOAuthLeaseExhausted
		} else {
			row.Status = codexOAuthLeaseFailed
		}
	}
	return s.db.WithContext(ctx).Save(&row).Error
}

func codexOAuthPhoneLeaseFromRow(row db.CodexOAuthPhoneLease, reused bool) *CodexOAuthPhoneLease {
	return &CodexOAuthPhoneLease{
		ActivationId:       row.ActivationID,
		PhoneE164:          row.PhoneE164,
		PhoneNational:      row.PhoneNational,
		CountryIso2:        row.CountryISO2,
		CountryCallingCode: row.CountryCallingCode,
		Label:              row.Label,
		ReuseCount:         row.UseCount,
		ReuseLimit:         row.MaxUseCount,
		Reused:             reused,
		ProfileKey:         row.ProfileKey,
		ExpiresAtUnix:      row.ExpiresAt,
	}
}

func codexOAuthActivationExpiresAt(activation *smsv1.SmsActivation) int64 {
	if activation != nil && activation.GetExpiresAt() != nil {
		return activation.GetExpiresAt().AsTime().Unix()
	}
	return time.Now().Add(defaultSMSLeaseDuration).Unix()
}

func ensureCodexOAuthPhoneLeaseUsable(phone *CodexOAuthPhoneLease, cfg CodexOAuthConfig) error {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" {
		return fmt.Errorf("codex oauth phone lease is missing")
	}
	if phone.GetReuseLimit() > 0 && phone.GetReuseCount() >= phone.GetReuseLimit() {
		return fmt.Errorf("phone_reuse_exhausted: reuse_count=%d reuse_limit=%d", phone.GetReuseCount(), phone.GetReuseLimit())
	}
	expiresAt := phone.GetExpiresAtUnix()
	if expiresAt > 0 && expiresAt <= time.Now().Unix()+int64(codexOAuthPhoneMinRemainingSeconds(cfg)) {
		return fmt.Errorf("phone_expired: expires_at=%d min_remaining_seconds=%d", expiresAt, codexOAuthPhoneMinRemainingSeconds(cfg))
	}
	return nil
}

func codexOAuthPhoneMinRemainingSeconds(cfg CodexOAuthConfig) int32 {
	cfg = cfg.withDefaults()
	if cfg.PhoneMinReuseRemainingSeconds > 0 {
		return cfg.PhoneMinReuseRemainingSeconds
	}
	value := cfg.PhoneFirstWaitSeconds + cfg.PhoneResendWaitSeconds + 60
	if value < defaultCodexOAuthPhoneMinReuseRemaining {
		return defaultCodexOAuthPhoneMinReuseRemaining
	}
	return value
}

func codexOAuthPhonePageFailureState(data map[string]any) string {
	text := strings.ToLower(fmt.Sprint(data))
	switch {
	case containsAny(text, "already linked to the maximum number of accounts", "used too many", "too many times", "maximum number", "max number", "limit exceeded", "too many attempts"):
		return "phone_reuse_exceeded"
	case containsAny(text, "try a different phone", "try another phone", "can't use this phone", "cannot use this phone", "unsupported phone", "invalid phone", "not valid", "rejected"):
		return "phone_rejected"
	case containsAny(text, "temporarily unavailable", "rate limit", "blocked"):
		return "phone_rejected"
	default:
		return ""
	}
}

func codexOAuthPhoneFailureDisposition(message string) (string, string) {
	text := strings.ToLower(message)
	switch {
	case containsAny(text, "phone_reuse_exceeded", "already linked to the maximum number of accounts", "used too many", "too many times", "maximum number", "max number", "limit exceeded"):
		return "phone_reuse_exceeded", codexOAuthLeaseExhausted
	case containsAny(text, "phone_rejected", "try a different phone", "try another phone", "can't use this phone", "cannot use this phone", "unsupported phone", "invalid phone", "rejected"):
		return "phone_rejected", codexOAuthLeaseFailed
	case containsAny(text, "phone_sms_timeout", "sms_error_code_timeout", "waitforcode", "otp not found", "empty code"):
		return "phone_sms_timeout", codexOAuthLeaseFailed
	case containsAny(text, "phone_expired"):
		return "phone_expired", codexOAuthLeaseExpired
	default:
		return "", ""
	}
}

func codexOAuthFailureLikelyUsedPhone(message string) bool {
	failureKind, _ := codexOAuthPhoneFailureDisposition(message)
	switch failureKind {
	case "phone_reuse_exceeded", "phone_rejected", "phone_sms_timeout":
		return true
	default:
		return false
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func codexOAuthFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func codexOAuthPhoneCountrySelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`select[name="country"],select[autocomplete="tel-country-code"],select[aria-label*="Country" i],select`),
	)
}

func codexOAuthPhoneInputSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[type="tel"][name="__reservedForPhoneNumberInput_tel"],input#tel[autocomplete="tel"],input[type="tel"]`)
}

func codexOAuthPhoneOTPSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="code"][autocomplete="one-time-code"][placeholder="Code"],input[autocomplete="one-time-code"],input[inputmode="numeric"]`)
}

func codexOAuthConsentSignalSelector() *browserautomationv1.BrowserSelector {
	return textSelector("Codex CLI", false)
}

func codexOAuthStageSelectorGroup(timeout time.Duration) *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(timeout,
		browserAuthLoginOTPSelector(),
		browserAuthLoginPasswordSelector(),
		codexOAuthPhoneInputSelector(),
		codexOAuthConsentSignalSelector(),
	)
}

func waitForURLCommand(commandID, pattern string, exact bool, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForUrl{
			WaitForUrl: &browserautomationv1.WaitForURLCommand{
				UrlPattern: pattern,
				Exact:      exact,
				Timeout:    durationpb.New(timeout),
			},
		},
	}
}

func newCodexOAuthPKCE() (codexOAuthPKCE, error) {
	verifier, err := randomURLToken(64)
	if err != nil {
		return codexOAuthPKCE{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	return codexOAuthPKCE{
		verifier:  verifier,
		challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func buildCodexOAuthAuthorizeURL(cfg CodexOAuthConfig, pkce codexOAuthPKCE, state string) string {
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", cfg.ClientID)
	values.Set("redirect_uri", cfg.RedirectURI)
	values.Set("scope", cfg.Scope)
	values.Set("code_challenge", pkce.challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("id_token_add_organizations", "true")
	values.Set("codex_cli_simplified_flow", "true")
	values.Set("state", state)
	values.Set("originator", "codex_cli_rs")
	return strings.TrimRight(cfg.AuthURL, "?") + "?" + values.Encode()
}

func codexOAuthCodeFromCallback(rawURL string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	values := parsed.Query()
	if errText := strings.TrimSpace(values.Get("error")); errText != "" {
		return "", "", fmt.Errorf("codex oauth callback error: %s", errText)
	}
	code := strings.TrimSpace(values.Get("code"))
	state := strings.TrimSpace(values.Get("state"))
	if code == "" {
		return "", "", fmt.Errorf("codex oauth callback code missing")
	}
	return code, state, nil
}

func exchangeCodexOAuthToken(ctx context.Context, cfg CodexOAuthConfig, code, verifier string) (codexOAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", cfg.RedirectURI)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return codexOAuthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return codexOAuthTokenResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return codexOAuthTokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token exchange failed: status %d %s", resp.StatusCode, compactBrowserAuthText(string(body), 500))
	}
	var tokens codexOAuthTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return codexOAuthTokenResponse{}, err
	}
	if tokens.IDToken == "" || tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return codexOAuthTokenResponse{}, fmt.Errorf("codex oauth token response missing required tokens")
	}
	return tokens, nil
}

func buildCodexAuthJSON(tokens codexOAuthTokenResponse) ([]byte, error) {
	_, accountID := codexOAuthAuthClaims(tokens.IDToken)
	tokenPayload := map[string]any{
		"id_token":      tokens.IDToken,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}
	if accountID != "" {
		tokenPayload["account_id"] = accountID
	}
	auth := map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens":         tokenPayload,
		"last_refresh":   time.Now().UTC().Format(time.RFC3339),
	}
	return json.MarshalIndent(auth, "", "  ")
}

func codexOAuthAuthClaims(idToken string) (string, string) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if padded, padErr := base64.URLEncoding.DecodeString(parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)); padErr == nil {
			payload = padded
		} else {
			return "", ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ""
	}
	email := claimString(claims["email"])
	accountID := ""
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		accountID = claimString(auth["chatgpt_account_id"])
	}
	if email == "" {
		if profile, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
			email = claimString(profile["email"])
		}
	}
	return email, accountID
}

func claimString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func maskPhone(e164, national string) string {
	value := strings.TrimSpace(e164)
	if value == "" {
		value = strings.TrimSpace(national)
	}
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
}
