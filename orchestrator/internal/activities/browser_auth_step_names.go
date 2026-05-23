package activities

import (
	"fmt"
)

func browserAuthStartStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountStart, nil
	case browserAuthModeLogin:
		return stepLoginSessionStart, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}

func browserAuthBrowserStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountBrowser, nil
	case browserAuthModeLogin:
		return stepLoginSessionBrowser, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}

func browserAuthOTPRequestStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountOTPRequest, nil
	case browserAuthModeLogin:
		return stepLoginSessionOTPRequest, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}

func browserAuthCompleteStepName(mode string) (string, error) {
	switch mode {
	case browserAuthModeRegister:
		return stepRegisterAccountComplete, nil
	case browserAuthModeLogin:
		return stepLoginSessionComplete, nil
	default:
		return "", fmt.Errorf("unsupported browser auth mode: %s", mode)
	}
}
