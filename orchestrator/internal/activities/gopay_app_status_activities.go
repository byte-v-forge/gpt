package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Server) GoPayAppStatusActivity(ctx context.Context, input GoPayAppStepInput) (GoPayAppStepOutput, error) {
	output := GoPayAppStepOutput{StateJson: normalizeGoPayWorkflowStateJSON(input.GetStateJson())}
	data := map[string]any{}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayAppStatus, false, true)
	_, err := step.run(func() (any, error) {
		statusResp, statusErr := s.goPayStatusForState(ctx, output.GetStateJson())
		output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(statusResp))
		data["status"] = goPayStatusSnapshotData(goPayStatusSnapshot(statusResp, statusErr))
		if statusErr != nil {
			err := fmt.Errorf("Status: %w", statusErr)
			data["error_message"] = err.Error()
			return data, err
		}
		if statusResp == nil {
			err := fmt.Errorf("Status returned empty response")
			data["error_message"] = err.Error()
			return data, err
		}
		output.Stage = statusResp.GetStage()
		output.Phone = statusResp.GetPhone()
		output.ActivationId = currentGoPaySignupSMSActivationID(output.GetStateJson())
		output.Ready = goPayStatusTokenReady(statusResp)
		output.AccountTokenReady = output.GetReady()
		switch statusResp.GetStage() {
		case "ready":
			output.SignupComplete = true
			output.SignupPinComplete = true
		case "signup_pin_required", "signup_pin_otp_pending":
			output.SignupComplete = true
		}
		data["ready"] = output.GetReady()
		data["account_token_ready"] = output.GetAccountTokenReady()
		data["signup_complete"] = output.GetSignupComplete()
		data["signup_pin_complete"] = output.GetSignupPinComplete()
		if output.GetActivationId() != "" {
			data["sms_activation_id"] = output.GetActivationId()
		}
		return data, nil
	})
	output.Data = protoData(data)
	return output, err
}

func currentGoPaySignupSMSActivationID(stateJSON string) string {
	var state map[string]any
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return ""
	}
	for _, key := range []string{"_signup_sms_activation_id", "signup_sms_activation_id"} {
		if value := strings.TrimSpace(fmt.Sprint(state[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}
