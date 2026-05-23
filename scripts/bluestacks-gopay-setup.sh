#!/usr/bin/env bash
set -euo pipefail

PACKAGE_NAME=${PACKAGE_NAME:-com.gojek.gopay}
DOWNLOAD_DIR=${DOWNLOAD_DIR:-"$HOME/Downloads"}
PROXY_HOST=${PROXY_HOST:-10.0.2.2}
PROXY_PORT=${PROXY_PORT:-10809}
ADB_SERIAL=${ADB_SERIAL:-}
APK_PATH=${APK_PATH:-}
KEEP_TMP=${KEEP_TMP:-false}

log() {
  printf '[bluestacks-gopay] %s\n' "$*"
}

die() {
  printf '[bluestacks-gopay] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  scripts/bluestacks-gopay-setup.sh [options]

Options:
  --serial SERIAL       ADB serial. Default: auto-detect BlueStacks, then 127.0.0.1:5565.
  --proxy HOST:PORT     Android system HTTP proxy. Default: 10.0.2.2:10809.
  --apk PATH            Specific .apk/.apkm/.xapk/.apks package to install.
  --downloads DIR       Directory to scan for latest local GoPay package. Default: ~/Downloads.
  --no-install          Only configure and verify proxy.
  -h, --help            Show help.

Environment:
  ADB_SERIAL, PROXY_HOST, PROXY_PORT, APK_PATH, DOWNLOAD_DIR, KEEP_TMP=true.
EOF
}

