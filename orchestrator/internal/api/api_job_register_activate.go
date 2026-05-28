package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) runRegisterAndActivateAction(ctx context.Context, jobID string, params map[string]string) error {
	pin, err := s.runtimeSecretValue(ctx, params["gopay_pin_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["gopay_pin_secret_key"])

	registered, err := s.runBrowserRegisterAccountAction(ctx, jobID, params["account_id"], params)
	if err != nil {
		return err
	}
	accountID := registered.AccountID
	register := registered.Register
	combined := map[string]any{
		"action":           actionRegisterAndActivate,
		"account_id":       accountID,
		"register_account": registered.Data,
	}

	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{
		JobId:        jobID,
		AccountId:    accountID,
		SessionToken: register.GetSessionToken(),
		AccessToken:  register.GetAccessToken(),
	})
	mergeActionData(combined, "probe_plus_trial", structMap(probe.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !probe.GetChecked() {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), combined)
	}
	if !probe.GetPlusTrialEligible() && !probe.GetPlusActive() {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), combined)
	}

	login, err := s.runGoPayAppAuth(ctx, jobID, goPayAppActionOptions{
		Phone:       params["gopay_phone"],
		CountryCode: params["gopay_country_code"],
		Pin:         pin,
	})
	mergeActionData(combined, "gopay_login", structMap(login.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppLogin, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !login.GetAccountTokenReady() {
		return s.markActionFailed(ctx, jobID, stepGoPayAppLogin, jobstatus.FailedRetryable, false, true, fmt.Errorf("gopay account token is not ready after login"), combined)
	}

	payment, err := s.runGoPayPaymentActionStep(ctx, pb.GoPayActivityInput{
		JobId:             jobID,
		AccountId:         accountID,
		SessionToken:      register.GetSessionToken(),
		AccessToken:       register.GetAccessToken(),
		UseAccountToken:   login.GetAccountTokenReady(),
		CheckoutUrl:       probe.GetCheckoutUrl(),
		CheckoutSessionId: probe.GetCheckoutSessionId(),
		StateJson:         login.GetStateJson(),
		Pin:               pin,
		CountryCode:       params["gopay_country_code"],
	})
	mergeActionData(combined, "gopay_payment", structMap(payment.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, combined)
	}

	if err := s.activities.PersistActivatedActivity(ctx, pb.PersistActivatedInput{
		AccountId:         accountID,
		SessionToken:      register.GetSessionToken(),
		AccessToken:       register.GetAccessToken(),
		ChargeRef:         payment.GetChargeRef(),
		PlusTrialEligible: payment.GetPlusTrialEligible(),
		PlusTrialChecked:  payment.GetPlusTrialChecked(),
		PlusActive:        payment.GetPlusActive(),
	}); err != nil {
		return s.markActionFailed(ctx, jobID, "persist_activated", jobstatus.FailedRecoverable, true, false, err, combined)
	}

	combined["session_token_present"] = strings.TrimSpace(register.GetSessionToken()) != ""
	combined["access_token_present"] = strings.TrimSpace(register.GetAccessToken()) != ""
	combined["activation_success"] = true
	combined["charge_ref"] = payment.GetChargeRef()
	combined["snap_token_present"] = payment.GetSnapToken() != ""
	combined["plus_trial_eligible"] = payment.GetPlusTrialEligible() || probe.GetPlusTrialEligible() || register.GetPlusTrialEligible()
	if err := s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)}); err != nil {
		return err
	}
	_, _ = s.jobStore.CreateWithID(ctx, jobID+"-probe", accountID, actionProbeAccount, map[string]string{"account_id": accountID, "source_job_id": jobID})
	return nil
}
