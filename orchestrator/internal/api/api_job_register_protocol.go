package api

import (
	"strings"

	"orchestrator/internal/jobstatus"
)

const protocolRegisterMode = "register"

func registerProtocolFailureStatus(err error) string {
	if isRegisterProtocolAlreadyExistsError(err) {
		return jobstatus.FailedFinal
	}
	return jobstatus.FailedRetryable
}

func registerProtocolRecoverable(error) bool {
	return false
}

func registerProtocolRetryable(err error) bool {
	return !isRegisterProtocolAlreadyExistsError(err)
}

func isRegisterProtocolAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	normalized = strings.NewReplacer("_", " ", "-", " ", ".", " ", ":", " ").Replace(normalized)
	return strings.Contains(normalized, "user already exist") ||
		strings.Contains(normalized, "account already exist")
}
