# PT Core Tasks (Focus List)

This file tracks **only** tasks that are directly related to building and hardening the `pt` tool itself (SDK/CLI/docs). Everything else (example manifests, dogfood projects, other product tracks) should be treated as **out of scope** for PT-core work.

## PT-only tasks to focus on

### Active (open / in progress)

- None currently. Use `pt next` / `pt ready` to drive new work.

### Recently completed (for context)

- `pt-55` — User sign-off: task authoring guidance
- `pt-140` — V2: task authoring guidance (targeted tests + manual "N/A")
- `pt-66` — `pkg/contract` Loading
- `pt-67` — `pkg/contract` Validation
- `pt-68` — `pt context` CLI
- `pt-69` — Context Contracts docs
- `pt-73` — Docs refresh
- `pt-90` — Default DB moved to `.pt/db.json`
- `pt-91` — Worktree-aware DB discovery
- `pt-100` — New project quick-start (`docs/QUICKSTART.md`)
- `pt-108` — Worktree tracking hardened (real git test)
- `pt-138` — `pt ready` applies `--limit` after filtering blocked tasks
- `pt-139` — `pt validate` runs DoD from project root when store is `.pt/db.json`

## How to use this list

- Use `pt next --all-phases` to drive work, but only claim tasks from the PT-core list above.
- Mark non-PT tasks as blocked (“out of focus”) so the conductor doesn’t recommend them.
- If a non-PT task is a pure artifact (e.g., one-off test tasks), close it to reduce noise.

## Regenerating the list (optional)

If needed, regenerate this file by querying the store and filtering on PT-core artifacts (`code:cmd/pt/`, `code:pkg/pt/`, `code:contract*`, `doc:pt-*`).
