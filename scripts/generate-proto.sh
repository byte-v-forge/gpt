#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

gen_go() {
  local service="$1"
  shift
  mkdir -p "${ROOT}/${service}/pb"
  rm -f "${ROOT}/${service}/pb"/*.pb.go "${ROOT}/${service}/pb"/*_grpc.pb.go
  protoc -I "${ROOT}/proto" \
    --go_out="${ROOT}/${service}/pb" \
    --go-grpc_out="${ROOT}/${service}/pb" \
    "$@"
}

gen_py() {
  local service="$1"
  shift
  python3 -m grpc_tools.protoc -I "${ROOT}/proto" \
    --python_out="${ROOT}/${service}" \
    --grpc_python_out="${ROOT}/${service}" \
    "$@"
}

root_proto() {
  printf '%s/proto/%s\n' "$ROOT" "$1"
}

orchestrator_protos=("${ROOT}"/proto/*.proto)

gen_go account-db "$(root_proto account_db.proto)"
gen_go orchestrator "${orchestrator_protos[@]}"

gen_py channels/gopay/app "$(root_proto gopay_app.proto)"
python3 -m grpc_tools.protoc \
  -I "${ROOT}/proto" \
  --python_out="${ROOT}/channels/gopay/payment/gopay-flow" \
  --grpc_python_out="${ROOT}/channels/gopay/payment/gopay-flow" \
  "$(root_proto payment.proto)"
