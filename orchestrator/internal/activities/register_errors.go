package activities

import (
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"strings"
)

func isAccountAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return isAccountAlreadyExistsMessage(err.Error())
}

func isAccountAlreadyExistsMessage(message string) bool {
	normalized := strings.ToLower(message)
	normalized = strings.NewReplacer("_", " ", "-", " ", ".", " ", ":", " ").Replace(normalized)

	return strings.Contains(normalized, "user already exist") ||
		strings.Contains(normalized, "account already exist")
}

func isUserAlreadyExistsStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case gptplugin.AccountStatusUserAlreadyExists, gptplugin.AccountStatusEmailAlreadyExists:
		return true
	default:
		return false
	}
}
