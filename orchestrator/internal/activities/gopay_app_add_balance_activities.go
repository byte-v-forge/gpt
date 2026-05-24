package activities

import (
	"context"
	"fmt"
	"strings"
)

const (
	goPayEnvelopeLinkSecretKey       = "gopay_add_balance_envelope_link"
	rekberinajaAccessTokenSecretKey  = "gopay_add_balance_rekberinaja_access_token"
	rekberinajaRefreshTokenSecretKey = "gopay_add_balance_rekberinaja_refresh_token"
)

func (s *Server) GoPayAppAddBalanceActivity(ctx context.Context, input GoPayAppAddBalanceInput) (GoPayAppAddBalanceOutput, error) {
	output := GoPayAppAddBalanceOutput{StateJson: normalizeGoPayWorkflowStateJSON(input.GetStateJson())}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppEnsureBalance, false, true)
	_, err := step.run(func() (any, error) {
		return s.runGoPayAddBalance(ctx, step, input, &output, data)
	})
	output.Data = protoData(data)
	return output, err
}

func (s *Server) GoPayAppBalanceCheckActivity(ctx context.Context, input GoPayAppStepInput) (GoPayAppStepOutput, error) {
	output := GoPayAppStepOutput{StateJson: normalizeGoPayWorkflowStateJSON(input.GetStateJson())}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppEnsureBalance, false, true)
	resp, nextStateJSON, err := s.validateGoPayAccountTokenForState(ctx, output.GetStateJson())
	output.StateJson = nextStateJSON
	data := checkTokenValidData(resp)
	s.syncGoPayBalanceCheckState(ctx, output.GetStateJson(), data)
	if err != nil {
		data["error_message"] = err.Error()
	} else if resp != nil {
		output.Stage = resp.GetStage()
		output.Phone = resp.GetPhone()
		output.Ready = resp.GetSuccess() && resp.GetTokenValid()
		output.AccountTokenReady = output.GetReady()
		data["balance_ready"] = resp.GetHasMinBalance() || resp.GetBalanceAmount() >= 1
	}
	if !goPayBalanceCheckDataReady(data) && (err != nil || resp == nil || !resp.GetSuccess()) {
		if cached, ok := goPayCachedBalanceReadyData(output.GetStateJson(), "workflow_state_cache"); ok {
			data = mergeGoPayBalanceCheckData(data, cached)
			applyGoPayBalanceCheckReadyOutput(&output, data)
		} else if storedStateJSON, loadErr := s.loadGoPayAppState(ctx); loadErr == nil {
			if cached, ok := goPayCachedBalanceReadyData(storedStateJSON, "stored_state_cache"); ok {
				output.StateJson = normalizeGoPayWorkflowStateJSON(storedStateJSON)
				data = mergeGoPayBalanceCheckData(data, cached)
				applyGoPayBalanceCheckReadyOutput(&output, data)
			}
		} else {
			data["state_cache_error"] = loadErr.Error()
		}
	}
	if input.GetJobId() != "" {
		if goPayBalanceCheckDataReady(data) {
			step.progress("gopay balance ready", map[string]any{
				"balance_amount":   data["balance_amount"],
				"balance_currency": data["balance_currency"],
				"source":           data["source"],
				"status":           "balance_ready",
			})
		} else {
			step.progress("checking gopay balance", map[string]any{
				"balance_amount":   data["balance_amount"],
				"balance_currency": data["balance_currency"],
				"error_message":    data["error_message"],
				"methods":          goPayAddBalanceSelectionMethods(),
				"status":           "awaiting_selection",
			})
		}
	}
	output.Data = protoData(data)
	return output, nil
}

func (s *Server) syncGoPayBalanceCheckState(ctx context.Context, stateJSON string, data map[string]any) {
	if !goPayWorkflowStatePresent(stateJSON) {
		return
	}
	if goPayWorkflowStateHasAnyKey(stateJSON, "_signup_sms_activation_id", "signup_sms_activation_id") {
		data["state_sync_skipped"] = "sms_activation_state"
		return
	}
	storedStateJSON, err := s.loadGoPayAppState(ctx)
	if err != nil {
		data["state_sync_error"] = err.Error()
		return
	}
	if !goPaySameStoredAccountState(storedStateJSON, stateJSON) {
		data["state_sync_skipped"] = "account_state_mismatch"
		return
	}
	if err := s.saveGoPayAppState(ctx, stateJSON); err != nil {
		data["state_sync_error"] = err.Error()
		return
	}
	data["state_synced"] = true
}

