package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/byte-v-forge/common-lib/hashx"
	"github.com/google/uuid"
	"orchestrator/db"
)

func (s *Server) saveAccountProxyUsage(ctx context.Context, input proxyUsageInput) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("database is not configured")
	}
	row := accountProxyUsageRow(input)
	return s.db.WithContext(ctx).Create(&row).Error
}

type proxyUsageInput struct {
	JobID          string
	AccountID      string
	N8NExecutionID string
	Purpose        string
	ProxyURL       string
	Data           map[string]any
}

func accountProxyUsageRow(input proxyUsageInput) db.AccountProxyUsage {
	data := input.Data
	raw, _ := json.Marshal(data)
	return db.AccountProxyUsage{
		ID:             uuid.NewString(),
		AccountID:      strings.TrimSpace(input.AccountID),
		JobID:          strings.TrimSpace(input.JobID),
		N8NExecutionID: strings.TrimSpace(input.N8NExecutionID),
		Purpose:        proxyUsagePurpose(input.Purpose, data),
		ProxyURLHash:   hashx.ShortSHA256(strings.TrimSpace(input.ProxyURL), 16),
		SessionIDHash:  sessionIDHash(data),
		ExitIP:         firstDataString(data, "exit_ip", "ip"),
		CountryCode:    firstDataString(data, "country_code", "exit_country", "country"),
		Region:         firstDataString(data, "region", "exit_region"),
		City:           firstDataString(data, "city", "exit_city"),
		AttemptIndex:   uint32(firstDataFloat(data, "attempt", "attempt_index")),
		Accepted:       firstDataBool(data, "accepted", "preflight_passed"),
		ErrorMessage:   firstDataString(data, "error_message", "error"),
		RawJSON:        string(raw),
	}
}

func proxyUsagePurpose(purpose string, data map[string]any) string {
	if purpose = strings.TrimSpace(purpose); purpose != "" {
		return purpose
	}
	return firstDataString(data, "purpose", "proxy_purpose", "usage")
}

func sessionIDHash(data map[string]any) string {
	if value := firstDataString(data, "session_id_hash", "proxy_session_hash"); value != "" {
		return value
	}
	return hashDataString(data, "session_id", "proxy_session_id")
}

func hashDataString(data map[string]any, keys ...string) string {
	value := firstDataString(data, keys...)
	if value == "" {
		return ""
	}
	return hashx.ShortSHA256(value, 16)
}

func firstDataString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := dataValue(data, key); value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func firstDataFloat(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := dataValue(data, key).(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case uint32:
			return float64(value)
		case json.Number:
			out, _ := value.Float64()
			return out
		case string:
			var out float64
			_, _ = fmt.Sscanf(value, "%f", &out)
			return out
		}
	}
	return 0
}

func firstDataBool(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := dataValue(data, key).(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}

func dataValue(data map[string]any, key string) any {
	if data == nil || key == "" {
		return nil
	}
	if value, ok := data[key]; ok {
		return value
	}
	for _, nestedKey := range []string{"ip_fraud_check", "edge_access_check", "exit_geo"} {
		if nested, ok := data[nestedKey].(map[string]any); ok {
			if value := dataValue(nested, key); value != nil {
				return value
			}
		}
	}
	return nil
}
