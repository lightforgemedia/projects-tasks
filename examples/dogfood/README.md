# Dogfood Demo: Telemetry Service

This example shows how to run `pt` for a realistic mini-project with multiple agents.

## Files
- `manifest.toml`: tasks for API, frontend widget, alerts, and cleanup.
- Store: defaults to `.pt.db` in the project root; override with `PT_DB`.

## Runbook (multi-agent)
1) **Sync the plan**
```bash
pt sync examples/dogfood/manifest.toml
```
2) **Find work**
```bash
pt ready --role=backend-dev --verbose
```
3) **Claim with identity**
```bash
pt claim <id> --as=<your-name>
```
4) **Execute + validate**
- Implement the task.
- Run `pt validate <id> [--yes]` (records manual steps in the comment).
5) **Review**
```bash
pt approve <id>          # closes
pt reject <id> --reason "..."  # back to in_progress with a comment
```
6) **Cleanup / handoff**
- If stuck, `pt release <id>` to unblock others.
- Leave a concise comment with status/next step.

## Tips for agents without context
- `pt context init <id>` to see required inputs for your role.
- Use `pt ready --verbose` to see blockers and assignees before claiming.
- Respect dependencies; don’t claim blocked items.

## Automation hook ideas (until built-in hooks land)
- Wrap `pt validate` to notify chat/slack on success/fail.
- Wrap `pt approve` to trigger deploy or merge workflows.
- Wrap `pt reject` to file a follow-up checklist.

## Integration script
Run a simple end-to-end sanity check from the repo root:
```bash
bash examples/dogfood/integration.sh
```
