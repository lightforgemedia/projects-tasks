# Realworld App Example

This example simulates a realistic project (“SaaS Workspace Onboarding”) using `pt` and its internal store.

## How to run
```bash
# From repo root
pt sync examples/realworld-app/manifest.toml
pt ready --role=backend-dev --verbose
pt claim <id>
# ...implement...
pt validate --yes <id>
pt approve <id>
```

## Manifest
`examples/realworld-app/manifest.toml` defines:
- DB schema migration for workspaces
- Backend endpoints for creating workspaces and inviting members
- Frontend flows (workspace creation page, invite dialog)
- Observability metric hook for the onboarding funnel

Dependencies wire backend → frontend → observability to model a real delivery sequence. Manual DoD steps are present; use `--yes` to auto-confirm after you’ve performed them. Blockers/assignee info is surfaced with `pt ready --verbose`.
