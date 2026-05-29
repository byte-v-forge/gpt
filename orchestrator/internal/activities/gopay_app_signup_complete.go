package activities

import (
	"context"
	"fmt"

	pb "orchestrator/pb"
)

func (s *Server) completeGoPayAppSignup(ctx context.Context, input GoPayAppOTPCompleteInput) (GoPayAppOTPOutput, error) {
	stepName := stepGoPayAppSignup
	output := GoPayAppOTPOutput{
		Operation:      goPayAppOTPOperationSignup,
		TimeoutSeconds: s.paymentOtpTimeout(ctx),
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
