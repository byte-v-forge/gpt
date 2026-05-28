package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) runGoPayQRISPaymentActivateAction(ctx context.Context, jobID string, params map[string]string) error {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: params["account_id"], SourceJobId: params["source_job_id"]})
	combined := map[string]any{
		"action":                actionGoPayQRISPaymentActivate,
		"activation_mode":       "qris_payment",
		"payment_type":          "qris",
		"tokenization":          "qris",
		"otp_channel":           "not_required",
		"uses_wa":               false,
		"uses_gopay_app_flow":   false,
		"uses_gopay_app_token":  false,
		"manual_confirmation":   true,
		"manual_payment_button": true,
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["account_id"] = account.GetAccountId()
	if err := s.jobStore.SetAccountID(ctx, jobID, account.GetAccountId()); err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, combined)
	}

	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: account.GetAccountId()})
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

	stateJSON := "{}"
	prepare, err := s.prepareGoPayPaymentAction(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: account.GetAccountId(), Tokenization: "qris", StateJson: stateJSON})
	stateJSON = firstNonEmpty(prepare.GetStateJson(), stateJSON)
	combined["gopay_payment_prepare"] = structMap(prepare.GetData())
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPaymentPrepare, jobstatus.FailedRetryable, false, true, err, combined)
	}

	payment, err := s.runGoPayPaymentActionStep(ctx, pb.GoPayActivityInput{JobId: jobID, AccountId: account.GetAccountId(), Tokenization: "qris", PreparedFlowId: prepare.GetFlowId(), StateJson: stateJSON})
	stateJSON = firstNonEmpty(payment.GetStateJson(), stateJSON)
	combined["gopay_payment"] = structMap(payment.GetData())
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayPayment, jobstatus.FailedRetryable, false, true, err, combined)
	}
	combined["payment_completed"] = true
	combined["charge_ref"] = payment.GetChargeRef()
	combined["snap_token_present"] = payment.GetSnapToken() != ""
	combined["state_json_present"] = strings.TrimSpace(stateJSON) != ""

	tier, err := s.activities.ProbeTierAtomicActivity(ctx, pb.ProbeTierActivityInput{JobId: jobID, AccountId: account.GetAccountId()})
	mergeActionData(combined, "probe_tier", structMap(tier.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepProbeTier, jobstatus.FailedRecoverable, true, false, err, combined)
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) prepareGoPayPaymentAction(ctx context.Context, input pb.GoPayActivityInput) (pb.GoPayPaymentPrepareOutput, error) {
	checkout, err := s.activities.GoPayPaymentPrepareCheckoutActivity(ctx, input)
	if err != nil {
		return checkout, err
	}
	input.PreparedFlowId = checkout.GetFlowId()
	input.CheckoutUrl = checkout.GetCheckoutUrl()
	input.CheckoutSessionId = checkout.GetCheckoutSessionId()
	input.StateJson = checkout.GetStateJson()

	link, err := s.activities.GoPayPaymentPrepareLinkActivity(ctx, input)
	merged := mergeGoPayPaymentPrepareActionOutput(checkout, link)
	if err != nil {
		return merged, err
	}
	if link.GetRetryableFreshCheckout() {
		message := strings.TrimSpace(stringMapValue(structMap(link.GetData()), "error_message"))
		if message == "" {
			message = "chatgpt approve blocked"
		}
		return merged, fmt.Errorf("payment prepare link blocked: %s", message)
	}
	return merged, nil
}

func mergeGoPayPaymentPrepareActionOutput(checkout pb.GoPayPaymentPrepareOutput, link pb.GoPayPaymentPrepareOutput) pb.GoPayPaymentPrepareOutput {
	out := link
	if out.GetFlowId() == "" {
		out.FlowId = checkout.GetFlowId()
	}
	if out.GetCheckoutUrl() == "" {
		out.CheckoutUrl = checkout.GetCheckoutUrl()
	}
	if out.GetCheckoutSessionId() == "" {
		out.CheckoutSessionId = checkout.GetCheckoutSessionId()
	}
	if out.GetStateJson() == "" {
		out.StateJson = checkout.GetStateJson()
	}
	data := map[string]any{"prepare_checkout": structMap(checkout.GetData()), "prepare_link": structMap(link.GetData())}
	for key, value := range structMap(link.GetData()) {
		data[key] = value
	}
	out.Data = structData(data)
	return out
}

func (s *Server) waitForManualGoPayPaymentAction(ctx context.Context, jobID string, timeoutSeconds int32) error {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1800
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for {
		confirmed, found, err := s.jobStore.GetParam(ctx, jobID, manualGoPayPaymentConfirmParam)
		if err != nil {
			return err
		}
		if found && strings.EqualFold(strings.TrimSpace(confirmed), "true") {
			_ = s.jobStore.DeleteParam(ctx, jobID, manualGoPayPaymentConfirmParam)
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("manual gopay payment not confirmed after %ds", timeoutSeconds)
		}
		wait := time.Second
		if remaining < wait {
			wait = remaining
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func stringMapValue(data map[string]any, key string) string {
	if data[key] == nil {
		return ""
	}
	return fmt.Sprint(data[key])
}
