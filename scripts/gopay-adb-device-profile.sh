#!/usr/bin/env bash
set -euo pipefail

pkg="${GOPAY_ANDROID_PACKAGE:-com.gojek.gopay}"
adb_args=(adb)
if [[ -n "${ADB_SERIAL:-}" ]]; then
  adb_args+=(-s "$ADB_SERIAL")
fi

trim() {
  sed -e 's/\r//g' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

first_line() {
  head -n 1 | trim
}

prop() {
  "${adb_args[@]}" shell getprop "$1" 2>/dev/null | first_line
}

setting() {
  "${adb_args[@]}" shell settings get "$1" "$2" 2>/dev/null | first_line
}

emit() {
  local key="$1"
  local value="${2:-}"
  [[ -z "$value" || "$value" == "null" ]] && return 0
  printf 'export %s=%q\n' "$key" "$value"
}

brand="$(prop ro.product.brand)"
manufacturer="$(prop ro.product.manufacturer)"
model="$(prop ro.product.model)"
android_release="$(prop ro.build.version.release)"
android_id="$(setting secure android_id)"

screen="$("${adb_args[@]}" shell wm size 2>/dev/null | tr -d '\r' | awk -F': ' '
  /Override size/ { value=$2 }
  /Physical size/ && value == "" { value=$2 }
  END { gsub(/[[:space:]]/, "", value); print value }
')"

wifi_dump="$("${adb_args[@]}" shell dumpsys wifi 2>/dev/null | tr -d '\r' || true)"
wifi_info="$(printf '%s\n' "$wifi_dump" | grep -m1 'mWifiInfo SSID:' || true)"
ssid="$(printf '%s\n' "$wifi_info" | sed -nE 's/.*mWifiInfo SSID: "?([^",]+)"?, BSSID:.*/\1/p' | first_line)"
bssid="$(printf '%s\n' "$wifi_info" | sed -nE 's/.*BSSID: ([0-9A-Fa-f:]{17}),.*/\1/p' | first_line | tr '[:lower:]' '[:upper:]')"
if [[ -z "$ssid" || "$ssid" == "<unknown ssid>" ]]; then
  ssid="$(printf '%s\n' "$wifi_dump" | sed -nE 's/.*ssid: "([^"]+)".*/\1/p' | first_line)"
fi
if [[ -z "$bssid" ]]; then
  bssid="$(printf '%s\n' "$wifi_dump" | sed -nE 's/.*bssid: ([0-9A-Fa-f:]{17}).*/\1/p' | first_line | tr '[:lower:]' '[:upper:]')"
fi
ip_address="$("${adb_args[@]}" shell ip -f inet addr show wlan0 2>/dev/null | tr -d '\r' | sed -nE 's/.*inet ([0-9.]+)\/.*/\1/p' | first_line)"

pkg_dump="$("${adb_args[@]}" shell dumpsys package "$pkg" 2>/dev/null | tr -d '\r' || true)"
version_name="$(printf '%s\n' "$pkg_dump" | sed -nE 's/.*versionName=([^[:space:]]+).*/\1/p' | first_line)"
version_code="$(printf '%s\n' "$pkg_dump" | sed -nE 's/.*versionCode=([0-9]+).*/\1/p' | first_line)"
installer="$(printf '%s\n' "$pkg_dump" | sed -nE 's/.*installerPackageName=([^[:space:]]+).*/\1/p' | first_line)"
if printf '%s\n' "$pkg_dump" | grep -q 'android.permission.ACCESS_FINE_LOCATION: granted=true\|android.permission.ACCESS_COARSE_LOCATION: granted=true'; then
  location="${GOPAY_LOCATION:-}"
  location_accuracy="${GOPAY_LOCATION_ACCURACY:-}"
else
  location="NA"
  location_accuracy="NA"
fi

gms_dump="$("${adb_args[@]}" shell dumpsys package com.google.android.gms 2>/dev/null | tr -d '\r' || true)"
gms_version="$(printf '%s\n' "$gms_dump" | sed -nE 's/.*versionCode=([0-9]+).*/\1/p' | first_line)"

cat <<'EOF'
# Source this output into the GoPay app-service environment.
# X-DeviceToken/FCM is app-private; the service generates a plausible token when GOPAY_DEVICE_TOKEN is omitted.
EOF
emit GOPAY_PHONE_MAKE "$manufacturer"
if [[ -n "$brand" && -n "$model" ]]; then
  emit GOPAY_PHONE_MODEL "${brand},${model}"
fi
emit GOPAY_ANDROID_VERSION "$android_release"
emit GOPAY_SCREEN "$screen"
emit GOPAY_UNIQUE_ID "$android_id"
emit GOPAY_IMEI "$android_id"
emit GOPAY_IP_ADDRESS "$ip_address"
emit GOPAY_WIFI_MAC "$bssid"
emit GOPAY_WIFI_SSID "$ssid"
emit GOPAY_APP_VERSION "$version_name"
emit GOPAY_APP_BUILD "$version_code"
emit GOPAY_INSTALLER_PACKAGE "$installer"
emit GOPAY_GMS_VERSION "$gms_version"
emit GOPAY_LOCATION "$location"
emit GOPAY_LOCATION_ACCURACY "$location_accuracy"
