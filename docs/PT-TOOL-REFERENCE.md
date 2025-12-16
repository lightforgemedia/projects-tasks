# PT Tool Reference

**Purpose**: Implementation details for the PT workflow tool. Will evolve.

See [WORKFLOW-PRINCIPLES.md](./WORKFLOW-PRINCIPLES.md) for the stable philosophy.

---

## Current State (as of 2025-12)

### What Works

- Core state machine: `open → in_progress → needs_review → done`
- Manifest sync with TOML `[[task]]` blocks
- Workflow conductor (`pt next`) and phase visualization (`pt workflow status`)
- Gate evaluation (soft/hard) with explicit soft overrides (`--override-soft=...`)
- Worktree integration for branch-per-task
- History tracking
- Manual blockers (`pt blocked` / `pt unblock`) respected by `pt next`
- Ad-hoc task edits (`pt update`) and recovery (`pt reopen`)
- Draft mode for exploratory work (`pt claim --draft`)

### Known Risks / Rough Edges

| Issue | Impact | Workaround |
|-------|--------|------------|
| Store is a single JSON file | Git merge conflicts + last-write-wins across branches | Use `pt snapshot` / `pt export` for shareable artifacts; avoid merging the store |
| Multiple workflow files | Workflow commands may require explicit selection | Set `PT_WORKFLOW=workflows/<name>.toml` or pass `--workflow=PATH` |

---

## CLI Quick Reference

Key commands (current behavior; see `pt --help` for the full list):

- `pt ready [--role=ROLE] [--phase=PHASE|--all-phases] [--verbose] [--include-blocked]`
  - Default: hides manually blocked tasks; add `--include-blocked` to see them (with reasons).
- `pt show <id> [--json] [--workflow=PATH]`
  - Shows phase when a workflow is selected (use `PT_WORKFLOW` or `--workflow`).
- `pt list [--status=...] [--role=ROLE] [--phase=PHASE] [--workflow=PATH] [--json] [--porcelain]`
  - `--phase` requires a workflow; use `--workflow` when multiple workflow files exist.
- `pt next [--workflow=PATH] [--all-phases] [--json]`
  - Conductor-style “what now?”; avoids recommending manually blocked tasks.
- `pt ready [--workflow=PATH] ...`
  - When multiple `workflows/*.toml` exist, select one via `--workflow` (or `PT_WORKFLOW`) for phase-aware output.
- `pt claim <id> [--workflow=PATH] ...`
  - When multiple `workflows/*.toml` exist, select one via `--workflow` (or `PT_WORKFLOW`) so gates are enforced deterministically.
- `pt review write <id> --kind=pre|post|demo [--phase=ID]`
  - Writes a markdown review under `.pt/reviews/` and links it to the task via a comment.
  - Pair with `pt sync --generate-phase-reviews` so phase kickoff/closeout/demo checkpoints exist and `pt next` can recommend them.
  - Demo conventions live in `docs/DEMO_DESIGN.md`.
- Context Contracts:
  - `pt context init <id> --role=builder > context.builder.json`
  - `pt context validate context.builder.json` (infers contract from payload role; override with `--contract=PATH`)

## Workflow Template: risk-first

Create `workflows/risk-first.toml`:

```toml
name = "risk-first"
description = "Prove unknowns → Explore UX → Build (hardest first) → Integrate → Signoff"

[phase_assignment]
default_phase = "build"
label_prefix = "phase:"

[phase_assignment.by_template]
spike = "prove"
discovery = "prove"
# explore tasks use discovery template + phase:explore label

[[phases]]
id = "prove"
name = "Prove Unknowns"
order = 1
description = "Validate external deps, algorithms, data shapes"

[phases.gate]
type = "soft"
condition = "all_closed"
reminder_before = "External dependencies not validated. Integration risk high."

[phases.proof]
required = true
description = "Captured real response or documented behavior"
hint = "Save to outputs/<task-id>/response.json"

[[phases]]
id = "explore"
name = "Explore UX"
order = 2
description = "Present 3-5 options, user picks direction"

[phases.gate]
type = "soft"
condition = "phase:prove complete"

[phases.checkpoint]
trigger = "all_tasks_complete"
prompt = "User must pick direction (add comment: user-picked:<option>)"

[[phases]]
id = "build"
name = "Build"
order = 3
description = "Implement hardest/vaguest first"

[phases.gate]
type = "hard"
condition = "phase:explore has_comment:user-picked"
block_message = "Build blocked. User must pick direction in explore phase."

[[phases]]
id = "integrate"
name = "Integrate"
order = 4
description = "Wire real systems, retire mocks"

[phases.gate]
type = "soft"
condition = "phase:build complete"

[[phases]]
id = "signoff"
name = "Signoff"
order = 5
description = "Full validation, user acceptance"

[phases.gate]
type = "hard"
condition = "phase:integrate complete"
block_message = "Signoff blocked. All integration tasks must be closed."
```

