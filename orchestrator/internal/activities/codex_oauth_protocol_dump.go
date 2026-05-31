package activities

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/common-lib/stringx"
	"sort"
	"strings"

	"orchestrator/pb"
)

func fetchCodexOAuthClientAuthSessionDump(ctx context.Context, client *GptClient, state *pb.CodexOAuthProtocolState, data *codexOAuthStepData, stage string) error {
	if client == nil || state == nil || data == nil || !client.cfg.ProtocolSessionDumpEnabled {
		return nil
	}
	resp, err := client.get(ctx, "https://auth.openai.com/api/accounts/client_auth_session_dump", "https://auth.openai.com/email-verification", false)
	if err != nil {
		data.setClientAuthSessionDumpError(codexOAuthProtocolSafeText(err.Error(), 220))
		return nil
	}
	data.setClientAuthSessionDumpStatus(resp.StatusCode)
	if resp.StatusCode != 200 {
		return nil
	}
	payload := codexOAuthProtocolResponseJSON(resp)
	if payload == nil {
		data.setClientAuthSessionDumpError("json_missing")
		return nil
	}
	applyCodexOAuthClientAuthSessionDump(payload, state, data, stage)
	return nil
}

func applyCodexOAuthClientAuthSessionDump(payload map[string]any, state *pb.CodexOAuthProtocolState, data *codexOAuthStepData, stage string) {
	cas, _ := payload["client_auth_session"].(map[string]any)
	data.setClientAuthSessionDumpSeen(true)
	data.setClientAuthSessionKeys(codexOAuthProtocolMapKeys(cas, 24))
	if clientID := strings.TrimSpace(stringAny(cas["openai_client_id"])); clientID != "" {
		state.ClientId = clientID
		data.setClientAuthOpenAIClientIDPresent(true)
	}
	if sessionID := strings.TrimSpace(stringx.FirstNonEmptyAny(payload["session_id"], cas["session_id"])); sessionID != "" {
		data.setClientAuthSessionIDPresent(true)
	}
	if mode := strings.TrimSpace(stringAny(cas["email_verification_mode"])); mode != "" {
		data.setClientAuthEmailVerificationMode(mode)
	}
	if value, ok := boolAny(cas["email_verified"]); ok {
		data.setClientAuthEmailVerified(value)
	}
	applyCodexOAuthDumpPhoneState(cas, state, data, stage)
}

func applyCodexOAuthDumpPhoneState(cas map[string]any, state *pb.CodexOAuthProtocolState, data *codexOAuthStepData, stage string) {
	phone := strings.TrimSpace(stringAny(cas["phone_number"]))
	channel := strings.TrimSpace(stringAny(cas["phone_verification_channel"]))
	state.PhoneStateKnown = true
	state.PhonePresent = phone != ""
	state.PhoneVerificationChannel = channel
	if phone != "" {
		state.PhoneMask = maskPhone(phone, "")
	} else {
		state.PhoneMask = ""
	}
	data.setClientAuthPhoneStateKnown(true)
	data.setClientAuthPhonePresent(state.PhonePresent)
	data.setClientAuthPhoneStatus(codexOAuthProtocolPhoneStatus(state.PhonePresent, channel))
	if channel != "" {
		data.setClientAuthPhoneVerificationChannel(channel)
	}
	if state.PhoneMask != "" {
		data.setClientAuthPhoneMask(state.PhoneMask)
	}
	if strings.TrimSpace(stage) == "add_phone" && !state.PhonePresent {
		data.setAddPhoneRequired(true)
		data.setAddPhoneRequiredSource("client_auth_session_dump")
	}
}

func codexOAuthProtocolPhoneStatus(present bool, channel string) string {
	if present {
		if strings.TrimSpace(channel) != "" {
			return "present_verified_channel"
		}
		return "present"
	}
	if strings.TrimSpace(channel) != "" {
		return "channel_without_number"
	}
	return "missing"
}

func codexOAuthProtocolMapKeys(data map[string]any, limit int) []string {
	if len(data) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return keys
}

func codexOAuthProtocolStageFromDump(state *pb.CodexOAuthProtocolState, fallback string) string {
	if state != nil && state.PhoneStateKnown && state.PhonePresent && fallback == "add_phone" {
		return "consent"
	}
	return fallback
}

func stringAny(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func boolAny(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	}
	return false, false
}
