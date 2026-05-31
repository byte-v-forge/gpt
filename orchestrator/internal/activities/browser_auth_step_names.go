package activities

import (
	"fmt"
	"orchestrator/internal/contracts"
)

func browserAuthStartStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return contracts.StepRegisterAccountStart, nil
	case browserAuthModeLogin:
		return contracts.StepLoginSessionStart, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}

func browserAuthOTPRequestStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return contracts.StepRegisterAccountOTPRequest, nil
	case browserAuthModeLogin:
		return contracts.StepLoginSessionOTPRequest, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}

func browserAuthCompleteStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return contracts.StepRegisterAccountComplete, nil
	case browserAuthModeLogin:
		return contracts.StepLoginSessionComplete, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}
