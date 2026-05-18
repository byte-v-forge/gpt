package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func normalizeGoPayOTPChannel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sms", "otp_sms":
		return "sms"
	case "wa", "whatsapp", "otp_wa":
		return "wa"
	default:
		return ""
	}
}

func goPayOTPMethod(channel string) string {
	switch normalizeGoPayOTPChannel(channel) {
	case "sms":
		return "sms"
	case "wa":
		return "wa"
	default:
		return ""
	}
}

func goPayOTPChannelFromMethod(method string) string {
	return normalizeGoPayOTPChannel(method)
}

func (s *Server) markGoPaySMSMessageSent(ctx context.Context, activationID string, data map[string]any) error {
	if strings.TrimSpace(activationID) == "" {
		return fmt.Errorf("sms activation id missing")
	}
	if s.smsClient == nil {
		return fmt.Errorf("sms client not configured")
	}
	resp, err := s.smsClient.MarkMessageSent(ctx, &pb.MarkMessageSentRequest{ActivationId: activationID})
	data["sms_mark_sent"] = providerActionData(resp, err)
	if err != nil {
		return fmt.Errorf("MarkMessageSent: %w", err)
	}
	if resp == nil || !resp.GetSuccess() {
		message := ""
		if resp != nil {
			message = resp.GetErrorMessage()
		}
		if message == "" {
			message = "empty response"
		}
		return fmt.Errorf("MarkMessageSent: %s", message)
	}
	return nil
}

func (s *Server) finishGoPayAppOTPReady(ctx context.Context, jobID, stepName string, output GoPayAppOTPOutput, data map[string]any) (GoPayAppOTPOutput, error) {
	stateJSON := normalizeGoPayWorkflowStateJSON(output.GetStateJson())
	tokenResp, nextStateJSON, err := s.validateGoPayAccountTokenForState(ctx, stateJSON)
	output.StateJson = nextStateJSON
	data["check_token_valid_after"] = checkTokenValidData(tokenResp)
	if err != nil {
		return output, s.completeGoPayAppOTPStep(ctx, jobID, stepName, data, err)
	}
	if !tokenResp.GetTokenValid() {
		message := strings.TrimSpace(tokenResp.GetErrorMessage())
		if message == "" {
			message = "token invalid"
		}
		return output, s.completeGoPayAppOTPStep(ctx, jobID, stepName, data, fmt.Errorf("%s", message))
	}
	statusAfter, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusAfter))
	data["status_after"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusAfter, statusErr))
	if statusErr != nil {
		return output, s.completeGoPayAppOTPStep(ctx, jobID, stepName, data, statusErr)
	}
	output.Ready = goPayStatusTokenReady(statusAfter)
	output.AccountTokenReady = true
	output.Stage = statusAfter.GetStage()
	output.Phone = statusAfter.GetPhone()
	data["ready"] = output.GetReady()
	data["account_token_ready"] = true
	return output, s.completeGoPayAppOTPStep(ctx, jobID, stepName, data, nil)
}

func (s *Server) completeGoPayAppOTPStep(ctx context.Context, jobID, stepName string, data map[string]any, stepErr error) error {
	if data == nil {
		data = map[string]any{}
	}
	if stepErr != nil {
		data["error_message"] = stepErr.Error()
	}
	return s.completeActivityStep(ctx, jobID, stepName, false, true, data, stepErr)
}

func gopayAppOTPStepName(input GoPayAppOTPStartInput) string {
	if stepName := strings.TrimSpace(input.GetStepName()); stepName != "" {
		return stepName
	}
	return stepGoPayAppLogin
}

func authStartData(resp *pb.AuthStartResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":    true,
		"success":             resp.GetSuccess(),
		"error_message":       resp.GetErrorMessage(),
		"mode":                resp.GetMode(),
		"stage":               resp.GetStage(),
		"otp_sent":            resp.GetOtpSent(),
		"verification_method": resp.GetVerificationMethod(),
		"ready":               resp.GetReady(),
		"pin_setup_required":  resp.GetPinSetupRequired(),
	}
}

func authCompleteData(resp *pb.AuthCompleteResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":     true,
		"success":              resp.GetSuccess(),
		"error_message":        resp.GetErrorMessage(),
		"mode":                 resp.GetMode(),
		"stage":                resp.GetStage(),
		"phone":                resp.GetPhone(),
		"ready":                resp.GetReady(),
		"pin_setup_required":   resp.GetPinSetupRequired(),
		"pin_setup_complete":   resp.GetPinSetupComplete(),
		"sensitive_values_set": false,
	}
}

func createPinStartData(resp *pb.CreatePinStartResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":    true,
		"success":             resp.GetSuccess(),
		"error_message":       resp.GetErrorMessage(),
		"otp_sent":            resp.GetOtpSent(),
		"verification_method": resp.GetVerificationMethod(),
	}
}

func createPinRetryData(resp *pb.CreatePinRetryResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present": true,
		"success":          resp.GetSuccess(),
		"error_message":    resp.GetErrorMessage(),
		"otp_sent":         resp.GetOtpSent(),
	}
}

func providerActionData(resp *pb.ProviderActionResponse, err error) map[string]any {
	data := map[string]any{"response_present": resp != nil}
	if resp != nil {
		data["success"] = resp.GetSuccess()
		data["error_message"] = resp.GetErrorMessage()
	}
	if err != nil {
		data["error"] = err.Error()
	}
	return data
}

func signupStartData(resp *pb.SignupStartResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":    true,
		"success":             resp.GetSuccess(),
		"error_message":       resp.GetErrorMessage(),
		"otp_sent":            resp.GetOtpSent(),
		"verification_method": resp.GetVerificationMethod(),
		"retry_timer_seconds": resp.GetRetryTimerSeconds(),
		"raw_json":            resp.GetRawJson(),
	}
}

func signupRetryData(resp *pb.SignupRetryResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present": true,
		"success":          resp.GetSuccess(),
		"error_message":    resp.GetErrorMessage(),
		"otp_sent":         resp.GetOtpSent(),
		"raw_json":         resp.GetRawJson(),
	}
}

func signupCompleteData(resp *pb.SignupCompleteResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":     true,
		"success":              resp.GetSuccess(),
		"error_message":        resp.GetErrorMessage(),
		"phone":                resp.GetPhone(),
		"pin_setup_required":   resp.GetPinSetupRequired(),
		"sensitive_values_set": false,
		"raw_json":             resp.GetRawJson(),
	}
}

func createPinCompleteData(resp *pb.CreatePinCompleteResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":     true,
		"success":              resp.GetSuccess(),
		"error_message":        resp.GetErrorMessage(),
		"phone":                resp.GetPhone(),
		"pin_setup_complete":   resp.GetPinSetupComplete(),
		"sensitive_values_set": false,
	}
}
