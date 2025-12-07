#!/usr/bin/env bash
set -euo pipefail
# Mock CI check: fails if PT_STATUS_TO is not "closed" or if PT_ID contains "fail".
if [[ "${PT_STATUS_TO:-}" != "closed" ]]; then
  echo "CI check expects closing transition"
  exit 1
fi
if echo "${PT_ID:-}" | grep -q "fail"; then
  echo "CI reported failure for ${PT_ID}"
  exit 1
fi
