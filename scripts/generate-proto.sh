#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_ROOT="${SOURCE_ROOT:-$(cd "${ROOT}/.." && pwd)}"
COMMON_PROTO_DIR="${COMMON_PROTO_DIR:-${SOURCE_ROOT}/common-lib/proto}"

if [[ ! -f "${COMMON_PROTO_DIR}/byte/v/forge/contracts/mailbox/v1/mailbox.proto" ]]; then
  printf 'required common proto not found under: %s\n' "${COMMON_PROTO_DIR}" >&2
  exit 1
fi

gen_go() {
  local service="$1"
  shift
  mkdir -p "${ROOT}/${service}/pb"
  rm -f "${ROOT}/${service}/pb"/*.pb.go "${ROOT}/${service}/pb"/*_grpc.pb.go
  protoc -I "${ROOT}/proto" -I "${COMMON_PROTO_DIR}" \
    --go_out="${ROOT}/${service}/pb" \
    --go-grpc_out="${ROOT}/${service}/pb" \
    "$@"
}

root_proto() {
  printf '%s/proto/%s\n' "$ROOT" "$1"
}

orchestrator_protos=("${ROOT}"/proto/*.proto)

gen_go gpt-account "$(root_proto gpt_account.proto)"
gen_go orchestrator "${orchestrator_protos[@]}"
