#!/usr/bin/env bash
set -euo pipefail

PT_BIN="${PT_BIN:-./pt}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "$ROOT"

echo "[integration] building pt"
go build -o pt ./cmd/pt

echo "[integration] running tests"
go test ./...

echo "[integration] syncing dogfood manifest"
"$PT_BIN" sync examples/dogfood/manifest.toml >/dev/null

echo "[integration] listing ready tasks (should succeed)"
"$PT_BIN" ready --all-phases --verbose >/dev/null

echo "[integration] ok"
