#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_ROOT="${SOURCE_ROOT:-$(cd "${ROOT}/../.." && pwd)}"
GPT_PROTO_DIR="${GPT_PROTO_DIR:-${SOURCE_ROOT}/gpt/proto}"
COMMON_PROTO_DIR="${COMMON_PROTO_DIR:-${SOURCE_ROOT}/common-lib/proto}"
OUT_DIR="${OUT_DIR:-${ROOT}/src/proto}"
LOCAL_PLUGIN="${ROOT}/node_modules/.bin/protoc-gen-ts_proto"
AGGREGATE_PLUGIN="${SOURCE_ROOT}/webui/node_modules/.bin/protoc-gen-ts_proto"
PLUGIN="${PROTOC_GEN_TS_PROTO:-}"

if [[ -z "${PLUGIN}" ]]; then
  if [[ -x "${LOCAL_PLUGIN}" ]]; then
    PLUGIN="${LOCAL_PLUGIN}"
  elif [[ -x "${AGGREGATE_PLUGIN}" ]]; then
    PLUGIN="${AGGREGATE_PLUGIN}"
  fi
fi

if [[ -z "${PLUGIN}" || ! -x "${PLUGIN}" ]]; then
  printf 'ts-proto plugin not found; run npm install in webui first\n' >&2
  exit 1
fi
if [[ ! -d "${GPT_PROTO_DIR}" || ! -f "${COMMON_PROTO_DIR}/byte/v/forge/contracts/mailbox/v1/mailbox.proto" ]]; then
  printf 'required proto dirs not found: %s %s\n' "${GPT_PROTO_DIR}" "${COMMON_PROTO_DIR}" >&2
  exit 1
fi

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

ORCHESTRATOR_PROTOS=("${GPT_PROTO_DIR}"/orchestrator*.proto)
PROTO_INCLUDES=("-I" "${GPT_PROTO_DIR}" "-I" "${COMMON_PROTO_DIR}")
if [[ -d /usr/include/google/protobuf ]]; then
  PROTO_INCLUDES+=("-I" "/usr/include")
fi

protoc "${PROTO_INCLUDES[@]}" \
  --plugin="protoc-gen-ts_proto=${PLUGIN}" \
  --ts_proto_out="${OUT_DIR}" \
  --ts_proto_opt=onlyTypes=true,outputServices=none,esModuleInterop=true,useJsonWireFormat=true,snakeToCamel=false \
  --ts_proto_opt=Mbyte/v/forge/contracts/mailbox/v1/mailbox.proto=@byte-v-forge/common-ui/proto/byte/v/forge/contracts/mailbox/v1/mailbox \
  "${GPT_PROTO_DIR}/gpt_account.proto" \
  "${GPT_PROTO_DIR}/gopay_app.proto" \
  "${GPT_PROTO_DIR}/payment.proto" \
  "${ORCHESTRATOR_PROTOS[@]}"
