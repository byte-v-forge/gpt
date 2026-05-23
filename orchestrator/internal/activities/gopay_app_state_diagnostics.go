package activities

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func goPayAppStateDiagnostics(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	data := map[string]any{"state_present": raw != ""}
	if raw == "" {
		return data
	}
	var state map[string]any
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		data["state_parse_error"] = err.Error()
		return data
	}
	proxy := stringFromAny(state["_gopay_proxy"])
	if proxy != "" {
		data["proxy_hash"] = shortStateHash(proxy)
	}
	if sessionHash := stringFromAny(state["_proxy_runtime_session_hash"]); sessionHash != "" {
		data["proxy_runtime_session_hash"] = sessionHash
	}
	if rotated, ok := state["_proxy_runtime_session_rotated"].(bool); ok {
		data["proxy_runtime_session_rotated"] = rotated
	}
	if endpoints := intFromAny(state["_proxy_runtime_pool_endpoints"]); endpoints > 0 {
		data["proxy_runtime_pool_endpoints"] = endpoints
	}
	if scope := stringFromAny(state["_signup_rate_limit_scope"]); scope != "" {
		data["signup_rate_limit_scope"] = scope
		data["signup_rate_limited_at"] = intFromAny(state["_signup_rate_limited_at"])
		data["signup_cooldown_until"] = intFromAny(state["_signup_cooldown_until"])
	}
	if delay := intFromAny(state["_signup_initiate_delay_seconds"]); delay > 0 {
		data["signup_initiate_delay_seconds"] = delay
	}
	if device := mapFromAny(state["device"]); len(device) > 0 {
		data["device_fingerprint"] = goPayAppDeviceFingerprint(device)
		data["device_profile"] = goPayAppDeviceProfile(device)
	}
	return data
}

func goPayAppDeviceFingerprint(device map[string]any) string {
	out := []string{}
	addPlain := func(label, key string) {
		if value := stringFromAny(device[key]); value != "" {
			out = append(out, label+"="+value)
		}
	}
	addHash := func(label, key string) {
		if value := stringFromAny(device[key]); value != "" {
			out = append(out, label+"#"+shortStateHash(value))
		}
	}
	addPlain("profile", "profile_id")
	addPlain("make", "x-phonemake")
	addPlain("model", "x-phonemodel")
	addPlain("os", "x-deviceos")
	addPlain("screen", "m1_screen")
	addPlain("tls", "tls_profile")
	addHash("uid", "x-uniqueid")
	addHash("session", "x-session-id")
	addHash("tx", "transaction-id")
	addHash("d1", "d1")
	addHash("conn", "m1_connection_id")
	addHash("widevine", "m1_widevine_id")
	addHash("wifi", "m1_wifi_mac")
	addHash("ssid", "m1_wifi_ssid")
	addHash("sig", "m1_signature")
	addHash("sig_time", "m1_signature_time")
	addHash("firebase", "m1_firebase_app_instance_id")
	addHash("uuid", "m1_device_uuid")
	addHash("adid", "advertising_id")
	addHash("appset", "app_set_id")
	addHash("devtoken", "x-devicetoken")
	addHash("imei", "x-imei")
	addHash("ip", "x-ipaddress")
	if xM1 := goPayAppXM1(device); xM1 != "" {
		out = append(out, "x_m1#"+shortStateHash(xM1))
	}
	return strings.Join(out, "/")
}

func goPayAppDeviceProfile(device map[string]any) map[string]any {
	profile := map[string]any{}
	putPlain := func(outKey, key string) {
		if value := stringFromAny(device[key]); value != "" {
			profile[outKey] = value
		}
	}
	putHash := func(outKey, key string) {
		if value := stringFromAny(device[key]); value != "" {
			profile[outKey] = shortStateHash(value)
		}
	}
	putPlain("profile_id", "profile_id")
	putPlain("app_version", "x-appversion")
	putPlain("app_id", "x-appid")
	putPlain("platform", "x-platform")
	putPlain("phone_make", "x-phonemake")
	putPlain("phone_model", "x-phonemodel")
	putPlain("device_os", "x-deviceos")
	putPlain("user_agent", "user-agent")
	putPlain("screen", "m1_screen")
	putPlain("tls_profile", "tls_profile")
	putPlain("gojek_country_code", "gojek-country-code")
	putPlain("location", "x-location")
	putPlain("location_accuracy", "x-location-accuracy")
	putHash("unique_id_hash", "x-uniqueid")
	putHash("session_id_hash", "x-session-id")
	putHash("transaction_id_hash", "transaction-id")
	putHash("d1_hash", "d1")
	putHash("appsflyer_id_hash", "m1_appsflyer_id")
	putHash("widevine_id_hash", "m1_widevine_id")
	putHash("wifi_mac_hash", "m1_wifi_mac")
	putHash("wifi_ssid_hash", "m1_wifi_ssid")
	putHash("m1_signature_hash", "m1_signature")
	putHash("m1_signature_time_hash", "m1_signature_time")
	putHash("firebase_app_instance_id_hash", "m1_firebase_app_instance_id")
	putHash("m1_device_uuid_hash", "m1_device_uuid")
	putHash("advertising_id_hash", "advertising_id")
	putHash("app_set_id_hash", "app_set_id")
	putHash("imei_hash", "x-imei")
	putHash("ip_address_hash", "x-ipaddress")
	putHash("device_token_hash", "x-devicetoken")
	putHash("user_uuid_hash", "user-uuid")
	putPlain("installer_package", "installer_package")
	putPlain("gms_version", "gms_version")
	if xM1 := goPayAppXM1(device); xM1 != "" {
		profile["x_m1_hash"] = shortStateHash(xM1)
	}
	return profile
}

func goPayAppXM1(device map[string]any) string {
	fields := []string{
		"3:" + firstNonEmptyString(stringFromAny(device["m1_appsflyer_id"]), "0-0"),
		"4:" + firstNonEmptyString(stringFromAny(device["m1_connection_id"]), "131072"),
		"5:" + stringFromAny(device["x-phonemake"]) + "|3200|2",
		"6:" + firstNonEmptyString(stringFromAny(device["m1_wifi_mac"]), "02:00:00:00:00:00"),
		"7:" + firstNonEmptyString(stringFromAny(device["m1_wifi_ssid"]), "<unknown ssid>"),
		"8:" + firstNonEmptyString(stringFromAny(device["m1_screen"]), "1080x2148"),
		"9:passive,network,fused,gps",
		"10:1",
		"11:" + stringFromAny(device["m1_widevine_id"]),
		"13:" + stringFromAny(device["m1_signature"]),
		"14:" + stringFromAny(device["m1_signature_time"]),
		"15:" + stringFromAny(device["m1_firebase_app_instance_id"]),
		"16:" + stringFromAny(device["m1_device_uuid"]),
	}
	return strings.Join(fields, ",")
}

func shortStateHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])[:12]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapFromAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok && typed != nil {
		return typed
	}
	return map[string]any{}
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}
