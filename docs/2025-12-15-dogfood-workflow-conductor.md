# Dogfood: Workflow + Conductor + Hooks (Findings)

**Date:** 2025-12-15  
**Scope:** Stabilize the “don’t skip ahead” SDLC loop when a workflow is active and hooks run `go test ./...` before transitions.

## What was validated

The following end-to-end loop works with a workflow enabled:

1. `pt next` suggests the next action (work vs review).
2. `pt claim <id>` triggers `pre-claim` hooks and blocks on failure.
3. `pt validate --yes <id>` triggers `pre-validate` hooks, runs DoD tests, and moves the task to `needs_review`.
4. `pt approve <id>` triggers `pre-approve` hooks and closes the task.

### Hooks evidence

`hooks.toml` runs `go test ./...` and writes output to `/tmp/pt_hook_go_test.log`. In the dogfood run, the hook passed and the log contained the expected `ok ...` lines.

## Fixes that unblocked dogfooding

- Tests are now resilient to a parent environment setting `PT_WORKFLOW` (cleared in `cmd/pt/store_helpers_test.go`), which is important when hooks run `go test ./...` from arbitrary directories.
- Workflow semantics were corrected so the first phase gate is treated as an **exit gate** (blocks later phases) rather than an **entry gate** (which incorrectly blocked claiming tasks in phase 1).
- Workflow file resolution was hardened so `PT_WORKFLOW=workflows/<name>.toml` works even when invoked from a subdirectory.

## Issues found (logged as PT tasks)

1. **Flag ordering UX:** `pt validate <id> --yes` fails due to Go’s flag parsing rules and produces a misleading “missing id” error.
   - Tracked as: `pt-122` (local store).
2. **Identity leakage in guidance:** `pt next` recommends `pt claim ... --as=<local username>` instead of a placeholder or omission.
   - Tracked as: `pt-123` (local store).

## Note on task persistence

The default store file `.pt.db.json` is currently ignored by git (`.gitignore`), so newly-created tasks won’t appear in commits. If task history needs to be shareable/reviewable via git, use `pt export`/`pt snapshot` or switch to a tracked store location for the repo.

