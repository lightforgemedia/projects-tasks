# SDLC Flow with pt (Dogfooded)

This document captures a repeatable, evidence-first SDLC using the `pt` store-backed CLI. The goal is to guide agents and humans through a risk-first, output-driven workflow while keeping `pt` generic and hook-extensible.

## Why This Works
- **Risk-first:** External dependencies are validated before coding, reducing rework.
- **Outputs-first:** Every task requires user-visible artifacts (screenshots, sample payloads, test logs).
- **Guided Next Steps:** Tasks carry `next_hint` (planned) and hooks echo the next action, keeping agents on rails.
- **Hooks-as-rails:** Pre/post hooks enforce gates (fixtures, tests) and broadcast outcomes without baking rules into the core SDK.
- **Dogfooding:** The flow is encoded as a manifest and run with `pt sync/ready/claim/validate/approve`.

## Example SDLC Flow (Tasks)
1) **External Dependencies (highest risk)**
   - Validate access to external services/APIs.
   - Capture real responses and cache fixtures.
   - DoD: fixture exists, schema documented.
2) **Frontend (mocked data)**
   - Build UI against cached fixtures.
   - DoD: screenshot(s), component tests.
3) **Backend: External Wiring**
   - Integrate external services first using fixtures as contracts.
   - DoD: go test ./..., integration smoke with fixtures.
4) **Backend: Internal Data**
   - Replace mocks with real internal data sources; keep mocks as test fallbacks.
   - DoD: schema/docs updated; tests green.
5) **Full Flow Demo & Review**
   - Run end-to-end, capture screenshots/video, provide sample outputs.
   - DoD: all tests pass; reviewer-approved artifacts.

## How `pt` Drives the Flow
- **Manifests:** Encode the flow as tasks with deps and roles (see `phases/sdlc_dogfood.toml`).
- **Labels/next hints (planned):** Add `next_hint` in description or label `next:<task>` to suggest what to do after completion.
- **Hooks:** Pre/post hooks enforce gates and emit guidance:
  - `pre-claim`: WIP/policy + fixture check for external deps.
  - `pre-validate`: lint/tests; fail if artifacts missing.
  - `post-validate`: echo artifacts collected; remind to run `pt ready`.
  - `pre-approve`: optional CI check.
  - `post-approve`: suggest next task (from `next_hint`).
- **Artifacts in DoD:** Manual text should specify required outputs (screenshots, payloads, URLs). `pt validate --yes` records confirmations in comments.

## Dogfood Checklist
- Use `pt` for this work:
  - `pt sync phases/sdlc_dogfood.toml`
  - `pt ready --verbose`
  - `pt claim --as <you> <id>`
  - `pt validate --yes <id>` (hooks enforce gates)
  - `pt approve <id>` (hooks notify)
- Keep `hooks.toml` (repo) + optional global hooks for team-wide rails.
