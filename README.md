# projects-tasks — Project & Task Management (store-backed)

This repository ships a **Go SDK** and **CLI** with a built-in store to provide structured project/phase planning, templated tasks, and ready-to-work queues for human or agent contributors.

**Goal:** Provide structured project/task planning (manifests, templates, validation, state machine) on a simple built-in store.

## System Shape

- **Data Backbone:** Built-in JSON store (or pluggable backend) for issues/deps/status/labels.
- **Go SDK (`pkg/pt`):** Reusable library exposing plan/sync/ready/validate primitives. Any service or agent can embed this logic.
- **CLI (`cmd/pt`):** A thin wrapper around the SDK. Guides users through schema fields, enforces state machines (`planned` → `ready` → `in_progress` → `needs_review` → `done`), and handles user interaction.
- **Manifests:** TOML/JSON files for "Phase Bundles". These act as "Infrastructure as Code" for project management.
- **Context Contracts:** Strict schemas (`contracts/*.toml`) that validate agent inputs (files, goals, criteria) to prevent drift and hallucination.
- **Validation:** "Definition of Done" (DoD) per template, plus ready-work queries that only surface unblocked tasks.

## Library & CLI Split

### SDK (`pkg/pt`)
Stateless logic package.
- `PlanPhase(manifest)`: Validates and parses a manifest into a graph structure.
- `Sync(manifest)`: Idempotently applies the plan to the store (creates/updates issues, sets deps).
- `Ready(filter)`: Returns tasks where `status != done` AND `blocking_deps == done`.
- `Validate(taskID)`: Runs defined hooks (tests, lint) and returns pass/fail.
- `Claim(taskID, owner)`, `Release(taskID)`, `Reject(taskID, reason)`.

### CLI (`cmd/pt`)
- `pt sync <manifest.toml>`: Applies a plan.
- `pt ready [--role=coder|architect]`: Lists available work.
- `pt claim <id>`: Marks as `in_progress`.
- `pt validate <id>`: Runs hooks; if pass → `needs_review`.
- `pt approve/reject <id>`: Human review steps.

## Architecture

Store-backed flow:
- Manifest (TOML) → `pt sync` → issues/labels/deps persisted.
- `pt ready` filters unblocked tasks (deps closed, status open).
- `pt claim/validate/approve/reject` drive the state machine.
- Context builder/validator commands keep agent payloads in spec.

## Workflow & State Machine

The system enforces the following transitions:

1.  **Planned** → **Ready**: (Automatic) All dependencies are `done`.
2.  **Ready** → **In Progress**: (Manual) `pt claim`.
3.  **In Progress** → **Needs Review**: (Auto/Manual) `pt validate` passes (tests/hooks ok).
4.  **In Progress** → **In Progress**: (Auto) `pt validate` fails (fix and retry).
5.  **Needs Review** → **Done**: (Manual) `pt approve`.
6.  **Needs Review** → **In Progress**: (Manual) `pt reject "Fix X"`.

## Manifest Example

Manifests allow defining a phase of work in a single file.

```toml
# phases/login_flow.toml
title = "User Login Flow"
owner = "team-auth"

[[tasks]]
template = "backend_endpoint"
title = "Implement POST /login"
role = "backend-dev"
estimated_effort = "4h"
[tasks.params]
    path = "/login"
    method = "POST"
[tasks.dod]
    tests = ["go test ./src/auth/..."]
    manual = "Verify JWT is returned in header"

[[tasks]]
template = "frontend_component"
title = "Login Form UI"
role = "frontend-dev"
deps = ["Implement POST /login"] # Dependency by title
[tasks.params]
    component = "LoginForm"
[tasks.dod]
    validation_cmd = "bun test src/components/LoginForm.test.ts"
```

## CLI Usage

The `pt` CLI manages the lifecycle of tasks defined in your manifests.

### Quick Flow (bd-style helper)
1) `pt sync phases/<file>.toml` — apply manifest to the store.
2) `pt ready --role=<role> --verbose` — find unblocked work.
3) `pt claim <id> [--as=you]` — assign and start.
4) Do the work, then `pt validate <id> [--yes]` — moves to `needs_review` with manual notes.
5) `pt approve <id>` or `pt reject <id> --reason="..."` — review outcomes.
6) If stuck, `pt release <id>` — returns to open.
7) Add `--json` to commands to get machine-readable outputs (includes hook results).

