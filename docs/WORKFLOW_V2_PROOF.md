# Workflow V2 Proof: Enforcement + Conformance Checklist
**Date:** 2025-12-16

## Goal
Prove `PT_ENGINE=v2` is safe to make default by:
1) **Parity**: V2 decisions match V1 for existing workflows/templates.
2) **Single Source of Truth**: CLI enforcement and guidance both route through the V2 engine under flag.
3) **Extensibility**: New workflow templates work without code changes; invalid workflows fail with actionable errors.

This document inventories **where PT enforces process today** (V1), and what must be covered by **V2 conformance tests** before flipping defaults.

---

## Enforcement inventory (current behavior)

### `pt claim <id>` (primary gate enforcement)
Location: `cmd/pt/main.go:cmdClaim`

Decision points:
- **Identity**: `requireUser()` must succeed.
- **Workflow selection**: if a workflow file exists (`findWorkflowFile()`), gates are evaluated.
- **Gate evaluation**: `Workflow.CheckGate(...)` returns:
  - `canProceed` (bool)
  - `isHard` (bool)
  - `blockingPhase` (string)
  - `msg` (string)
- **Hard block**: error and non-zero exit.
- **Soft block**: requires `--override-soft=REASON` and writes an auditable `gate-override: <phase> <reason>` comment.
- **State transition**: `Transitioner.Claim(...)` moves `open -> in_progress`, assigns user.
- **Side effects**: hooks (`pre-claim`, `post-claim`), optional draft label.

What V2 must preserve:
- Exact hard/soft behavior (exit codes + messages).
- Same override comment format and timing (after claim, before returning).
- No gate evaluation when no workflow configured.

### `pt validate <id>` (DoD gate, not workflow gate)
Location: `cmd/pt/main.go:cmdValidate`

Decision points:
- Draft warning / optional prompt (skippable via `--yes`).
- Hook gating: `pre-validate` may block.
- DoD execution: `ValidationRunner.ValidateDoD(...)` must pass.
- State transition: `SubmitForReview(...)` moves to `needs_review`, writes comment summary.
- Post hooks: `post-validate`.

Workflow gate interaction today:
- **No workflow gates checked here** (only DoD + hooks).

What V2 must preserve:
- Same DoD execution semantics and output.
- Same review comment formatting.
- If V2 introduces workflow-gating here, it must be feature-flagged and conformance-tested.

### `pt approve <id>` (close)
Location: `cmd/pt/main.go:cmdApprove`

Decision points:
- Hook gating: `pre-approve` may block.
- State transition: `Transitioner.Approve(...)` closes task and cleans labels/comments as defined by transitioner.
- Post hooks: `post-approve`.

Workflow gate interaction today:
- **No workflow gates checked here**.

What V2 must preserve:
- Approval should not become “workflow gated” without an explicit workflow rule and conformance coverage.

### Other lifecycle commands (no workflow gating)
Locations: `cmd/pt/main.go`
- `pt release`: unassign/unblock a task (transitioner).
- `pt reject`: move back to `in_progress` with reason comment.
- `pt reopen`: reopen closed task to `in_progress`.
- `pt blocked` / `pt unblock`: manual block tracking.

What V2 must preserve:
- No unintended coupling to workflow selection (unless explicitly added to workflow rules + tests).

---

## V2 proof criteria (what must be true before flipping defaults)

### A) Conformance: V1 vs V2 parity
Add conformance scenarios that run the same command under:
- `PT_ENGINE=` (V1 default)
- `PT_ENGINE=v2`

And assert:
- Same exit code and error message class (hard vs soft vs ok).
- Same store mutations (status/labels/comments/assignee).
- Same JSON output shape for `--json` commands.

Minimum scenarios:
1) Claim allowed (no workflow, or gate passes).
2) Claim hard blocked (phase hard gate fails).
3) Claim soft blocked without override (must error).
4) Claim soft blocked with override (must succeed and add override comment).
5) Claim deps blocked (task deps not closed; should be unclaimable regardless of workflow gates).
6) Validate success: DoD tests run, task -> `needs_review`.
7) Approve success: task -> `closed`.

### B) Routing: engine is the authority under `PT_ENGINE=v2`
Under V2 flag:
- CLI must call engine for:
  - Phase selection / current phase
  - Gate evaluation for claim (and any future workflow-gated transitions)
- CLI must not duplicate decision logic in multiple places.

### C) Extensibility: workflows as data
Prove:
- All `workflows/*.toml` compile under V2 engine.
- At least one new workflow template compiles (no code change).
- Invalid gate expressions fail with actionable diagnostics:
  - include workflow file name, phase id, and the condition string.

---

## Implementation hooks (how to structure V2 enforcement)
Recommended V2 API shape (engine-only, no CLI parsing):
- `DecideClaim(taskID, issue, meta, allIssues, comments) -> {allowed, hard, phase, message, needsOverride}`
- `DecideValidate(...)` / `DecideApprove(...)` start as “always allowed” until workflow rules exist.

CLI responsibilities stay limited to:
- parse flags
- fetch state (store)
- call engine decision
- apply transition via Transitioner
- record override/comment side effects