func goPayAddBalanceSelectionMethods() []string {
	return []string{"manual_transfer", "envelope", "rekberinaja"}
}

func goPayBalanceCheckDataReady(data map[string]any) bool {
	if boolMapValue(data, "has_min_balance") || int64MapValue(data, "balance_amount") >= 1 {
		return true
	}
	return boolMapValue(data, "balance_ready")
}

func mergeGoPayBalanceCheckData(current, cached map[string]any) map[string]any {
	if len(current) == 0 {
		return cached
	}
	merged := make(map[string]any, len(current)+len(cached)+1)
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range cached {
		merged[key] = value
	}
	merged["live_check"] = current
	return merged
}

func applyGoPayBalanceCheckReadyOutput(output *GoPayAppStepOutput, data map[string]any) {
	if output == nil {
		return
	}
	output.Stage = stringMapValue(data, "stage")
	output.Phone = stringMapValue(data, "phone")
	output.Ready = true
	output.AccountTokenReady = true
}

func (s *Server) applyGoPayAddBalanceReadyFromToken(ctx context.Context, output *GoPayAppAddBalanceOutput, data map[string]any, reason string) bool {
	if s.gopayClient == nil {
		data["balance_check_error"] = "gopay-app client not configured"
		return false
	}
	resp, nextStateJSON, err := s.validateGoPayAccountTokenForState(ctx, output.GetStateJson())
	output.StateJson = nextStateJSON
	checkData := checkTokenValidData(resp)
	if reason = strings.TrimSpace(reason); reason != "" {
		checkData["reason"] = reason
	}
	if err != nil {
		checkData["error_message"] = err.Error()
		data["balance_check_error"] = err.Error()
		data["balance_check"] = checkData
		return false
	}
	data["balance_check"] = checkData
	if resp == nil || !resp.GetSuccess() || !resp.GetTokenValid() {
		return false
	}
	ready := resp.GetHasMinBalance() || resp.GetBalanceAmount() >= 1
	if !ready {
		return false
	}
	output.Success = true
	output.Method = "balance_poll"
	output.Status = "balance_ready"
	data["method"] = output.GetMethod()
	data["status"] = output.GetStatus()
	data["add_balance_complete"] = true
	data["balance_ready"] = true
	data["has_min_balance"] = resp.GetHasMinBalance()
	data["balance_amount"] = resp.GetBalanceAmount()
	if currency := strings.TrimSpace(resp.GetBalanceCurrency()); currency != "" {
		data["balance_currency"] = currency
	}
	return true
}

func (s *Server) runGoPayAddBalance(ctx context.Context, step activityStep, input GoPayAppAddBalanceInput, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	if s.applyGoPayAddBalanceReadyFromToken(ctx, output, data, "before_add_balance_method") {
		step.progress("gopay balance already ready", map[string]any{
			"balance_amount": data["balance_amount"],
			"currency":       data["balance_currency"],
		})
		return data, nil
	}
	addBalance := input.GetAddBalance()
	if addBalance == nil {
		err := fmt.Errorf("add_balance is required")
		data["error_message"] = err.Error()
		return data, err
	}
	switch {
	case addBalance.GetManualTransfer() != nil:
		return s.prepareManualTransferAddBalance(ctx, step, addBalance.GetManualTransfer(), output, data)
	case addBalance.GetEnvelope() != nil:
		return s.claimEnvelopeAddBalance(ctx, step, addBalance.GetEnvelope(), output, data)
	case addBalance.GetRekberinaja() != nil:
		return s.submitRekberinajaAddBalance(ctx, step, addBalance.GetRekberinaja(), input.GetTargetPhone(), output, data)
	default:
		err := fmt.Errorf("add_balance method is required")
		data["error_message"] = err.Error()
		return data, err
	}
}
