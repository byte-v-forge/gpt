package activities

import (
	"context"
	"fmt"
	"time"

	pb "orchestrator/pb"
)

func (s *Server) retryGoPayAppSignupOTP(ctx context.Context, input GoPayAppOTPStartInput) (GoPayAppOTPOutput, error) {
	stepName := stepGoPayAppSignupRetry
	output := GoPayAppOTPOutput{
		Operation:      goPayAppOTPOperationSignup,
		TimeoutSeconds: s.paymentOtpTimeout(ctx),
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