### Inspect & Manage
- `pt list --status=in_progress,needs_review` — see WIP/review queues.
- `pt show <id> [--json]` — see title/status/DoD/comments.
- `pt add "Title" --role=... --template=... [--manual|--tests|--validation-cmd]` — ad-hoc task.
- `pt comment <id> "text"` — append notes.
- `pt snapshot` — copy `.pt.db.json` to a timestamped backup.
- Multi-project (read-only/plan): `pt multi-ready --dbs=a.json,b.json` to aggregate ready tasks; `pt propose <manifest> --db=...` to show adds/updates without writing.
- Search/read: `pt search --query="text"` across title/labels/description; `pt list`/`pt show` for status/DoD/comments; `--db`/`--prefix` to target specific stores.

## Multi-Agent & No-Context Onboarding
- Attribution: always `pt claim <id> --as=<identity>` so ownership is explicit; `pt release <id>` when you stop to unblock others.
- Collision avoidance: use `pt ready --verbose` to see blockers/assignees; do not bypass blocked tasks.
- Fresh context: `pt context init <id>` to pull role-specific inputs; read issue text + DoD when you arrive mid-stream.
- Staleness hygiene: add a comment when scope changes; re-validate before review after rebasing or major edits.
- Identity enforcement: `pt claim` requires a non-empty identity (set `$USER` or pass `--as`).

## Task Creation & Taxonomy
- Templates: `backend_endpoint` (APIs), `frontend_component` (UI), `bug_fix` (regressions), `refactor` (cleanup), `observability_hook` (SLO/alerts), `migration` (schema).
- Include: `title`, `role`, `template`, `deps`, optional `next_hint`, DoD (`tests`, `manual`). Keep titles unique; reference deps by title or ID.
- Example:
```toml
[[tasks]]
template = "bug_fix"
title = "Fix login redirect loop"
role = "backend-dev"
deps = ["Implement POST /login"]
next_hint = "Backend integration is next"
[tasks.dod]
tests = ["go test ./services/auth/..."]
manual = "Verify browser redirect to /dashboard after login"
```
- Post-task cleanup: status/labels should match reality (`pt validate`, then `pt approve`/`pt reject`; `pt release` if abandoning). Keep comments concise and actionable.

### 1. Plan & Sync
Apply a manifest to the store. This creates issues, sets dependencies, and stores validation rules.
```bash
pt sync phases/login_flow.toml
# Output:
# Implement POST /login -> proj-12
# Login Form UI -> proj-13
```

### 2. Find Work
List tasks that are unblocked (dependencies done) and available (status=open).
```bash
pt ready --role=backend-dev
# Output:
# proj-12 [task] Implement POST /login
# Flags: --role, --limit, --sort=priority|title, --verbose (shows assignee + blockers with dep IDs)
```

### 3. Claim & Execute
Mark a task as `in_progress` and assign it to yourself.
```bash
pt claim proj-12           # uses $USER
pt claim proj-12 --as bob  # override assignee
# Output: Claimed proj-12 as <user>
```

### 4. Validate
Run the tests and checks defined in the manifest. If successful, the task moves to `needs_review`. Use `--yes` to auto-confirm manual steps (the confirmed steps are echoed in the review comment).
```bash
pt validate proj-12 --yes
# ... running validation_cmd ...
# Task proj-12 marked needs_review (comment includes manual steps)
```

### 5. Review
Reviewers can approve or reject the work.
```bash
# Approve (closes the issue)
pt approve proj-12

# Reject (sends back to in_progress with a comment)
pt reject proj-12 --reason="Tests are flaky"
```

### 6. Agent Context (Advanced)
Agents use contracts to ensure they have all required inputs before working.

```bash
# 1. Generate context skeleton from task details
pt context init --role=builder proj-12 > context.json

# 2. Validate context against the contract
pt context validate --contract=contracts/builder.toml context.json
# Output: Context is valid.
```

- More examples live in `phases/login_flow.toml`, `phases/hotfix_bug.toml`, and `examples/dogfood/manifest.toml` (multi-agent demo).

