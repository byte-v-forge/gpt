package activities

import (
	"fmt"
	"strings"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func commandResultMap(results []*browserautomationv1.BrowserCommandResult, commandID string) map[string]any {
	for _, result := range results {
		if result.GetCommandId() != commandID {
			continue
		}
		if result.GetJsonValue() == nil {
			return nil
		}
		if value, ok := result.GetJsonValue().AsInterface().(map[string]any); ok {
			return value
		}
		return nil
	}
	return nil
}

func commandResultCurrentURL(results []*browserautomationv1.BrowserCommandResult, commandID string) string {
	for _, result := range results {
		if result.GetCommandId() == commandID {
			return strings.TrimSpace(result.GetCurrentUrl())
		}
	}
	return ""
}

func browserAuthCommandSucceeded(results []*browserautomationv1.BrowserCommandResult, commandID string) bool {
	for _, result := range results {
		if result.GetCommandId() == commandID {
			return result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_SUCCEEDED
		}
	}
	return false
}

func browserAuthMatchedCount(results []*browserautomationv1.BrowserCommandResult, commandID string) int {
	for _, result := range results {
		if result.GetCommandId() == commandID &&
			result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_SUCCEEDED {
			return int(result.GetMatchedCount())
		}
	}
	return 0
}

func browserAuthAnyCommandSucceeded(results []*browserautomationv1.BrowserCommandResult, commandIDs ...string) bool {
	wanted := map[string]bool{}
	for _, commandID := range commandIDs {
		wanted[commandID] = true
	}
	for _, result := range results {
		if wanted[result.GetCommandId()] && result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_SUCCEEDED {
			return true
		}
	}
	return false
}

func browserAuthPageStateData(results []*browserautomationv1.BrowserCommandResult, commandID string) map[string]any {
	data := commandResultMap(results, commandID)
	if data == nil {
		return nil
	}
	out := map[string]any{}
	if url := sanitizeBrowserAuthURL(stringMapValue(data, "url")); url != "" {
		out["url"] = url
	}
	if title := stringMapValue(data, "title"); title != "" {
		out["title"] = title
	}
	if inputs := browserAuthStringList(data["inputs"], 5, 120); len(inputs) > 0 {
		out["inputs"] = inputs
	}
	if actions := browserAuthStringList(data["actions"], 8, 120); len(actions) > 0 {
		out["actions"] = actions
	} else if hints := browserAuthTextHints(stringMapValue(data, "text")); len(hints) > 0 {
		out["actions"] = hints
	}
	return out
}

func browserAuthPageHasAny(data map[string]any, terms ...string) bool {
	if data == nil {
		return false
	}
	haystack := strings.ToLower(fmt.Sprint(data["url"]) + " " + fmt.Sprint(data["title"]) + " " + fmt.Sprint(data["inputs"]) + " " + fmt.Sprint(data["actions"]))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(haystack, term) {
			return true
		}
	}
	return false
}
