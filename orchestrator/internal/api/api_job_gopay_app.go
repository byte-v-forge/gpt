package api

import (
	"fmt"
	"strings"

	"orchestrator/pb"
)

const (
	goPayAppPinSecretKey = "gopay_app_pin:"

	goPayAppOperationProvision      = "provision"
	goPayAppOperationLogin          = "login"
	goPayAppOperationSignup         = "signup"
	goPayAppOperationEnsurePINSetup = "ensure_pin_setup"
	goPayAppOperationCheckBalance   = "check_balance"
	goPayAppOperationCheckPIN       = "check_pin"
	goPayAppOperationChangePhone    = "change_phone"
)

func normalizeGoPayAppOperation(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", goPayAppOperationProvision, "full":
		return goPayAppOperationProvision
	case "auth", "logon", goPayAppOperationLogin:
		return goPayAppOperationLogin
	case "balance", "check-balance", goPayAppOperationCheckBalance:
		return goPayAppOperationCheckBalance
	case "check-pin", "pin_check", goPayAppOperationCheckPIN:
		return goPayAppOperationCheckPIN
	case "register", goPayAppOperationSignup:
		return goPayAppOperationSignup
	case "pin", "set_pin", "create-pin", "create_pin", "ensure-pin-setup", goPayAppOperationEnsurePINSetup:
		return goPayAppOperationEnsurePINSetup
	case "change", "rebind", "change-phone", goPayAppOperationChangePhone:
		return goPayAppOperationChangePhone
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func goPayAppOperationStep(operation string) string {
	switch operation {
	case goPayAppOperationLogin, goPayAppOperationCheckBalance, goPayAppOperationCheckPIN:
		return stepGoPayAppLogin
	case goPayAppOperationSignup:
		return stepGoPayAppSignup
	case goPayAppOperationEnsurePINSetup:
		return stepGoPayAppEnsurePINSetup
	case goPayAppOperationChangePhone:
		return stepGoPayAppChangePhone
	default:
		return "gopay_app_" + operation
	}
}

func goPayAppUserID(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return goPayLocalSource
}

func goPayAppStepFromDeactivateComplete(output pb.GoPayAppDeactivateCompleteOutput) pb.GoPayAppStepOutput {
	return pb.GoPayAppStepOutput{ActivationId: output.GetActivationId(), DeactivateComplete: output.GetDeactivateComplete(), Data: output.GetData(), StateJson: output.GetStateJson()}
}

func goPaySignupOTPNotReceivedError(wait pb.OTPWaitOutput) error {
	reason := wait.GetErrorMessage()
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("gopay signup otp not received: %s", reason)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
