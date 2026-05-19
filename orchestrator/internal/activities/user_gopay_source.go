package activities

import (
	"fmt"

	"orchestrator/internal/gopayotp"
)

func normalizeGoPayUserID(value string) (string, error) {
	source, err := normalizeGoPaySource(value)
	if err != nil {
		return "", fmt.Errorf("user_id must be local or tg:<user_id>")
	}
	return source, nil
}

func normalizeGoPaySource(value string) (string, error) {
	source, err := gopayotp.NormalizeSource(value)
	if err != nil {
		return "", fmt.Errorf("user_id must be local or tg:<user_id>")
	}
	return source, nil
}

func goPayOTPQueueKey(source, purpose string) (string, error) {
	return gopayotp.QueueKey(source, purpose)
}
