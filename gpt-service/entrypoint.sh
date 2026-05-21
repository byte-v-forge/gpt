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
account_listen_addr=${GPT_ACCOUNT_LISTEN_ADDR:-:50052}
payment_addr=${GPT_GOPAY_PAYMENT_INTERNAL_ADDR:-127.0.0.1:50054}
payment_listen_addr=${GPT_GOPAY_PAYMENT_LISTEN_ADDR:-:50054}
gopay_app_port=${GPT_GOPAY_APP_INTERNAL_PORT:-50060}
gopay_app_addr=${GPT_GOPAY_APP_INTERNAL_ADDR:-127.0.0.1:${gopay_app_port}}

(
  export LISTEN_ADDR="$account_listen_addr"
  export PG_DSN="$service_pg_dsn"
  exec /app/bin/account-db
) &
pids+=("$!")

(
  export GOPAY_APP_PORT="$gopay_app_port"
  export PG_DSN="$service_pg_dsn"
  export GOPAY_APP_PG_DSN="${GOPAY_APP_PG_DSN:-$service_pg_dsn}"
  exec /app/bin/gopay-app
) &
pids+=("$!")

(
  export GOPAY_PAYMENT_LISTEN_ADDR="$payment_listen_addr"
  exec /app/bin/gopay-payment --listen "$payment_listen_addr"
) &
pids+=("$!")

export LISTEN_ADDR=${GPT_SERVICE_LISTEN_ADDR:-:50051}
export GPT_SERVICE_PG_DSN="$service_pg_dsn"
export GPT_ACCOUNT_ADDR="$account_addr"
export GPT_GOPAY_APP_ADDR="$gopay_app_addr"
export GPT_GOPAY_PAYMENT_ADDR="$payment_addr"
exec /app/bin/gpt-workflows
