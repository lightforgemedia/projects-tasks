# PT Conductor: `pt next` (Stability + SDLC Guidance)

**Date:** 2025-12-15  
**Problem:** Even with workflow guidance, agents can still “skip ahead” by picking attractive tasks or treating gates as ignorable. We want a *stable control loop* where PT is authoritative about the next safe action and why.

## Goals

1. Provide a single, deterministic entry point for orchestration: `pt next`.
2. Reduce skipping ahead by defaulting to the current (earliest unfinished) phase.
3. Avoid blocking work that doesn’t require end-user input:
   - Prefer internal review (needs_review) before demanding end-user checkpoints.
4. Provide machine-readable outputs for orchestration runtimes (`--json`).

## CLI Surface

```
pt next [--role=ROLE] [--strict] [--all-phases] [--json] [--db=PATH] [--prefix=...]
```

- `--role`: filters “work” recommendations to tasks with `role:<ROLE>`.
- `--strict`: treats soft gates as blocking unless overridden (see `pt claim --override-soft`).
- `--all-phases`: debug visibility; normal mode focuses on current phase.
- `--json`: structured output for agents.

## Modes (Deterministic)

`pt next` returns one of:

- **BLOCKED**: hard gate or strict soft gate prevents progress in the next phase.
- **REVIEW**: tasks exist in `needs_review` that should be approved/rejected before starting new work.
- **WORK**: a task is safe to claim now.
- **PLAN**: no actionable tasks, but project DoD is missing or not yet satisfied → create/sync tasks.
- **DONE**: no open/in_progress/needs_review tasks and project DoD exists.

## Decision Order

1. Load store + workflow (if present) + project DoD path/status.
2. If any tasks in `needs_review`:
   - Recommend review first (internal ownership) unless the only pending work is end-user checkpoint work.
3. Determine current phase (earliest phase with unfinished tasks).
4. If workflow gates block later phases:
   - In `--strict`, soft gate is treated like BLOCKED.
   - In default mode, soft gate becomes a warning (but `pt claim` still requires explicit override).
5. Recommend one next action:
   - Claim a current-phase open task (WORK), or
   - Provide unblock steps (BLOCKED), or
   - Provide planning steps (PLAN), or
   - Confirm completion (DONE).

## Output Contract

### Text output must include:
- Mode
- One primary command
- Why (1–3 short lines)
- Next step: “run `pt next` again”

### JSON output schema (stable keys)

```jsonc
{
  "mode": "WORK|REVIEW|BLOCKED|PLAN|DONE",
  "recommended": [
    {"cmd": "pt claim pt-12 --as=me", "kind": "work|review|unblock|plan"}
  ],
  "why": ["..."],
  "current_phase": {"id": "risk", "name": "Risk", "order": 1},
  "blocking": {
    "gate_type": "hard|soft",
    "blocking_phase": "risk",
    "message": "Complete risk tasks first",
    "unblock_steps": ["pt ready --phase=risk", "pt claim pt-1 --as=me"]
  },
  "approvals_needed": ["internal|end_user"],
  "ask_user": false,
  "ask_user_prompts": [],
  "project_dod": {"path": "PROJECT_DOD.md", "exists": true}
}
```

## Non-Goals (for this checkpoint)

- No new workflow file format.
- No auto-generation of tasks.
- No daemon / multi-user sync.

## Git Checkpoints

1. **Docs-only checkpoint** (this file): record intent + contract.
2. Implement `pt next` + tests + docs/help updates, run `go test ./...`, commit.
3. Improve task creation defaults (`pt add` / `pt sync` phase labeling + handoff placeholders), tests, commit.

