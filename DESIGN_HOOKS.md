# Hook Design

Goal: Allow project-local automation around `pt` state transitions without modifying core code. Hooks are declarative (TOML), run external commands, and receive a consistent payload so automation can be audited and replayed.

## Events
- `pre-sync`, `post-sync`
- `pre-claim`, `post-claim`
- `post-release`
- `pre-validate`, `post-validate`
- `post-approve`, `post-reject`
- `pre-reopen`, `post-reopen`
- `pre-update`, `post-update`

## Config (TOML)
Place `hooks.toml` in the project root (or set `PT_HOOKS=path`). Each `[[hook]]` has:
- `event`: one of the above.
- `cmd`: shell command to run (string).
- `on_fail`: `fail` (block the action) or `warn` (log only). Default: `fail` for pre-hooks, `warn` for post-hooks.
- `timeout`: optional (e.g., `10s`).

Optional matchers (to avoid workflow footguns):
- `only_templates`, `skip_templates`
- `only_roles`, `skip_roles`
- `only_labels`, `skip_labels`

Each matcher value is a comma-separated list of patterns:
- exact match: `discovery`
- prefix match: `checkpoint:` (matches `checkpoint:required`, `checkpoint:demo`, …)
- wildcard suffix: `checkpoint:*` or `phase:*`

Parser note: `hooks.toml` is parsed by a small, dependency-free parser. Keep values simple:
- Prefer no embedded double-quotes inside `cmd`.
- Prefer placeholders (`{{id}}`, `{{template}}`) and env vars (`$PT_ID`, `$PT_TEMPLATE`) over complex escaping.

Example:
```toml
[defaults]
  timeout = "15s"
  on_fail = "warn"
  verbose = true

[[hook]]
  event = "pre-claim"
  # Skip policy checks for checkpoint tasks.
  skip_labels = "checkpoint:*"
  cmd = "scripts/policy/check-owner.sh {{id}} {{assignee}}"
  on_fail = "fail"

[[hook]]
  event = "post-validate"
  cmd = "scripts/notify/slack.sh validate {{id}} {{status_to}}"

[[hook]]
  event = "post-approve"
  cmd = "scripts/deploy/trigger.sh {{id}}"
```

## Env Payload (to the hook command)
- `PT_EVENT`: event name (e.g., `pre-claim`).
- `PT_ID`: task ID (e.g., `pt-12`).
- `PT_TITLE`: task title.
- `PT_ASSIGNEE`: assignee after the transition (for claim/release).
- `PT_ACTOR`: who ran the CLI (from `--as` or `$USER`).
- `PT_STATUS_FROM` / `PT_STATUS_TO`: before/after statuses.
- `PT_ROLE`: task role label.
- `PT_TEMPLATE`: task template (e.g., `backend_endpoint`, `discovery`).
- `PT_LABELS`: comma-separated labels.
- `PT_PHASE`: best-effort phase label value (from `phase:<id>`), if present.
- `PT_DOD`: truncated DoD JSON (small; see stdin payload below).

## Stdin Payload (recommended for hooks)
The full hook payload (JSON) is sent on stdin for every hook. This avoids `E2BIG` environment limits for large task metadata.

## Execution Rules
- Pre-hooks run before state mutation; `on_fail=fail` blocks the action.
- Post-hooks run after mutation; `on_fail=warn` logs but does not roll back.
- Hooks are run serially per event in file order.
- Hook results are returned to the CLI as structured entries (ok/warn/fail/skipped).

## Wiring Plan
Hooks are wired in `cmd/pt` (CLI only). The library remains stateless.

Load order:
1. `$HOME/.config/pt/hooks.toml` (optional)
2. `./hooks.toml` (project root)
3. `PT_HOOKS=/path/to/hooks.toml` overrides both

Debugging:
- `pt hooks` prints the merged config as JSON.
- `PT_HOOK_VERBOSE=1` forces logging.
- `PT_SKIP_HOOKS=1` disables hooks (debug only; use sparingly).

## Use Cases
- Enforce ownership policy before claim.
- Notify chat after validate passes/fails.
- Trigger deploy after approve.
- Emit audit log to a file or SIEM on every transition.

## Default safety guardrails
This repo’s `hooks.toml` runs `go test ./...` on `pre-claim`, `pre-validate`, and `pre-approve`, but skips that check for:
- `template=discovery|spike` (planning / proof tasks)
- `label=checkpoint:*` (kickoff/closeout/demo rails)
