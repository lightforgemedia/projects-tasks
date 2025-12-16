# Pulse — Agentic Incremental UI Testing & Interaction Engine

Pulse is an example project to dogfood `pt` on a realistic, risk-heavy workflow:

- **Micro-flows**: tiny, independent UI journeys defined in TOML.
- **Impact-based execution**: run only flows impacted by `git diff` + a P0 smoke set.
- **Pulse runtime**: a lightweight in-page shim (`window.__pulse`) that exposes semantic DOM ops.
- **Hybrid transport**: CDP for lifecycle; extension/in-page runtime for scoped DOM interaction.

This project is intentionally built **risk-first**: prove unknowns (browser control + injection) before building runners and UX.

## Running PT for this project

From `projects/pulse/`:

```bash
export PT_DB="$PWD/.pt/db.json"
export PT_PREFIX="pulse"
export PT_WORKFLOW="$PWD/workflows/risk-first.toml"

../../pt ready --verbose
../../pt next
```

## Project Definition of Done

See `projects/pulse/PROJECT_DOD.md`.

