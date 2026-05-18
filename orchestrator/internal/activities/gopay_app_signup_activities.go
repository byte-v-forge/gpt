package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pb "orchestrator/pb"
)

func (s *Server) startGoPayAppSignup(ctx context.Context, input GoPayAppOTPStartInput) (GoPayAppOTPOutput, error) {
	stepName := stepGoPayAppSignup
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	if input.GetResetState() {
		stateJSON = "{}"
	}
	output := GoPayAppOTPOutput{
		Operation:      goPayAppOTPOperationSignup,
		TimeoutSeconds: s.paymentOtpTimeout(),
		StateJson:      stateJSON,
	}
	otpChannel := normalizeGoPayOTPChannel(input.GetOtpChannel())
	output.OtpChannel = otpChannel
	data := map[string]any{
		"operation":   goPayAppOTPOperationSignup,
		"otp_channel": otpChannel,
	}
	defer func() {
		output.Data = protoData(data)
	}()

	step, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true)
	if err != nil {
		return output, err
	}
	if s.gopayClient == nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay app client not configured"))
	}
	if input.GetResetState() {
		data["state_reset"] = true
	}
	phone := strings.TrimSpace(input.GetPhone())
	if phone == "" {
		phone = configuredGoPayPhone()
	}
	if phone == "" {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("GOPAY_PHONE_NUMBER is required"))
	}

	statusBefore, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusBefore))
	data["status_before"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusBefore, statusErr))
	if statusErr != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("Status before signup: %w", statusErr))
	}
	output.Stage = statusBefore.GetStage()
	output.Phone = statusBefore.GetPhone()
	stage := strings.TrimSpace(statusBefore.GetStage())
	if goPayStatusTokenReady(statusBefore) {
		output.Ready = true
		output.AccountTokenReady = true
		output.SignupComplete = true
		data["ready"] = true
		data["account_token_ready"] = true
		data["signup_complete"] = true
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, nil)
	}
	if stage == "signup_pin_required" || stage == "signup_pin_otp_pending" {
		output.SignupComplete = true
		output.PinSetupRequired = true
		data["signup_complete"] = true
		data["pin_setup_required"] = true
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, nil)
	}
	if stage == "signup_otp_pending" {
		output.OtpRequired = true
		output.IssuedAfterUnix = statusBefore.GetSignupOtpSentAtUnix()
		if output.GetIssuedAfterUnix() <= 0 {
			output.IssuedAfterUnix = time.Now().Unix()
		}
		data["otp_required"] = true
		data["issued_after_unix"] = output.GetIssuedAfterUnix()
		step.update(data)
		return output, nil
	}

	startedAt := time.Now().Unix()
	startResp, err := s.gopayClient.SignupStart(ctx, &pb.SignupStartRequest{
		Phone:       phone,
		CountryCode: configuredGoPayCountryCode(),
		OtpChannel:  goPayOTPMethod(otpChannel),
		StateJson:   output.GetStateJson(),
	})
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(startResp))
	if err == nil && strings.TrimSpace(input.GetSmsActivationId()) != "" {
		output.StateJson, err = goPayStateWithSignupSMSActivationID(output.GetStateJson(), input.GetSmsActivationId())
	}
	data["signup_start"] = signupStartData(startResp)
	if err != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup start: %w", err))
	}
	if startResp == nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup start returned empty response"))
	}
	output.VerificationMethod = startResp.GetVerificationMethod()
	if output.GetOtpChannel() == "" {
		output.OtpChannel = goPayOTPChannelFromMethod(startResp.GetVerificationMethod())
	}
	if !startResp.GetSuccess() {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup start: %s", startResp.GetErrorMessage()))
	}

	statusAfter, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusAfter))
	data["status_after"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusAfter, statusErr))
	if statusErr != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("Status after gopay signup start: %w", statusErr))
	}
	output.Stage = statusAfter.GetStage()
	output.Phone = statusAfter.GetPhone()
	if !startResp.GetOtpSent() {
		stage = strings.TrimSpace(statusAfter.GetStage())
		if goPayStatusTokenReady(statusAfter) || stage == "signup_pin_required" || stage == "signup_pin_otp_pending" {
			output.SignupComplete = true
			output.PinSetupRequired = stage == "signup_pin_required" || stage == "signup_pin_otp_pending"
			data["signup_complete"] = true
			data["pin_setup_required"] = output.GetPinSetupRequired()
			return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, nil)
		}
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup start did not send OTP and gopay app not ready: stage=%s", statusAfter.GetStage()))
	}

	output.OtpRequired = true
	output.IssuedAfterUnix = authOtpIssuedAfterUnix(statusAfter, startedAt)
	if normalizeGoPayOTPChannel(output.GetOtpChannel()) == "sms" {
		if err := s.markGoPaySMSMessageSent(ctx, input.GetSmsActivationId(), data); err != nil {
			return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, err)
		}
	}
	data["otp_required"] = true
	data["issued_after_unix"] = output.GetIssuedAfterUnix()
	data["verification_method"] = output.GetVerificationMethod()
	data["otp_channel"] = output.GetOtpChannel()
	step.update(data)
	return output, nil
}