## Automation Hooks (planned)
- Events: `pre|post-sync`, `pre|post-claim`, `post-release`, `pre|post-validate`, `post-approve`, `post-reject`.
- Config: `hooks.toml` (repo) or `$HOME/.config/pt/hooks.toml` (global). Override with `PT_HOOKS=path`. See `examples/hooks.toml` and `DESIGN_HOOKS.md` for schema. Fields: `event`, `cmd`, optional `on_fail`, `timeout`.
- Env payload to hooks: `PT_EVENT`, `PT_ID`, `PT_TITLE`, `PT_ASSIGNEE`, `PT_ACTOR`, `PT_STATUS_FROM`, `PT_STATUS_TO`, `PT_ROLE`, `PT_DOD` (JSON).
- Full JSON payload is provided via stdin (prefer stdin for large payloads); enable hook logging with `PT_HOOK_VERBOSE=1`.
- Semantics: pre-hooks can block on failure; post-hooks warn by default. Run serially, log command + exit when `--hook-verbose` (flag), `defaults.verbose=true` in hooks, or `PT_HOOK_VERBOSE=1`. Hook outputs are surfaced as `[hook ok]...` and included in `--json` outputs.
- Until implemented, wrap `pt` commands in your own scripts for notifications, policy, or deployment triggers.
- Bypass: set `PT_SKIP_HOOKS=1` to disable hook execution temporarily.
- Security note: hooks are arbitrary shell; review them before use. Prefer stdin payload for large data and set explicit timeouts/on_fail.

## Persistence & Multi-Project Use
- Store: `.pt.db.json` in the repo by default (override with `PT_DB`). File locking is used for same-host safety.
- Snapshots: commit the store to git if you want history, or add a post-approve hook to copy `.pt.db.json` to `snapshots/pt-$(date).json`.
- Multiple projects: use one repo per project, or separate directories with different `PT_DB` paths/prefixes. For many concurrent projects, keep each in its own working dir to avoid cross-contamination.
- Cross-project design: see `DESIGN_MULTI_PROJECT.md` for proposed `--db`/`--prefix` overrides, `pt propose`, and `pt multi-ready`.

## Task Templates & Schema

The SDK supports various templates to standardize work:

- **`backend_endpoint`**: Standard API work.
- **`frontend_component`**: UI components.
- **`migration`**: DB schema changes.
- **`bug_fix`**: Requires "Reproduction Steps" in DoD.
- **`refactor`**: Requires "Existing Tests Pass" + "No Regression".
- **`observability_hook`**: Requires metrics/alerts definition.

**Common Schema Fields:**
- `title` (required)
- `role` (e.g., "backend-dev", "architect")
- `deps` (list of parent task titles or IDs)
- `validation_cmd` (shell command for `pt validate`)
- `manual_check_text` (prompt for human/agent verification)
- `on_failure` (`block`, `flag`)

## Use Cases

- **Seed a Feature:** Define 10 interconnected tasks in TOML, sync once, and have the team swarm on "Ready" tasks.
- **Agent-Driven Loop:**
    1. Agent polls `pt ready --role=coder-bot`.
    2. Claims task, implements, runs `pt validate`.
    3. On success, moves to `needs_review`.
    4. On failure, reads output, iterates (self-corrects), retries.
- **Emergency Hotfix:** Add a `bug_fix` task to a "Hotfix" manifest. `pt sync` creates it; `pt ready` prioritizes it (if dependencies allow).
- **Cross-Team Handoff:** Frontend tasks stay invisible in `pt ready` until backend dependencies are marked `done`.
- **Gated Reviews:** `pt validate` ensures tests pass *before* a human is asked to review, saving time.

## Test Cases

| Scenario | Steps | Expected |
| :--- | :--- | :--- |
| **Sync Malformed Manifest** | `pt sync bad.toml` | Fails gracefully; no partial issues created. |
| **Sync Idempotency** | Run `pt sync` twice | No duplicate issues; updates fields if changed. |
| **Dependency Gating** | A->B. Mark A `done` | B appears in `pt ready`. B absent while A is `in_progress`. |
| **Validation Rejection** | `pt reject <id> "Fix imports"` | Task status `needs_review` → `in_progress`; comment recorded. |
| **Validation Failure** | Force test fail; `pt validate` | Task stays `in_progress`; error output surfaced. |
| **Concurrent Claim** | Two agents `pt claim <id>` | First succeeds; second gets "Already claimed" error. |
| **Manual Gate** | `pt validate` on task w/ manual check | CLI prompts "Did you check X?"; blocks until confirmed. |
| **Cross-Phase Deps** | Phase 2 depends on Phase 1 task | Phase 2 task only ready when Phase 1 task is done. |

## Notes for Contributors
- **Language:** Go (1.21+) for SDK/CLI.
- **Testing:** Always run `go test ./...` (no skips).
- **Style:** Follow standard Go idioms.
