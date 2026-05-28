package activities

import (
	"encoding/json"
	"strings"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"orchestrator/pb"
)

func browserAuthRegisterResponse(results []*browserautomationv1.BrowserCommandResult) *pb.RegisterResponse {
	cookiesData := commandResultMap(results, "capture-cookies")
	cookies := browserCookieMaps(cookiesData)
	session := browserAuthSessionFromResults(results)
	if session.sessionToken == "" {
		session.sessionToken = extractBrowserSessionToken(cookies)
	}
	return &pb.RegisterResponse{
		Success:           true,
		SessionToken:      session.sessionToken,
		AccessToken:       session.accessToken,
		DeviceId:          extractCookieValue(cookies, "oai-did", "oai-device-id"),
		PlusTrialEligible: false,
		PlusTrialChecked:  false,
	}
}

func browserAuthSessionFromResults(results []*browserautomationv1.BrowserCommandResult) browserAuthSession {
	data := browserAuthSessionData(results)
	session := browserAuthSession{}
	if token := stringMapValue(data, "sessionToken"); token != "" {
		session.sessionToken = token
	} else {
		session.sessionToken = stringMapValue(data, "session_token")
	}
	if token := stringMapValue(data, "accessToken"); token != "" {
		session.accessToken = token
	} else {
		session.accessToken = stringMapValue(data, "access_token")
	}
	return session
}

func browserAuthSessionData(results []*browserautomationv1.BrowserCommandResult) map[string]any {
	body := browserAuthCommandText(results, "extract-session-body")
	if body == "" {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil
	}
	return data
}

func browserAuthCommandText(results []*browserautomationv1.BrowserCommandResult, commandID string) string {
	for _, result := range results {
		if result.GetCommandId() == commandID {
			return strings.TrimSpace(result.GetText())
		}
	}
	return ""
}
