#!/usr/bin/env bash
set -Eeuo pipefail

pids=()

cleanup() {
  local status=$?
  if ((${#pids[@]} > 0)); then
    kill "${pids[@]}" >/dev/null 2>&1 || true
    wait "${pids[@]}" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

service_pg_dsn=${GPT_SERVICE_PG_DSN:-}
if [[ -z "$service_pg_dsn" ]]; then
  echo "GPT_SERVICE_PG_DSN is required" >&2
  exit 1
fi

account_addr=${GPT_ACCOUNT_INTERNAL_ADDR:-127.0.0.1:50052}
payment_addr=${GPT_GOPAY_PAYMENT_INTERNAL_ADDR:-127.0.0.1:50054}
gopay_app_port=${GPT_GOPAY_APP_INTERNAL_PORT:-50060}
gopay_app_addr=${GPT_GOPAY_APP_INTERNAL_ADDR:-127.0.0.1:${gopay_app_port}}
payment_config=${GOPAY_PAYMENT_CONFIG:-/app/gopay-flow/config.json}
if [[ ! -f "$payment_config" ]]; then
  mkdir -p "$(dirname "$payment_config")"
  GOPAY_PAYMENT_CONFIG_PATH="$payment_config" python - <<'PY'
import json
import os

config = {
    "auth": {"session_token": os.environ.get("GOPAY_PAYMENT_SESSION_TOKEN", "")},
    "gopay": {
        "country_code": os.environ.get("GOPAY_COUNTRY_CODE", "62"),
        "phone_number": os.environ.get("GOPAY_PHONE_NUMBER", ""),
        "pin": os.environ.get("GOPAY_PIN", ""),
        "tokenization": os.environ.get("GOPAY_TOKENIZATION", "true"),
        "browser_locale": os.environ.get("GOPAY_BROWSER_LOCALE", "zh-CN"),
        "pin_locale": os.environ.get("GOPAY_PIN_LOCALE", "id"),
    },
    "proxies": {
        "checkout": os.environ.get("GOPAY_CHECKOUT_PROXY_URL", ""),
        "payment": os.environ.get("GOPAY_PAYMENT_PROXY_URL", ""),
    },
}
with open(os.environ["GOPAY_PAYMENT_CONFIG_PATH"], "w", encoding="utf-8") as handle:
    json.dump(config, handle, ensure_ascii=False)
PY
fi

(
  export LISTEN_ADDR="$account_addr"
  export PG_DSN="$service_pg_dsn"
  exec /app/bin/account-db
) &
pids+=("$!")

(
  export GOPAY_APP_PORT="$gopay_app_port"
  export PG_DSN="$service_pg_dsn"
  export GOPAY_APP_PG_DSN="${GOPAY_APP_PG_DSN:-$service_pg_dsn}"
  exec python /app/gopay-app/app_server.py
) &
pids+=("$!")

(
  exec python /app/gopay-flow/payment_server.py --config "$payment_config" --listen "$payment_addr"
) &
pids+=("$!")

export LISTEN_ADDR=${GPT_SERVICE_LISTEN_ADDR:-:50051}
export GPT_SERVICE_PG_DSN="$service_pg_dsn"
export GPT_ACCOUNT_ADDR="$account_addr"
export GPT_GOPAY_APP_ADDR="$gopay_app_addr"
export GPT_GOPAY_PAYMENT_ADDR="$payment_addr"
exec /app/bin/gpt-workflows
