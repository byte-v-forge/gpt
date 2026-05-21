#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_ROOT="${SOURCE_ROOT:-$(cd "${ROOT}/.." && pwd)}"
MAILBOX_PROTO_DIR="${MAILBOX_PROTO_DIR:-${SOURCE_ROOT}/mailbox/proto}"
MAILBOX_EMAIL_PROTO="${MAILBOX_PROTO_DIR}/email.proto"
MAILBOX_SERVICE_PROTO="${MAILBOX_PROTO_DIR}/mailbox_service.proto"

if [[ ! -f "${MAILBOX_EMAIL_PROTO}" || ! -f "${MAILBOX_SERVICE_PROTO}" ]]; then
  printf 'mailbox proto not found under: %s\n' "${MAILBOX_PROTO_DIR}" >&2
  exit 1
fi

gen_go() {
  local service="$1"
  shift
  mkdir -p "${ROOT}/${service}/pb"
  rm -f "${ROOT}/${service}/pb"/*.pb.go "${ROOT}/${service}/pb"/*_grpc.pb.go
  protoc -I "${ROOT}/proto" -I "${MAILBOX_PROTO_DIR}" \
    --go_out="${ROOT}/${service}/pb" \
    --go-grpc_out="${ROOT}/${service}/pb" \
    "$@"
}

root_proto() {
  printf '%s/proto/%s\n' "$ROOT" "$1"
}

orchestrator_protos=("${ROOT}"/proto/*.proto "${MAILBOX_EMAIL_PROTO}" "${MAILBOX_SERVICE_PROTO}")

gen_go account-db "$(root_proto account_db.proto)"
gen_go orchestrator "${orchestrator_protos[@]}"
gen_go gopay "$(root_proto gopay_app.proto)" "$(root_proto payment.proto)"
