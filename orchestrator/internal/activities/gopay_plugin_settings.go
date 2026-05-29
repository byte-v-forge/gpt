package activities

import (
	"context"
	"time"

	"orchestrator/internal/gptsettings"
)

func (s *Server) goPayPluginValues(ctx context.Context) map[string]string {
	return s.pluginValues(ctx, "gopay")
}

func (s *Server) changePhoneDisabledValue(ctx context.Context) bool {
	return gptsettings.BoolValue(s.goPayPluginValues(ctx), "change_phone_disabled", false)
}

func seconds(value time.Duration) int32 {
	if value <= 0 {
		return 0
	}
	return int32(value / time.Second)
}
