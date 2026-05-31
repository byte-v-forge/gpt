package api

import (
	"strings"
)

func normalizeN8NAuthIDs(jobID string, accountID string, n8nExecutionID string) (string, string, string) {
	return strings.TrimSpace(jobID), strings.TrimSpace(accountID), strings.TrimSpace(n8nExecutionID)
}

func n8nAuthResultLabel(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "auth"
}
