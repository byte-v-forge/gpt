package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Server) waitForManualGoPayPaymentAction(ctx context.Context, jobID string, timeoutSeconds int32) error {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1800
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for {
		confirmed, found, err := s.jobStore.GetParam(ctx, jobID, manualGoPayPaymentConfirmParam)
		if err != nil {
			return err
		}
		if found && strings.EqualFold(strings.TrimSpace(confirmed), "true") {
			_ = s.jobStore.DeleteParam(ctx, jobID, manualGoPayPaymentConfirmParam)
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("manual gopay payment not confirmed after %ds", timeoutSeconds)
		}
		wait := time.Second
		if remaining < wait {
			wait = remaining
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func stringMapValue(data map[string]any, key string) string {
	if data[key] == nil {
		return ""
	}
	return fmt.Sprint(data[key])
}