NO_INSTALL=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --serial)
      [[ $# -ge 2 ]] || die "--serial requires a value"
      ADB_SERIAL=$2
      shift 2
      ;;
    --proxy)
      [[ $# -ge 2 ]] || die "--proxy requires HOST:PORT"
      PROXY_HOST=${2%:*}
      PROXY_PORT=${2##*:}
      [[ -n "$PROXY_HOST" && -n "$PROXY_PORT" && "$PROXY_HOST" != "$PROXY_PORT" ]] || die "invalid --proxy: $2"
      shift 2
      ;;
    --apk)
      [[ $# -ge 2 ]] || die "--apk requires a path"
      APK_PATH=$2
      shift 2
      ;;
    --downloads)
      [[ $# -ge 2 ]] || die "--downloads requires a path"
      DOWNLOAD_DIR=$2
      shift 2
      ;;
    --no-install)
      NO_INSTALL=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

need adb
need python3
need unzip

detect_bluestacks_serial() {
  local conf="/Users/Shared/Library/Application Support/BlueStacks/bluestacks.conf"
  local port=""
  if [[ -f "$conf" ]]; then
    port=$(awk -F'"' '/bst\.instance\..*\.status\.adb_port=/ { value=$2 } END { print value }' "$conf")
  fi
  if [[ -z "$port" ]]; then
    port=$(lsof -nP -iTCP -sTCP:LISTEN 2>/dev/null | awk '/BlueStack/ && $9 ~ /:[0-9]+$/ { sub(/^.*:/, "", $9); print $9; exit }')
  fi
  printf '127.0.0.1:%s' "${port:-5565}"
}

if [[ -z "$ADB_SERIAL" ]]; then
  ADB_SERIAL=$(detect_bluestacks_serial)
fi

adb_for_device() {
  adb -s "$ADB_SERIAL" "$@"
}

connect_adb() {
  if [[ "$ADB_SERIAL" == 127.0.0.1:* || "$ADB_SERIAL" == localhost:* ]]; then
    adb connect "$ADB_SERIAL" >/dev/null || true
  fi
  adb_for_device get-state >/dev/null 2>&1 || die "ADB device not available: $ADB_SERIAL"
  log "ADB device: $ADB_SERIAL"
}

configure_proxy() {
  log "set Android HTTP proxy: $PROXY_HOST:$PROXY_PORT"
  adb_for_device shell settings put global http_proxy "$PROXY_HOST:$PROXY_PORT"
  adb_for_device shell settings put global global_http_proxy_host "$PROXY_HOST"
  adb_for_device shell settings put global global_http_proxy_port "$PROXY_PORT"
  adb_for_device shell settings delete global http_proxy_pac >/dev/null 2>&1 || true
  adb_for_device shell settings put global captive_portal_mode 0
  adb_for_device shell settings put global network_avoid_bad_wifi 0
  adb_for_device reverse "tcp:$PROXY_PORT" "tcp:$PROXY_PORT" >/dev/null 2>&1 || true
}

verify_proxy() {
  log "verify proxy TCP"
  adb_for_device shell "echo | toybox nc -w 3 -q 1 '$PROXY_HOST' '$PROXY_PORT' >/dev/null && echo proxy_tcp_ok || echo proxy_tcp_fail" |
    grep -q proxy_tcp_ok || die "proxy TCP check failed: $PROXY_HOST:$PROXY_PORT"
  log "verify proxy HTTP"
  local status
  status=$(adb_for_device shell "printf 'GET http://connectivitycheck.gstatic.com/generate_204 HTTP/1.1\r\nHost: connectivitycheck.gstatic.com\r\nConnection: close\r\n\r\n' | toybox nc -w 6 -q 1 '$PROXY_HOST' '$PROXY_PORT' | head -1" | tr -d '\r')
  [[ "$status" == HTTP/* ]] || die "proxy HTTP check failed"
  log "$status"
}

find_latest_gopay_package() {
  python3 - "$DOWNLOAD_DIR" <<'PY'
import re
import sys
import zipfile
from pathlib import Path

root = Path(sys.argv[1]).expanduser()
patterns = ("*.apk", "*.apkm", "*.xapk", "*.apks")
items = []
for pattern in patterns:
    for path in root.glob(pattern):
        name = path.name.lower()
        if "gopay" not in name and "com.gojek.gopay" not in name:
            continue
        versions = re.findall(r"(?<!\d)(\d+(?:\.\d+){1,3})(?!\d)", path.name)
        if not versions:
            continue
        version = max(versions, key=lambda v: tuple(int(x) for x in v.split(".")))
        bundle = path.suffix.lower() in {".apkm", ".xapk", ".apks"}
        if not bundle and path.suffix.lower() == ".apk":
            try:
                with zipfile.ZipFile(path) as zf:
                    bundle = "base.apk" in zf.namelist()
            except Exception:
                bundle = False
        items.append((tuple(int(x) for x in version.split(".")), int(bundle), path.stat().st_mtime, str(path)))
if not items:
    sys.exit(1)
items.sort()
print(items[-1][3])
PY
}

abi_split_name() {
  local abilist=$1
  case ",$abilist," in
    *,arm64-v8a,*) printf 'split_config.arm64_v8a.apk' ;;
    *,armeabi-v7a,*) printf 'split_config.armeabi_v7a.apk' ;;
    *,x86_64,*) printf 'split_config.x86_64.apk' ;;
    *,x86,*) printf 'split_config.x86.apk' ;;
    *) printf '' ;;
  esac
}

density_split_name() {
  local density=$1
  if [[ "$density" -lt 160 ]]; then
    printf 'split_config.ldpi.apk'
  elif [[ "$density" -lt 213 ]]; then
    printf 'split_config.mdpi.apk'
  elif [[ "$density" -lt 320 ]]; then
    printf 'split_config.hdpi.apk'
  elif [[ "$density" -lt 480 ]]; then
    printf 'split_config.xhdpi.apk'
  elif [[ "$density" -lt 640 ]]; then
    printf 'split_config.xxhdpi.apk'
  else
    printf 'split_config.xxxhdpi.apk'
  fi
}

install_plain_apk() {
  local apk=$1
  log "install APK: $apk"
  adb_for_device install -r "$apk"
}

install_bundle() {
  local bundle=$1
  local tmp
  tmp=$(mktemp -d /tmp/gopay-bluestacks.XXXXXX)
  if [[ "$KEEP_TMP" == true ]]; then
    log "keeping temp dir: $tmp"
  fi

  log "extract bundle: $bundle"
  unzip -q "$bundle" -d "$tmp"
  [[ -f "$tmp/base.apk" ]] || die "bundle has no base.apk: $bundle"

  local abilist density abi_split density_split
  abilist=$(adb_for_device shell getprop ro.product.cpu.abilist | tr -d '\r')
  density=$(adb_for_device shell wm density | awk '{ print $NF }' | tr -d '\r')
  [[ "$density" =~ ^[0-9]+$ ]] || density=240
  abi_split=$(abi_split_name "$abilist")
  density_split=$(density_split_name "$density")

  local apks=("$tmp/base.apk")
  [[ -n "$abi_split" && -f "$tmp/$abi_split" ]] && apks+=("$tmp/$abi_split")
  [[ -n "$density_split" && -f "$tmp/$density_split" ]] && apks+=("$tmp/$density_split")
  [[ -f "$tmp/split_config.in.apk" ]] && apks+=("$tmp/split_config.in.apk")
  [[ -f "$tmp/split_config.en.apk" ]] && apks+=("$tmp/split_config.en.apk")

  log "install bundle splits: ${apks[*]##*/}"
  adb_for_device install-multiple -r "${apks[@]}"
  if [[ "$KEEP_TMP" != true ]]; then
    rm -rf "$tmp"
  fi
}

install_gopay() {
  local package_path=$APK_PATH
  if [[ -z "$package_path" ]]; then
    package_path=$(find_latest_gopay_package) || die "no local GoPay .apk/.apkm/.xapk/.apks found in $DOWNLOAD_DIR"
  fi
  [[ -f "$package_path" ]] || die "package not found: $package_path"

  local package_lower
  package_lower=$(printf '%s' "$package_path" | tr '[:upper:]' '[:lower:]')
  case "$package_lower" in
    *.apkm|*.xapk|*.apks)
      install_bundle "$package_path"
      ;;
    *.apk)
      if unzip -l "$package_path" 'base.apk' >/dev/null 2>&1; then
        install_bundle "$package_path"
      else
        install_plain_apk "$package_path"
      fi
      ;;
    *)
      die "unsupported package type: $package_path"
      ;;
  esac
}

launch_and_report() {
  log "launch GoPay"
  adb_for_device shell am force-stop "$PACKAGE_NAME" >/dev/null 2>&1 || true
  adb_for_device shell monkey -p "$PACKAGE_NAME" -c android.intent.category.LAUNCHER 1 >/dev/null || true
  local version
  version=$(adb_for_device shell dumpsys package "$PACKAGE_NAME" | grep -E 'versionCode=|versionName=' | head -2 | tr -d '\r' | paste -sd ' ' -)
  log "$version"
  log "current proxy: $(adb_for_device shell settings get global http_proxy | tr -d '\r')"
}

connect_adb
configure_proxy
verify_proxy
if [[ "$NO_INSTALL" != true ]]; then
  install_gopay
fi
launch_and_report
