package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const activationGoPayPinSecretKey = "activation_gopay_pin:"

func (s *Server) runActivateAccountAction(ctx context.Context, jobID string, params map[string]string) error {
	return s.runActivationPaymentAction(ctx, jobID, actionActivate, params)
}

func (s *Server) runAutopayAccountAction(ctx context.Context, jobID string, params map[string]string) error {
	return s.runActivationPaymentAction(ctx, jobID, actionAutopay, params)
}

func (s *Server) runActivationPaymentAction(ctx context.Context, jobID string, action string, params map[string]string) error {
	pin, err := s.runtimeSecretValue(ctx, params["gopay_pin_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["gopay_pin_secret_key"])

	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{
		AccountId:   params["account_id"],
		SourceJobId: params["source_job_id"],
	})
	combined := map[string]any{
		"action":        action,
		"account_id":    account.GetAccountId(),
		"source_job_id": params["source_job_id"],
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}
	if err := s.jobStore.SetAccountID(ctx, jobID, account.GetAccountId()); err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}

	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: account.GetAccountId()})
	probeData := structMap(probe.GetData())
	combined["probe_plus_trial"] = probeData
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !probe.GetChecked() {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), combined)
	}
	if action == actionAutopay && !probe.GetPlusTrialEligible() {
		return s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), combined)
	}
	if action == actionActivate && !probe.GetPlusTrialEligible() && !probe.GetPlusActive() {
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

	paymentInput := pb.GoPayActivityInput{
		JobId:             jobID,
		AccountId:         account.GetAccountId(),
		UseAccountToken:   login.GetAccountTokenReady(),
		CheckoutUrl:       probe.GetCheckoutUrl(),
		CheckoutSessionId: probe.GetCheckoutSessionId(),
		StateJson:         login.GetStateJson(),
		Pin:               pin,
		CountryCode:       params["gopay_country_code"],
	}
	if action == actionAutopay {
		paymentInput.Tokenization = "true"
	}
	payment, err := s.runGoPayPaymentActionStep(ctx, paymentInput)
	mergeActionData(combined, "gopay_payment", structMap(payment.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, combined)
	}

	if action == actionActivate {
		if err := s.activities.PersistActivatedActivity(ctx, pb.PersistActivatedInput{
			AccountId:         account.GetAccountId(),
			ChargeRef:         payment.GetChargeRef(),
			PlusTrialEligible: payment.GetPlusTrialEligible(),
			PlusTrialChecked:  payment.GetPlusTrialChecked(),
			PlusActive:        payment.GetPlusActive(),
		}); err != nil {
			return s.markActionFailed(ctx, jobID, "persist_activated", jobstatus.FailedRecoverable, true, false, err, combined)
		}
	} else {
		tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: account.GetAccountId()})
		mergeActionData(combined, "probe_tier", structMap(tier.GetData()))
		if err != nil {
			return s.markActionFailed(ctx, jobID, stepProbeTier, jobstatus.FailedRecoverable, true, false, err, combined)
		}
	}

	combined["charge_ref"] = payment.GetChargeRef()
	combined["snap_token_present"] = payment.GetSnapToken() != ""
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) runGoPayPaymentActionStep(ctx context.Context, input pb.GoPayActivityInput) (pb.GoPayActivityOutput, error) {
	start, err := s.activities.GoPayPaymentStartActivity(ctx, input)
	if err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(start.GetFlowId(), input.GetPreparedFlowId()))
		return pb.GoPayActivityOutput{Data: start.GetData(), StateJson: start.GetStateJson()}, err
	}

	otpSource := "not_required"
	if start.GetOtpRequired() {
		if err := s.requestGoPayPaymentSMSCode(ctx, input, stepGoPayPayment); err != nil {
			s.cancelGoPayPaymentAction(ctx, start.GetFlowId())
			return pb.GoPayActivityOutput{Data: start.GetData(), StateJson: start.GetStateJson()}, err
		}
		otp, err := s.waitForGoPayPaymentOTP(ctx, input, start.GetOtpTimeoutSeconds(), start.GetIssuedAfterUnix())
		if shouldRetryGoPayPaymentOTP(input, err) {
			start, err = s.resendGoPayPaymentOTP(ctx, start, input)
			if err == nil {
				err = s.requestGoPayPaymentSMSCode(ctx, input, stepGoPayPayment+"_retry")
			}
			if err == nil {
				otp, err = s.waitForGoPayPaymentOTP(ctx, input, start.GetOtpTimeoutSeconds(), start.GetIssuedAfterUnix())
			}
		}
		if err != nil {
			s.cancelGoPayPaymentAction(ctx, start.GetFlowId())
			return pb.GoPayActivityOutput{Data: start.GetData(), StateJson: start.GetStateJson()}, err
		}
		otpSource = otp.GetSource()
	}

	waitForManual := strings.EqualFold(strings.TrimSpace(input.GetTokenization()), "qris")
	payment, err := s.activities.GoPayPaymentCompleteActivity(ctx, pb.GoPayPaymentCompleteInput{
		JobId:                     input.GetJobId(),
		AccountId:                 input.GetAccountId(),
		FlowId:                    start.GetFlowId(),
		OtpParam:                  paymentOTPParam,
		SubmittedAtParam:          paymentOTPSubmittedAtParam,
		OtpIssuedAfterUnix:        start.GetIssuedAfterUnix(),
		OtpSource:                 otpSource,
		UseAccountToken:           start.GetUseAccountToken(),
		Data:                      start.GetData(),
		StateJson:                 start.GetStateJson(),
		Pin:                       input.GetPin(),
		WaitForManualConfirmation: waitForManual,
	})
	if err != nil {
		return payment, err
	}
	if !payment.GetAwaitingManualConfirmation() {
		return payment, nil
	}
	if err := s.waitForManualGoPayPaymentAction(ctx, input.GetJobId(), 1800); err != nil {
		s.cancelGoPayPaymentAction(ctx, paymentFlowID(payment.GetFlowId(), start.GetFlowId()))
		return payment, err
	}
	confirmed, err := s.activities.GoPayPaymentManualConfirmActivity(ctx, pb.GoPayPaymentManualConfirmInput{
		JobId:     input.GetJobId(),
		AccountId: input.GetAccountId(),
		FlowId:    paymentFlowID(payment.GetFlowId(), start.GetFlowId()),
		Data:      payment.GetData(),
		StateJson: payment.GetStateJson(),
	})
	if err != nil {
		return confirmed, err
	}
	return confirmed, nil
}

