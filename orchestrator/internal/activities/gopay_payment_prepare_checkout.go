package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) prepareGoPayPaymentCheckout(ctx context.Context, step activityStep, input GoPayActivityInput, account *pb.Account) (output GoPayPaymentPrepareOutput, err error) {
	sessionToken := strings.TrimSpace(input.GetSessionToken())
	if sessionToken == "" {
		sessionToken = s.cachedChatGPTSessionToken(ctx, account.GetAccountId())
	}
	accessToken := strings.TrimSpace(input.GetAccessToken())
	if accessToken == "" {
		accessToken = s.cachedChatGPTAccessToken(ctx, account.GetAccountId())
	}
	tokenization := strings.TrimSpace(input.GetTokenization())
	suppliedCheckoutURL := strings.TrimSpace(input.GetCheckoutUrl())
	suppliedCheckoutSessionID := strings.TrimSpace(input.GetCheckoutSessionId())
	gopayPhone := normalizeIndonesiaPhone(input.GetGopayPhone())
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())

	data := map[string]any{
		"stage":                  "checkout",
		"account_id":             account.GetAccountId(),
		"session_token_present":  sessionToken != "",
		"access_token_present":   accessToken != "",
		"tokenization":           tokenization,
		"checkout_url_present":   suppliedCheckoutURL != "",
		"checkout_session_id":    suppliedCheckoutSessionID,
		"checkout_reuse_blocked": suppliedCheckoutURL != "" || suppliedCheckoutSessionID != "",
		"gopay_phone_present":    gopayPhone != "",
		"payment_prepare_called": false,
		"prepared_flow_present":  false,
	}
	output = GoPayPaymentPrepareOutput{
		UseAccountToken: false,
		StateJson:       stateJSON,
		Stage:           "checkout",
	}
	defer func() {
		output.StateJson = stateJSON
		output.Data = protoData(data)
	}()
	if sessionToken == "" && accessToken == "" {
		return output, fmt.Errorf("session_token or access_token is required")
	}

	step.progress("creating gopay payment checkout", map[string]any{
		"tokenization":              tokenization,
		"supplied_checkout_present": suppliedCheckoutURL != "" || suppliedCheckoutSessionID != "",
		"checkout_reuse_blocked":    suppliedCheckoutURL != "" || suppliedCheckoutSessionID != "",
		"gopay_phone_present":       gopayPhone != "",
	})
	credential, err := s.paymentCredential(ctx, account.GetAccountId(), sessionToken, accessToken)
	if err != nil {
		return output, err
	}
	prepared, err := s.paymentClient.PrepareGoPayCheckout(ctx, &pb.PrepareGoPayCheckoutRequest{
		Credential:        credential,
		Tokenization:      tokenization,
		CheckoutUrl:       "",
		CheckoutSessionId: "",
		GopayPhone:        gopayPhone,
		GopayCountryCode:  input.GetCountryCode(),
	})
	data["payment_prepare_called"] = true
	data["payment_prepare_checkout"] = paymentPrepareData(prepared)
	applyGoPayPaymentPrepareResponse(&output, prepared)
	data["prepared_flow_present"] = output.GetFlowId() != ""
	step.progress("gopay payment checkout created", map[string]any{
		"success":             prepared != nil && prepared.GetSuccess(),
		"flow_id_present":     output.GetFlowId() != "",
		"checkout_session_id": output.GetCheckoutSessionId(),
		"checkout_attempt":    output.GetCheckoutAttempt(),
	})
	if err != nil {
		return output, err
	}
	if prepared == nil {
		return output, fmt.Errorf("payment prepare checkout returned empty response")
	}
	if !prepared.GetSuccess() {
		return output, fmt.Errorf("payment prepare checkout failed: %s", prepared.GetErrorMessage())
	}
	if output.GetFlowId() == "" {
		return output, fmt.Errorf("payment prepare checkout returned empty flow_id")
	}
	return output, nil
}

