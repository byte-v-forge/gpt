package activities

import (
	"fmt"
	"github.com/byte-v-forge/common-lib/stringx"
	"strings"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func browserAuthState(results []*browserautomationv1.BrowserCommandResult, commandID string) string {
	state, _ := browserAuthStateData(results, commandID)
	return state
}

func browserAuthStateData(results []*browserautomationv1.BrowserCommandResult, commandID string) (string, map[string]any) {
	data := commandResultMap(results, commandID)
	if data == nil {
		return "", nil
	}
	return strings.TrimSpace(stringMapValue(data, "state")), data
}

func browserAuthNetworkRequestStartedAtUnixMs(results []*browserautomationv1.BrowserCommandResult, commandID string) int64 {
	data := commandResultMap(results, commandID)
	if data == nil {
		return 0
	}
	request, ok := data["request"].(map[string]any)
	if !ok {
		return 0
	}
	return int64MapValue(request, "started_at_unix_ms")
}

func browserAuthStepError(mode, step, state string, data map[string]any) error {
	if state == "" {
		state = "unknown"
	}
	return fmt.Errorf("browser %s %s step failed: %s%s", mode, step, state, browserAuthFailureContext(data))
}

func browserAuthFailureContext(data map[string]any) string {
	if data == nil {
		return ""
	}
	fields := make([]string, 0, 4)
	appendText := func(key, label string, max int) {
		if value := stringx.CompactSnippet(stringMapValue(data, key), max); value != "" {
			fields = append(fields, label+"="+value)
		}
	}
	appendList := func(key, label string) {
		if values := browserAuthStringList(data[key], 5, 80); len(values) > 0 {
			fields = append(fields, label+"="+strings.Join(values, " | "))
		}
	}
	appendText("url", "url", 160)
	appendText("title", "title", 120)
	appendList("inputs", "inputs")
	appendList("actions", "actions")
	if len(fields) == 0 {
		return ""
	}
	return " (" + strings.Join(fields, "; ") + ")"
}

func browserAuthStringList(value any, limit, maxLen int) []string {
	out := make([]string, 0, limit)
	add := func(raw any) {
		if len(out) >= limit {
			return
		}
		if text := stringx.CompactSnippet(fmt.Sprint(raw), maxLen); text != "" {
			out = append(out, text)
		}
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			add(item)
		}
	case []string:
		for _, item := range typed {
			add(item)
		}
	case string:
		add(typed)
	}
	return out
}