func goPayStateWithSignupSMSActivationID(stateJSON string, activationID string) (string, error) {
	activationID = strings.TrimSpace(activationID)
	if activationID == "" {
		return normalizeGoPayWorkflowStateJSON(stateJSON), nil
	}
	var state map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stateJSON)), &state); err != nil {
		return "", err
	}
	if state == nil {
		state = map[string]any{}
	}
	state["_signup_sms_activation_id"] = activationID
	next, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(next), nil
}

func (s *Server) retryGoPayAppSignupOTP(ctx context.Context, input GoPayAppOTPStartInput) (GoPayAppOTPOutput, error) {
	stepName := stepGoPayAppSignupRetry
	output := GoPayAppOTPOutput{
		Operation:      goPayAppOTPOperationSignup,
		TimeoutSeconds: s.paymentOtpTimeout(),
		StateJson:      normalizeGoPayWorkflowStateJSON(input.GetStateJson()),
	}
	otpChannel := normalizeGoPayOTPChannel(input.GetOtpChannel())
	output.OtpChannel = otpChannel
	data := map[string]any{
		"operation":         goPayAppOTPOperationSignup,
		"otp_channel":       otpChannel,
		"sms_activation_id": input.GetSmsActivationId(),
	}
	defer func() {
		output.Data = protoData(data)
	}()

	if _, err := s.startActivityStep(ctx, input.GetJobId(), stepName, false, true); err != nil {
		return output, err
	}
	if s.gopayClient == nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay app client not configured"))
	}

	startedAt := time.Now().Unix()
	retryResp, err := s.gopayClient.SignupRetry(ctx, &pb.SignupRetryRequest{StateJson: output.GetStateJson()})
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(retryResp))
	data["signup_retry"] = signupRetryData(retryResp)
	if err != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup retry: %w", err))
	}
	if retryResp == nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup retry returned empty response"))
	}
	if !retryResp.GetSuccess() {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup retry: %s", retryResp.GetErrorMessage()))
	}

	statusAfter, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusAfter))
	data["status_after"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusAfter, statusErr))
	if statusErr != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("Status after gopay signup retry: %w", statusErr))
	}
	output.Stage = statusAfter.GetStage()
	output.Phone = statusAfter.GetPhone()
	output.OtpRequired = retryResp.GetOtpSent()
	output.IssuedAfterUnix = authOtpIssuedAfterUnix(statusAfter, startedAt)
	if output.GetIssuedAfterUnix() <= 0 {
		output.IssuedAfterUnix = startedAt
	}
	data["otp_required"] = output.GetOtpRequired()
	data["issued_after_unix"] = output.GetIssuedAfterUnix()
	data["otp_channel"] = output.GetOtpChannel()
	return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, nil)
}

func (s *Server) completeGoPayAppSignup(ctx context.Context, input GoPayAppOTPCompleteInput) (GoPayAppOTPOutput, error) {
	stepName := stepGoPayAppSignup
	output := GoPayAppOTPOutput{
		Operation:      goPayAppOTPOperationSignup,
		TimeoutSeconds: s.paymentOtpTimeout(),
		StateJson:      normalizeGoPayWorkflowStateJSON(input.GetStateJson()),
	}
	output.OtpChannel = normalizeGoPayOTPChannel(input.GetOtpChannel())
	data := protoDataMap(input.GetData())
	if data == nil {
		data = map[string]any{}
	}
	defer func() {
		output.Data = protoData(data)
	}()

	otp, err := s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetIssuedAfterUnix())
	if err != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, err)
	}
	completeResp, err := s.gopayClient.SignupComplete(ctx, &pb.SignupCompleteRequest{Otp: otp, StateJson: output.GetStateJson()})
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(completeResp))
	data["signup_complete"] = signupCompleteData(completeResp)
	data["otp_source"] = input.GetOtpSource()
	if err != nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup complete: %w", err))
	}
	if completeResp == nil {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup complete returned empty response"))
	}
	if !completeResp.GetSuccess() {
		return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, fmt.Errorf("gopay signup complete: %s", completeResp.GetErrorMessage()))
	}

	output.SignupComplete = true
	output.PinSetupRequired = completeResp.GetPinSetupRequired()
	output.Phone = completeResp.GetPhone()
	data["signup_complete"] = true
	data["pin_setup_required"] = output.GetPinSetupRequired()
	statusAfter, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusAfter))
	data["status_after"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusAfter, statusErr))
	if statusErr == nil {
		output.Stage = statusAfter.GetStage()
		output.Phone = statusAfter.GetPhone()
		output.Ready = goPayStatusTokenReady(statusAfter)
		output.AccountTokenReady = output.GetReady()
	}
	return output, s.completeGoPayAppOTPStep(ctx, input.GetJobId(), stepName, data, statusErr)
}
