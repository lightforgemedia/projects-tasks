#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")"/../../.. && pwd)"
PT_BIN="${ROOT}/pt"

OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/out"
rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

if [[ ! -x "${PT_BIN}" ]]; then
  echo "missing ${PT_BIN}; build first:" | tee "${OUT_DIR}/error.log"
  echo "  go build -o pt ./cmd/pt" | tee -a "${OUT_DIR}/error.log"
  exit 1
fi

DEMO_DIR="$(mktemp -d)"
trap 'rm -rf "${DEMO_DIR}"' EXIT

mkdir -p "${DEMO_DIR}/workflows" "${DEMO_DIR}/phases" "${DEMO_DIR}/.pt"

cat >"${DEMO_DIR}/workflows/a.toml" <<'EOF'
name = "wf-a"

[phase_assignment]
label_prefix = "phase:"
default_phase = "build"

[[phases]]
id = "build"
name = "Build"
order = 1
EOF

cat >"${DEMO_DIR}/workflows/b.toml" <<'EOF'
name = "wf-b"

[phase_assignment]
label_prefix = "phase:"
default_phase = "build"

[[phases]]
id = "build"
name = "Build"
order = 1
EOF

cat >"${DEMO_DIR}/phases/demo.toml" <<'EOF'
title = "Demo"

[[tasks]]
template = "backend_endpoint"
title = "Demo Task"
role = "dev"
artifact = "code:demo"
[tasks.dod]
tests = ["echo ok"]
manual = "demo"
criteria = ["ok"]
EOF

export PT_BACKEND=store
export PT_DB="${DEMO_DIR}/.pt/db.json"
export PT_SKIP_HOOKS=1

(
  cd "${DEMO_DIR}"
  "${PT_BIN}" sync "${DEMO_DIR}/phases/demo.toml"
) | tee "${OUT_DIR}/01-sync.txt"

(
  cd "${DEMO_DIR}"
  set +e
  "${PT_BIN}" next --json
  echo "exit=$?"
  set -e
) | tee "${OUT_DIR}/02-next-no-workflow.txt"

(
  cd "${DEMO_DIR}"
  "${PT_BIN}" next --workflow "${DEMO_DIR}/workflows/a.toml" --json
) | tee "${OUT_DIR}/03-next-with-workflow.txt"

(
  cd "${DEMO_DIR}"
  set +e
  "${PT_BIN}" claim pt-1 --as demo-agent
  echo "exit=$?"
  set -e
) | tee "${OUT_DIR}/04-claim-no-workflow.txt"

(
  cd "${DEMO_DIR}"
  "${PT_BIN}" claim pt-1 --as demo-agent --workflow "${DEMO_DIR}/workflows/a.toml"
) | tee "${OUT_DIR}/05-claim-with-workflow.txt"

(
  cd "${DEMO_DIR}"
  "${PT_BIN}" release pt-1 --as demo-agent
) | tee "${OUT_DIR}/06-release.txt"

(
  cd "${DEMO_DIR}"
  set +e
  "${PT_BIN}" ready --json
  echo "exit=$?"
  set -e
) | tee "${OUT_DIR}/07-ready-no-workflow.txt"

(
  cd "${DEMO_DIR}"
  "${PT_BIN}" ready --workflow "${DEMO_DIR}/workflows/a.toml" --json
) | tee "${OUT_DIR}/08-ready-with-workflow.txt"

echo "OK: wrote demo artifacts to ${OUT_DIR}"

