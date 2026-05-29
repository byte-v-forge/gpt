package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	goPayPaymentPinSecretKey          = "gopay_payment_pin:"
	goPayPaymentAddBalanceParam       = "gopay_add_balance_json"
	goPayPaymentDefaultWaitSeconds    = int32(1800)
	goPayAppSignupMaxPhoneAttemptsAPI = 3
)

func (s *Server) probeGoPayPaymentEligibility(ctx context.Context, jobID string, accountID string, data map[string]any) (pb.ProbePlusTrialActivityOutput, error) {
	probe, err := s.activities.ProbePlusTrialAtomicActivity(ctx, pb.ProbePlusTrialActivityInput{JobId: jobID, AccountId: accountID})
	mergeActionData(data, "probe_plus_trial", structMap(probe.GetData()))
	if err != nil {
		return probe, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, err, data)
	}
	if !probe.GetChecked() {
		return probe, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedRetryable, false, true, fmt.Errorf("plus trial eligibility is unknown"), data)
	}
	if !probe.GetPlusTrialEligible() {
		return probe, s.markActionFailed(ctx, jobID, stepProbePlusTrial, jobstatus.FailedFinal, false, false, fmt.Errorf("account is not plus trial eligible"), data)
	}
	return probe, nil
}

func (s *Server) checkGoPayAddBalanceReadyAction(ctx context.Context, jobID string, stateJSON string) (string, map[string]any, bool, string) {
	status, err := s.activities.GoPayAppBalanceCheckActivity(ctx, pb.GoPayAppStepInput{JobId: jobID, StateJson: stateJSON})
	data := structMap(status.GetData())
	if err != nil {
		return firstNonEmpty(status.GetStateJson(), stateJSON), map[string]any{"error_message": err.Error()}, false, err.Error()
	}
	ready, amount, currency := goPayAddBalanceBalanceReadyAPI(data)
	if ready {
		data["balance_ready"] = true
		data["balance_amount"] = amount
		if currency != "" {
			data["balance_currency"] = currency
		}
		return firstNonEmpty(status.GetStateJson(), stateJSON), data, true, ""
	}
	if message := stringMapValue(data, "error_message"); message != "" {
		return firstNonEmpty(status.GetStateJson(), stateJSON), data, false, message
	}
	if amount != 0 || currency != "" {
		return firstNonEmpty(status.GetStateJson(), stateJSON), data, false, fmt.Sprintf("balance %d %s < 1 IDR", amount, currency)
	}
	return firstNonEmpty(status.GetStateJson(), stateJSON), data, false, ""
}

func (s *Server) peekGoPayAddBalanceSelection(ctx context.Context, jobID string) (*pb.GoPayAddBalance, error) {
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if selected, err := goPayPaymentParamAddBalance(params); err != nil || goPayAddBalanceMethod(selected) != "" {
		return selected, err
	}
	raw, found, err := s.jobStore.GetParam(ctx, jobID, goPayAddBalanceSelectionParam)
	if err != nil || !found {
		return nil, err
	}
	return decodeGoPayAddBalance(raw)
}

func (s *Server) selectedGoPayAddBalanceParam(ctx context.Context, jobID string) (*pb.GoPayAddBalance, error) {
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if selected, err := goPayPaymentParamAddBalance(params); err != nil || goPayAddBalanceMethod(selected) != "" {
		return selected, err
	}
	raw, found, err := s.jobStore.GetParam(ctx, jobID, goPayAddBalanceSelectionParam)
	if err != nil || !found {
		return nil, err
	}
	addBalance, err := decodeGoPayAddBalance(raw)
	if err != nil {
		return nil, err
	}
	_ = s.jobStore.DeleteParam(ctx, jobID, goPayAddBalanceSelectionParam)
	return addBalance, nil
}

func encodeGoPayAddBalance(value *pb.GoPayAddBalance) (string, error) {
	if value == nil {
		return "", nil
	}
	out, err := protojson.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func decodeGoPayAddBalance(raw string) (*pb.GoPayAddBalance, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := &pb.GoPayAddBalance{}
	if err := protojson.Unmarshal([]byte(raw), out); err != nil {
		return nil, err
	}
	return out, nil
}

func goPayPaymentParamAddBalance(params map[string]string) (*pb.GoPayAddBalance, error) {
	return decodeGoPayAddBalance(params[goPayPaymentAddBalanceParam])
}

func paymentTimeoutSeconds(params map[string]string) int32 {
	value, _ := strconv.ParseInt(strings.TrimSpace(params["add_balance_confirm_timeout_seconds"]), 10, 32)
	if value <= 0 {
		return goPayPaymentDefaultWaitSeconds
	}
	return int32(value)
}

func goPayAddBalanceMethodOptionsAPI() []any {
	return []any{"manual_transfer", "envelope"}
}

func goPayAddBalanceBalanceReadyAPI(data map[string]any) (bool, int64, string) {
	source := data
	if nested, ok := data["status"].(map[string]any); ok && len(nested) > 0 {
		source = nested
	}
	amount := int64MapValueAPI(source, "balance_amount")
	currency := stringMapValue(source, "balance_currency")
	return boolMapValueAPI(source, "has_min_balance") || amount >= 1 || boolMapValueAPI(source, "balance_ready"), amount, currency
}

func boolMapValueAPI(data map[string]any, key string) bool {
	switch value := data[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

func int64MapValueAPI(data map[string]any, key string) int64 {
	switch value := data[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	default:
		return 0
	}
}

func goPayAppDeviceProxyMatchedAPI(expected pb.GoPayAppGenerateDeviceProxyOutput, actual pb.GoPayAppCheckSignupPhoneOutput) bool {
	expectedProxy := strings.TrimSpace(expected.GetProxyHash())
	actualProxy := strings.TrimSpace(actual.GetProxyHash())
	expectedDevice := strings.TrimSpace(expected.GetDeviceFingerprint())
	actualDevice := strings.TrimSpace(actual.GetDeviceFingerprint())
	return expectedProxy != "" && actualProxy != "" && expectedDevice != "" && actualDevice != "" && expectedProxy == actualProxy && expectedDevice == actualDevice
}

func isGoPaySignupPhoneRotatableErrorAPI(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "signup phone already registered") ||
		strings.Contains(message, "signup phone unavailable") ||
		strings.Contains(message, "status 429") ||
		strings.Contains(message, "ratelimit") ||
		strings.Contains(message, "rate_limited") ||
		strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "client.timeout") ||
		strings.Contains(message, "awaiting headers") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "eof") ||
		strings.Contains(message, "connection reset")
}

func isGoPaySignupOTPNotReceivedAPI(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "gopay signup otp not received") ||
		strings.Contains(message, "otp not received") ||
		strings.Contains(message, "otp not found") ||
		strings.Contains(message, "waitcode")
}
