# Hook Design (planned)

Goal: Allow project-local automation around `pt` state transitions without modifying core code. Hooks are declarative (TOML) and run external commands with a consistent env payload.

## Events
- `pre-sync`, `post-sync`
- `pre-claim`, `post-claim`
- `post-release`
- `pre-validate`, `post-validate`
- `post-approve`, `post-reject`

## Config (TOML)
Place `hooks.toml` in the project root (or set `PT_HOOKS=path`). Each `[[hook]]` has:
- `event`: one of the above.
- `cmd`: shell command to run (string).
- `on_fail`: `fail` (block the action) or `warn` (log only). Default: `fail` for pre-hooks, `warn` for post-hooks.
- `timeout`: optional (e.g., `10s`).

Example:
```toml
[defaults]
  timeout = "15s"
  on_fail = "warn"

[[hook]]
  event = "pre-claim"
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
- `PT_DOD`: JSON of the DoD for validate/approve/reject.

## Execution Rules (planned)
- Pre-hooks run before state mutation; `on_fail=fail` blocks the action.
- Post-hooks run after mutation; `on_fail=warn` logs but does not roll back.
- Hooks are run serially per event in file order.
- CLI should print hook command, duration, and exit code when `--verbose` is set.

## Wiring Plan
- Load hooks once at CLI start (respect `PT_HOOKS` path, default `hooks.toml`).
- Invoke `runHooks(event, context)` around transitions in `cmd/pt/main.go` (sync, claim, release, validate, approve, reject).
- Keep the library stateless: hook runner stays in CLI layer; SDK remains pure.

## Use Cases
- Enforce ownership policy before claim.
- Notify chat after validate passes/fails.
- Trigger deploy after approve.
- Emit audit log to a file or SIEM on every transition.