func (s *Server) refreshGoPayPaymentCheckout(ctx context.Context, step activityStep, input GoPayActivityInput) (output GoPayPaymentPrepareOutput, err error) {
	flowID := strings.TrimSpace(input.GetPreparedFlowId())
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	data := map[string]any{
		"stage":                 "checkout_refresh",
		"prepared_flow_present": flowID != "",
	}
	output = GoPayPaymentPrepareOutput{
		FlowId:    flowID,
		StateJson: stateJSON,
		Stage:     "checkout_refresh",
	}
	defer func() {
		output.StateJson = stateJSON
		output.Data = protoData(data)
	}()
	if flowID == "" {
		return output, fmt.Errorf("prepared_flow_id is required")
	}

	step.progress("refreshing gopay payment checkout", map[string]any{
		"flow_id_present": true,
	})
	prepared, err := s.paymentClient.RefreshPrepareGoPayCheckout(ctx, &pb.RefreshPrepareGoPayCheckoutRequest{FlowId: flowID})
	data["payment_prepare_checkout_refresh"] = paymentPrepareData(prepared)
	applyGoPayPaymentPrepareResponse(&output, prepared)
	step.progress("gopay payment checkout refreshed", map[string]any{
		"success":             prepared != nil && prepared.GetSuccess(),
		"flow_id_present":     output.GetFlowId() != "",
		"checkout_session_id": output.GetCheckoutSessionId(),
		"checkout_attempt":    output.GetCheckoutAttempt(),
	})
	if err != nil {
		return output, err
	}
	if prepared == nil {
		return output, fmt.Errorf("payment prepare checkout refresh returned empty response")
	}
	if !prepared.GetSuccess() {
		return output, fmt.Errorf("payment prepare checkout refresh failed: %s", prepared.GetErrorMessage())
	}
	return output, nil
}

func (s *Server) prepareGoPayPaymentLink(ctx context.Context, step activityStep, input GoPayActivityInput) (output GoPayPaymentPrepareOutput, err error) {
	flowID := strings.TrimSpace(input.GetPreparedFlowId())
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	data := map[string]any{
		"stage":                 "link",
		"prepared_flow_present": flowID != "",
	}
	output = GoPayPaymentPrepareOutput{
		FlowId:    flowID,
		StateJson: stateJSON,
		Stage:     "link",
	}
	defer func() {
		output.StateJson = stateJSON
		output.Data = protoData(data)
	}()
	if flowID == "" {
		return output, fmt.Errorf("prepared_flow_id is required")
	}

	step.progress("linking gopay payment checkout", map[string]any{
		"flow_id_present": true,
	})
	prepared, err := s.paymentClient.PrepareGoPayLink(ctx, &pb.PrepareGoPayLinkRequest{FlowId: flowID})
	data["payment_prepare_link"] = paymentPrepareData(prepared)
	applyGoPayPaymentPrepareResponse(&output, prepared)
	step.progress("gopay payment checkout linked", map[string]any{
		"success":                  prepared != nil && prepared.GetSuccess(),
		"flow_id_present":          output.GetFlowId() != "",
		"snap_token_present":       output.GetSnapToken() != "",
		"retryable_fresh_checkout": output.GetRetryableFreshCheckout(),
		"checkout_attempt":         output.GetCheckoutAttempt(),
	})
	if err != nil {
		return output, err
	}
	if prepared == nil {
		return output, fmt.Errorf("payment prepare link returned empty response")
	}
	if output.GetRetryableFreshCheckout() {
		data["fresh_checkout_required"] = true
		return output, nil
	}
	if !prepared.GetSuccess() {
		return output, fmt.Errorf("payment prepare link failed: %s", prepared.GetErrorMessage())
	}
	if output.GetFlowId() == "" {
		return output, fmt.Errorf("payment prepare link returned empty flow_id")
	}
	if output.GetSnapToken() == "" {
		return output, fmt.Errorf("payment prepare link returned empty snap_token")
	}
	return output, nil
}
