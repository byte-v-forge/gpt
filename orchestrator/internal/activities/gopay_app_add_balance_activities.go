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
	resp, nextStateJSON, err := s.validateGoPayAccountTokenForState(ctx, output.GetStateJson())
	output.StateJson = nextStateJSON
	data := checkTokenValidData(resp)
	if err != nil {
		data["error_message"] = err.Error()
	} else if resp != nil {
		output.Stage = resp.GetStage()
		output.Phone = resp.GetPhone()
		output.Ready = resp.GetSuccess() && resp.GetTokenValid()
		output.AccountTokenReady = output.GetReady()
		data["balance_ready"] = resp.GetHasMinBalance() || resp.GetBalanceAmount() >= 1
	}
	output.Data = protoData(data)
	return output, nil
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
