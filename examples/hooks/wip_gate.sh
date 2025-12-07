#!/usr/bin/env bash
set -euo pipefail
# Blocks claim if user already has 2 in_progress tasks (per role filter, if provided).
USER_NAME="${PT_ASSIGNEE:-${PT_ACTOR:-}}"
if [ -z "$USER_NAME" ]; then
  echo "missing user identity"
  exit 1
fi

if command -v pt >/dev/null 2>&1; then
  count=$(pt ready --role="${PT_ROLE:-}" --verbose | grep -c "@${USER_NAME}")
else
  # No pt on PATH (e.g., minimal hook env); allow claim.
  count=0
fi

if [ "$count" -ge 2 ]; then
  echo "WIP limit reached for ${USER_NAME} (${count} tasks)"
  exit 1
fi
