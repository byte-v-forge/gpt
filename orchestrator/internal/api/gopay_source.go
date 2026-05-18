package api

import (
	"fmt"
	"strings"
)

func normalizeGoPayUserID(value string) (string, error) {
	source, err := normalizeGoPaySource(value)
	if err != nil {
		return "", fmt.Errorf("user_id must be local or tg:<user_id>")
	}
	return source, nil
}

func normalizeGoPaySource(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "local" {
		return "local", nil
	}
	if strings.HasPrefix(value, "tg:") {
		userID := strings.TrimSpace(strings.TrimPrefix(value, "tg:"))
		if validTelegramUserID(userID) {
			return "tg:" + userID, nil
		}
	}
	return "", fmt.Errorf("user_id must be local or tg:<user_id>")
}

func validTelegramUserID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
