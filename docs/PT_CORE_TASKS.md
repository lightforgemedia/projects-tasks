# PT Core Tasks (Focus List)

This file tracks **only** tasks that are directly related to building and hardening the `pt` tool itself (SDK/CLI/docs). Everything else (example manifests, dogfood projects, other product tracks) should be treated as **out of scope** for PT-core work.

## PT-only tasks to focus on

Open / in progress PT-core tasks in the current store (grouped by intent):

- **Process / task quality**
  - `pt-55` — PT: review task authoring guidance with user
- **Contracts & context**
  - `pt-66` — Implement `pkg/contract` Loading (`code:contract-loading`)
  - `pt-67` — Implement `pkg/contract` Validation (`code:contract-validation`)
  - `pt-68` — Implement `pt context` CLI (`code:pt-context-cli`)
  - `pt-69` — Document Context Contracts (`doc:context-contracts`)
- **Docs & onboarding**
  - `pt-73` — Update Documentation (`doc:pt-docs`)
  - `pt-99` — Update README with workflow philosophy (`doc:readme-update`)
  - `pt-100` — Create quick-start guide for new projects (`doc:quickstart`)
- **DB / multi-project ergonomics**
  - `pt-90` — Move db outside git tracking (`code:db-location`)
  - `pt-91` — Auto-discover db location (`code:db-discovery`)
  - `pt-108` — Implement store split or worktree tracking alternative (`code:store-architecture`)
- **CLI / conductor UX**
  - `pt-95` — Improve phase assignment visibility (`code:phase-visibility`)
  - `pt-97` — Reduce command verbosity (`code:command-ux`)
  - `pt-137` — Fix: `pt next` should prefer claimable tasks (not just open) (`code:cmd/pt/next.go`)

## How to use this list

- Use `pt next --all-phases` to drive work, but only claim tasks from the PT-core list above.
- Mark non-PT tasks as blocked (“out of focus”) so the conductor doesn’t recommend them.
- If a non-PT task is a pure artifact (e.g., one-off test tasks), close it to reduce noise.

## Regenerating the list (optional)

If needed, regenerate this file by querying the store and filtering on PT-core artifacts (`code:cmd/pt/`, `code:pkg/pt/`, `code:contract*`, `doc:pt-*`).
