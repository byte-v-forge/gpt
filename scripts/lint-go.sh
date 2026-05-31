#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

modules=("gpt-account" "orchestrator" "pkg/gptplugin" "stripe")

unformatted="$(
  for module in "${modules[@]}"; do
    if [[ -d "${module}" ]]; then
      find "${module}" \
        \( -path '*/.git' -o -path '*/vendor' -o -path '*/node_modules' -o -path '*/dist' \) -prune \
        -o -name '*.go' -exec gofmt -l {} +
    fi
  done | sort -u
)"

if [[ -n "${unformatted}" ]]; then
  echo "gofmt required for:"
  echo "${unformatted}"
  exit 1
fi

for module in "${modules[@]}"; do
  if [[ -f "${module}/go.mod" ]]; then
    echo "go vet ${module}"
    (cd "${module}" && go vet ./...)
  fi
done
