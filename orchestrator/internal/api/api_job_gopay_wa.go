package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	goPayWAPaymentPinSecretKey         = "gopay_wa_payment_pin:"
	goPayWAPaymentAccessTokenSecretKey = "gopay_wa_payment_access_token:"
)

func (s *Server) runGoPayWAPaymentAction(ctx context.Context, jobID string, params map[string]string) error {
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["pin_secret_key"])

	accessToken, err := s.runtimeSecretValue(ctx, params["access_token_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["access_token_secret_key"])

	accountID := strings.TrimSpace(params["account_id"])
	sourceJobID := strings.TrimSpace(params["source_job_id"])
	userID := goPayAppUserID(params["user_id"])
	combined := map[string]any{
		"action":               actionGoPayWAPayment,
		"otp_channel":          "wa",
		"user_id":              userID,
		"payment_only":         true,
		"uses_account_token":   false,
		"uses_gopay_app_flow":  false,
		"access_token_present": strings.TrimSpace(accessToken) != "",
	}
	if accountID == "" && sourceJobID == "" && strings.TrimSpace(accessToken) == "" {
		err := fmt.Errorf("account_id, source_job_id, or access_token is required")
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedFinal, false, false, err, combined)
	}

	if accountID != "" || sourceJobID != "" {
		account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: accountID, SourceJobId: sourceJobID})
		if err != nil {
			return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
		}
		accountID = account.GetAccountId()
		if err := s.jobStore.SetAccountID(ctx, jobID, accountID); err != nil {
			return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
		}
	}
	combined["account_id"] = accountID

	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID, AccessToken: accessToken})
	combined["probe_plus_trial"] = structMap(probe.GetData())
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !probe.GetChecked() {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), combined)
	}
	if !probe.GetPlusTrialEligible() {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), combined)
	}

	waPhone, err := s.activities.GoPayResolveWAPhoneActivity(ctx, pb.GoPayResolveWAPhoneInput{JobId: jobID, UserId: userID, WaPhone: params["wa_phone"]})
	mergeActionData(combined, "wa_phone_resolution", structMap(waPhone.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayResolveWAPhone, jobstatus.FailedRetryable, false, true, err, combined)
	}
	userID = goPayAppUserID(waPhone.GetUserId())
	combined["user_id"] = userID
	combined["wa_phone_present"] = strings.TrimSpace(waPhone.GetWaPhone()) != ""

	stateJSON := "{}"
	prepare, err := s.prepareGoPayPaymentAction(ctx, pb.GoPayActivityInput{
		JobId:        jobID,
		AccountId:    accountID,
		AccessToken:  accessToken,
		Tokenization: "true",
		GopayPhone:   waPhone.GetWaPhone(),
		UserId:       userID,
		StateJson:    stateJSON,
		CountryCode:  params["country_code"],
	})
	stateJSON = firstNonEmpty(prepare.GetStateJson(), stateJSON)
	combined["gopay_payment_prepare"] = structMap(prepare.GetData())
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPaymentPrepare, jobstatus.FailedRetryable, false, true, err, combined)
	}

	payment, err := s.runGoPayPaymentActionStep(ctx, pb.GoPayActivityInput{
		JobId:          jobID,
		AccountId:      accountID,
		AccessToken:    accessToken,
		Tokenization:   "true",
		PreparedFlowId: prepare.GetFlowId(),
		GopayPhone:     waPhone.GetWaPhone(),
		OtpChannel:     "wa",
		UserId:         userID,
		StateJson:      stateJSON,
		Pin:            pin,
		CountryCode:    params["country_code"],
	})
	combined["gopay_payment"] = structMap(payment.GetData())
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["payment_completed"] = payment.GetChargeRef() != "" && payment.GetPlusActive()
	combined["payment_async_pending"] = false
	combined["charge_ref"] = payment.GetChargeRef()
	combined["snap_token_present"] = payment.GetSnapToken() != ""
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}
