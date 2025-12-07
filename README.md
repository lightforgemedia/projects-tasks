# projects-tasks — Project & Task Management on Beads (`bd`)

This repository ships a **Go SDK** and **CLI** that layer on top of the Beads CLI (`bd`) to provide structured project/phase planning, templated tasks, and ready-to-work queues for human or agent contributors.

**Goal:** Keep the Beads graph as the source of truth while adding higher-level ergonomics: phase manifests, task templates, validation rules, and state machine enforcement.

## System Shape

- **Data Backbone:** Beads issue graph stored/queried via `bd`. We read/write issues, dependencies, and statuses through the Beads API/CLI interface.
- **Go SDK (`pkg/pt`):** Reusable library exposing plan/sync/ready/validate primitives. Any service or agent can embed this logic.
- **CLI (`cmd/pt`):** A thin wrapper around the SDK. Guides users through schema fields, enforces state machines (`planned` → `ready` → `in_progress` → `needs_review` → `done`), and handles user interaction.
- **Manifests:** TOML/JSON files for "Phase Bundles". These act as "Infrastructure as Code" for project management.
- **Context Contracts:** Strict schemas (`contracts/*.toml`) that validate agent inputs (files, goals, criteria) to prevent drift and hallucination.
- **Validation:** "Definition of Done" (DoD) per template, plus ready-work queries that only surface unblocked tasks.

## Library & CLI Split

### SDK (`pkg/pt`)
Stateless logic package.
- `PlanPhase(manifest)`: Validates and parses a manifest into a graph structure.
- `Sync(manifest)`: Idempotently applies the plan to the `bd` graph (creates/updates issues, sets deps).
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

```
   Manifest (TOML)                Beads Graph (bd)
         |                              ^
         | pt sync                      |
         v                              |
   +------------------+    bd create/update|issues/deps
   | pt CLI / SDK     |--------------------+
   | (Stateless logic)|    bd query ready  |
   +------------------+------------------->|
         |                              v
         | pt ready / validate / claim  Ready Queue
         v                              |
   [Context Builder] <--(contract)-- [Validator]
         |
         v
   Human/Agent Worker
   (claim -> work -> validate -> review <-> reject/done)
```

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

### 1. Plan & Sync
Apply a manifest to the Beads graph. This creates issues, sets dependencies, and stores validation rules.
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

More examples live in `phases/login_flow.toml` and `phases/hotfix_bug.toml`.

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
| **Validation Rejection** | `pt reject <id> "Fix imports"` | Task status `needs_review` → `in_progress`; comment added to `bd`. |
| **Validation Failure** | Force test fail; `pt validate` | Task stays `in_progress`; error output surfaced. |
| **Concurrent Claim** | Two agents `pt claim <id>` | First succeeds; second gets "Already claimed" error. |
| **Manual Gate** | `pt validate` on task w/ manual check | CLI prompts "Did you check X?"; blocks until confirmed. |
| **Cross-Phase Deps** | Phase 2 depends on Phase 1 task | Phase 2 task only ready when Phase 1 task is done. |

## Notes for Contributors
- **Language:** Go (1.21+) for SDK/CLI.
- **Testing:** Always run `go test ./...` (no skips).
- **Style:** Follow standard Go idioms.
