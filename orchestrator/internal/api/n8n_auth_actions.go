package api

import (
	"fmt"
	"strings"
)

type n8nProtocolAuthRuntimeProfile struct {
	Protocol n8nProtocolAuthProfile
	Proxy    n8nDynamicProxyProfile
}

func n8nBrowserAuthProfileForAction(actionID string) (n8nBrowserAuthProfile, error) {
	profile, err := n8nRuntimeProfileForAction("browser", actionID)
	if err != nil {
		return n8nBrowserAuthProfile{}, err
	}
	if profile.BrowserAuth == nil {
		return n8nBrowserAuthProfile{}, unsupportedN8NAuthActionError("browser", profile.ActionID)
	}
	return *profile.BrowserAuth, nil
}

func n8nProtocolAuthProfileForAction(actionID string) (n8nProtocolAuthRuntimeProfile, error) {
	profile, err := n8nRuntimeProfileForAction("protocol", actionID)
	if err != nil {
		return n8nProtocolAuthRuntimeProfile{}, err
	}
	if profile.ProtocolAuth == nil {
		return n8nProtocolAuthRuntimeProfile{}, unsupportedN8NAuthActionError("protocol", profile.ActionID)
	}
	return *profile.ProtocolAuth, nil
}

func unsupportedN8NAuthActionError(kind string, actionID string) error {
	if actionID == "" {
		actionID = "unknown"
	}
	return fmt.Errorf("unsupported %s auth action: %s", strings.TrimSpace(kind), actionID)
}