## Workflow Template: maintenance

Use `workflows/maintenance.toml` for bugfix/refactor work in an existing codebase:

```bash
export PT_WORKFLOW=workflows/maintenance.toml
pt workflow status
pt next
```

**Note:** If a phase has **zero tasks**, comment-based gates (e.g. `has_comment:user-picked`) are treated as satisfied to avoid deadlocking the workflow. If you want a gate to be enforced, ensure at least one task is assigned to that phase.

---

## Manifest Schema

### Task Templates

Currently allowed:
- `backend_endpoint`
- `frontend_component`
- `migration`
- `bug_fix`
- `refactor`
- `observability_hook`
- `discovery`
- `spike`

### Task Structure

```toml
[[tasks]]
template = "spike"
title = "Spike: Validate Tradier chain endpoint"
role = "backend-dev"
artifact = "proof:outputs/spike-chain/response.json"
deps = []

[tasks.dod]
tests = ["go test ./pkg/tradier/... -run TestChain"]
manual = "Hit real endpoint, capture response, document edge cases"
criteria = ["real response captured", "rate limits documented", "error cases noted"]
```

### Phase Assignment

Tasks get assigned to phases by:
1. `phase_assignment.by_template` mapping in workflow
2. `phase:<id>` label on task
3. `default_phase` fallback

### Spike-Specific Fields (Proposed)

```toml
[[tasks]]
template = "spike"
title = "Spike: External API behavior"
max_hours = 2  # Time-boxed
artifact = "proof:outputs/<id>/..."  # Required proof

[tasks.dod]
criteria = ["yes/no answer", "pivot/workaround/proceed decision documented"]
```

---

## Mock Tracking (Proposed)

### Commands

```bash
# Register mock with provenance
pt mock register \
  --source=spike-chain \
  --file=testdata/chain.json \
  --integration-task=pt-12 \
  --expires=2024-04-15

# List mocks
pt mock list
# chain.json  source:spike-chain  integration:pt-12  expires:2024-04-15

# Check for problems
pt mock check
# Error: chain.json expires in 5 days
# Error: old.json has no integration task
# Error: stale.json integration task pt-8 not in current/next phase

# Retire mock (fails if still imported outside tests)
pt mock retire testdata/chain.json
```

### Manifest Schema (Proposed)

```toml
[[tasks]]
template = "spike"
title = "Spike: Chain endpoint"

[tasks.mock]
file = "testdata/chain.json"
expires_at = "2024-04-15"
integration_task = "pt-12"  # Must be current/next phase
```

### Mock Visibility

CLI output when mocks active:
```
⚠️  MOCK MODE: chain data from spike-chain (expires 2024-04-15)
    Integration: pt-12 (in_progress)
```

Machine-readable (with `--json`):
```json
{
  "mock_mode": true,
  "mocks": [
    {
      "file": "testdata/chain.json",
      "source": "spike-chain",
      "integration_task": "pt-12",
      "expires_at": "2024-04-15"
    }
  ]
}
```

---

## Store Location

### Current

`.pt.db.json` in repo root by default (gitignored in this repo).

**Problems**: Merge conflicts if tracked, worktree copies can diverge across branches, and every command mutates the store.

### Recommended

```bash
# Project-local, gitignored
echo ".pt/" >> .gitignore
PT_DB=.pt/db.json pt sync phases/...

# Or user-local (multi-project)
PT_DB=~/.pt/$(basename $PWD)/db.json pt sync phases/...
```

No code changes needed—just set `PT_DB` and add to `.gitignore`.

---

## Integration Test Cadence

### Recommended CI Configuration

```yaml
# .github/workflows/test.yml
jobs:
  unit:
    # Every PR
    steps:
      - run: go test ./... -short

  integration:
    # Nightly only
    if: github.event_name == 'schedule'
    steps:
      - run: go test ./... -tags=integration

  release-gate:
    # Before merge to main
    if: github.ref == 'refs/heads/main'
    steps:
      - run: go test ./... -tags=integration
      - run: pt mock check --fail-on-expired
```

---

## Worktree Commands

```bash
# Create worktree for task
pt worktree start pt-5

# Check active worktrees
pt worktree status

# Clean up (after merge)
pt worktree done pt-5

# Abort (discard work)
pt worktree abort pt-5 --force
```

**Known issues**: See bugs table above. Run from repo root, use absolute `PT_DB`.

---

## Debugging

```bash
# Show task with full metadata
pt show pt-5

# Show workflow phase status
pt workflow status

# Check if task can proceed
pt workflow check --task=pt-5

# View task history
pt history pt-5
```

---

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `PT_DB` | Store location | `.pt.db.json` |
| `PT_WORKFLOW` | Workflow file | Auto-discover `workflows/*.toml` |
| `PT_PREFIX` | Issue ID prefix | `pt` |

---

*Tool reference. Will evolve with the codebase.*
