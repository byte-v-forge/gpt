#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR/gopay-flow"

exec "$ROOT_DIR/.venv/bin/python" gopay.py --config config.json "$@"
