# Demo System Design

## Goal

Make “demo” a first-class, repeatable artifact: a user can run it and *see* the product working (not just read a summary). The demo should evolve per phase, and finish with an end‑to‑end walkthrough at signoff.

## Source of Truth: `.pt/demo/`

Store demo intent + evidence inside the project DB directory:

```
.pt/demo/
  README.md                 # Human entrypoint: what to run, what to expect
  demo.toml                 # Machine spec: scenarios/commands/artifacts
  phases/<phase-id>/...     # Optional phase demos (prove/explore/build/integrate)
  artifacts/<timestamp>/    # Captured outputs (logs, screenshots, reports)
```

This directory is the canonical location for demo instructions and outputs.

## Demo Spec (`demo.toml`)

Keep it small and uniform across project types:

```toml
project_type = "cli" # cli|web|library|service|extension

[[scenario]]
id = "help"
phase = "build"
goal = "Show CLI guidance and common commands"
commands = ["pt --help", "pt next --json"]
artifacts = ["artifacts/{{ts}}/help.txt", "artifacts/{{ts}}/next.json"]
expect = ["help includes workflow guidance", "next includes checkpoint rails"]

[[scenario]]
id = "e2e"
phase = "signoff"
goal = "End-to-end walkthrough"
commands = ["go test ./...", "go run ./cmd/<app> --demo"]
artifacts = ["artifacts/{{ts}}/tests.txt", "artifacts/{{ts}}/demo.log"]
```

## What a “Demo” Means by Project Type

- **CLI**: `--help`, 1–3 golden invocations, 1–2 failure cases, optional `--json` mode.
- **Web**: start server, URLs to visit, key screenshots (happy path + error state).
- **Library**: `go run ./examples/...`, expected stdout, API usage snippet.
- **Service/API**: start service + health check, curl scripts, responses captured.

## PT Integration (Rails, Not Gates)

- `pt review write <id> --kind=demo` should point at `.pt/demo/` and record exact run instructions + expected artifacts.
- Demo tasks should validate *existence of the demo artifact* (and ideally the captured outputs) without blocking unrelated work.
- Future: `pt demo run` executes `demo.toml`, captures artifacts under `.pt/demo/artifacts/<timestamp>/`, and comments paths back onto the task.

## Phase Expectations

- **prove**: show spike evidence (real payloads, notes).
- **explore**: show options/comparisons (ASCII flows, example IO).
- **build**: show a working slice.
- **integrate**: show real dependency/integration test evidence.
- **signoff**: show end‑to‑end demo a user can run.

