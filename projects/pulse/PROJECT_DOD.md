# Pulse — Project Definition of Done (MVP)

MVP is complete when:

- `go test ./...` passes from `projects/pulse/`.
- A local demo page + at least **3 micro-flows** run reliably:
  - “P0 smoke” flow(s) always run.
  - Impact filtering runs the correct tagged flows for a sample `git diff`.
- `pulse run` prints a clear per-step report (pass/fail + reason) and produces a `Snapshot` artifact on failure.
- Fingerprints exist and drift policy is enforced:
  - Structural drift hard-fails with actionable output.
  - Cosmetic drift is tolerated within configured bounds.
- Self-healing fallback works end-to-end for one drift scenario:
  - Runner detects fingerprint mismatch.
  - Runner emits a suggested fingerprint update file (`*.fp.toml`) for review.

## How to validate (commands + expected artifacts)

All commands below are safe to run from the repo root.

### 1) Unit tests (fast)
```bash
cd projects/pulse
go test ./...
```

### 2) Dry-run selection (impact mapping + P0 inclusion)
```bash
go run ./projects/pulse/cmd/pulse --dry-run --diff=HEAD~8..HEAD
```
Evidence:
- `projects/pulse/outputs/runs/dry-run.latest.txt`

### 3) Demo server (manual page verification)
```bash
go run ./projects/pulse/cmd/pulse --demo --addr 127.0.0.1:8085
```
Then open:
- `http://127.0.0.1:8085/products?query=socks`
- `http://127.0.0.1:8085/login`
- `http://127.0.0.1:8085/settings/profile`

Evidence:
- `projects/pulse/outputs/runs/demo-server.smoke.txt` (curl/grep proof of stable elements)

### 4) Run one passing flow (real browser)
```bash
# In one terminal
go run ./projects/pulse/cmd/pulse --demo --addr 127.0.0.1:8085

# In another
go run ./projects/pulse/cmd/pulse --run \
  --flow=projects/pulse/testdata/flows/valid/product_card_quickadd.toml \
  --base-url=http://127.0.0.1:8085 \
  --headless=true
```
Evidence:
- `projects/pulse/outputs/runs/run-one.latest.txt`

### 5) Run one intentional failure (snapshot + DOM)
Expected behavior: non-zero exit, with `snapshot:` and `dom:` paths printed and a `report.json` written.

Evidence:
- `projects/pulse/outputs/runs/run-fail.latest.txt`
- Example artifact set (from dogfooding):
  - `projects/pulse/outputs/runs/2025-12-16T05-47-02Z/report.json`
  - `projects/pulse/outputs/runs/2025-12-16T05-47-02Z/flows/product_card_intentional_fail/snapshot.png`
  - `projects/pulse/outputs/runs/2025-12-16T05-47-02Z/flows/product_card_intentional_fail/dom.html`

### 6) Integration E2E (3 flows headless)
```bash
cd projects/pulse
go test -tags=integration ./... -v
```
Evidence:
- `projects/pulse/outputs/runs/integration.latest.txt`

## Current status
- ✅ Unit tests, demo server, `pulse --dry-run`, `pulse --run`, failure artifacts, and integration E2E are implemented.
- ✅ Drift policy + “suggested patch” emission is implemented:
  - Cosmetic drift is tolerated (runner proceeds and emits a `patches/*.fp.toml` suggestion).
  - Structural drift hard-fails with actionable output and a suggested patch file.

Drift evidence:
- `projects/pulse/outputs/runs/drift.latest.txt`
- `projects/pulse/outputs/runs/drift.integration.latest.txt`
- Example patch artifacts:
  - `projects/pulse/outputs/runs/2025-12-16T06-01-09Z/patches/drift_cosmetic.step-0.fp.toml`
  - `projects/pulse/outputs/runs/2025-12-16T06-01-11Z/patches/drift_structural.step-0.fp.toml`