func (s *Server) resendGoPayPaymentOTP(ctx context.Context, start pb.GoPayPaymentStartOutput, input pb.GoPayActivityInput) (pb.GoPayPaymentStartOutput, error) {
	resend, err := s.activities.GoPayPaymentOTPResendActivity(ctx, pb.GoPayPaymentOTPResendInput{
		JobId:     input.GetJobId(),
		AccountId: input.GetAccountId(),
		FlowId:    start.GetFlowId(),
		Data:      start.GetData(),
	})
	if resend.GetData() != nil {
		start.Data = resend.GetData()
	}
	if resend.GetIssuedAfterUnix() > 0 {
		start.IssuedAfterUnix = resend.GetIssuedAfterUnix()
	}
	return start, err
}

func (s *Server) requestGoPayPaymentSMSCode(ctx context.Context, input pb.GoPayActivityInput, reason string) error {
	if normalizeGoPayOTPChannel(input.GetOtpChannel()) != "sms" || strings.TrimSpace(input.GetSmsActivationId()) == "" {
		return nil
	}
	_, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{
		JobId:        input.GetJobId(),
		ActivationId: input.GetSmsActivationId(),
		Reason:       reason,
	})
	return err
}

func (s *Server) waitForGoPayPaymentOTP(ctx context.Context, input pb.GoPayActivityInput, timeoutSeconds int32, issuedAfterUnix int64) (pb.OTPWaitOutput, error) {
	waitInput := pb.OTPWaitInput{
		JobId:            input.GetJobId(),
		StepName:         stepGoPayPayment,
		TimeoutSeconds:   timeoutSeconds,
		IssuedAfterUnix:  issuedAfterUnix,
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
	if normalizeGoPayOTPChannel(input.GetOtpChannel()) == "sms" && strings.TrimSpace(input.GetSmsActivationId()) != "" {
		waitInput.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: input.GetSmsActivationId()}}
	} else {
		source := strings.TrimSpace(input.GetUserId())
		if source == "" {
			source = goPayLocalSource
		}
		waitInput.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: source}}
	}
	otp, err := s.waitGoPayOTP(ctx, waitInput)
	if err != nil {
		return otp, err
	}
	if !otp.GetFound() {
		return otp, goPayPaymentOTPNotReceivedError(timeoutSeconds, otp)
	}
	return otp, nil
}

func shouldRetryGoPayPaymentOTP(input pb.GoPayActivityInput, err error) bool {
	return normalizeGoPayOTPChannel(input.GetOtpChannel()) == "sms" && isOTPWaitNotReceivedError(err)
}

func goPayPaymentOTPNotReceivedError(timeoutSeconds int32, wait pb.OTPWaitOutput) error {
	reason := strings.TrimSpace(wait.GetErrorMessage())
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("payment otp not received after %ds: %s", timeoutSeconds, reason)
}

func (s *Server) cancelGoPayPaymentAction(ctx context.Context, flowID string) {
	if strings.TrimSpace(flowID) == "" {
		return
	}
	_ = s.activities.GoPayPaymentCancelActivity(ctx, pb.GoPayPaymentCancelInput{FlowId: flowID})
}

func paymentFlowID(values ...string) string {
	return firstNonEmpty(values...)
}
