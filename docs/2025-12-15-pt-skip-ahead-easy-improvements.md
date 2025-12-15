# PT “Skip Ahead” Hardening — Easy Improvements

**Date:** 2025-12-15  
**Goal:** Reduce agents skipping ahead in the SDLC by making the *safe path* the *default path*.

## Problem

Even with workflow guidance (`pt workflow status/next/check`) and skill-style CLI messages, an agent can still:
- Claim tasks in later phases before earlier phases are complete.
- Treat “soft gates” as ignorable warnings with no audit trail.
- Use `pt ready` as a flat queue and pick attractive tasks instead of the correct phase-ordered work.

## Intended Behavior (Outcomes)

1. **Claim is the enforcement point**: `pt claim` must refuse to start work that violates workflow gates.
2. **Ready is phase-focused**: `pt ready` should default to the current (earliest unfinished) phase when a workflow exists.
3. **Soft gates require explicit override**: proceeding past a soft gate must be deliberate and recorded.
4. **Machine-readable guidance**: `--json` outputs include phase + gate info so orchestration agents can’t miss it.

## Plan (with Git Checkpoints)

Checkpoint A (this commit):
- Document the behavioral intent and acceptance criteria for the “easy improvements”.

Checkpoint B (implementation):
- Enforce workflow gates in `pt claim`:
  - Hard gate → block.
  - Soft gate → require `--override-soft="reason"`; record an override comment.
- Make `pt ready` workflow-aware:
  - Default to current phase; allow `--all-phases` / `--phase=<id>`.
- Standardize soft gate override semantics:
  - Comment format: `gate-override: <phase-id> <reason>`.
- Improve JSON outputs (`pt ready --json`, `pt claim --json`).
- Update docs (`README.md`, `docs/workflow-architecture.md`).
- Add tests proving enforcement + behavior.

## Acceptance Criteria

- With a workflow present:
  - `pt claim` blocks hard-gated tasks.
  - `pt claim` blocks soft-gated tasks unless `--override-soft` is provided.
  - Soft gate overrides are recorded as comments and are recognized by gate evaluation.
  - `pt ready` defaults to only the current phase (earliest unfinished) and expands only with `--all-phases` or `--phase`.
  - `pt ready --json` includes `phase` and `gate` info per task.
- `go test ./...` passes.

