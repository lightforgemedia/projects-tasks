# Spike: choose browser control backend (Go-Rod vs chromedp)

## Decision
Use **Go-Rod** as the Pulse controller backend for the MVP.

## Why
- **Ergonomic CDP wrapper** with a stable, high-level API for the “Controller” responsibilities (launch, navigate, network, JS eval).
- **Good fit for Pulse’s architecture**: we can inject `window.__pulse` via init scripts and drive interactions deterministically.
- **Keeps the MVP narrow**: we can postpone the “hybrid CDP + extension messaging” split until micro-flows and fingerprints are working.

## Evidence (real run)
Integration smoke test executed successfully:
- Command: `go test -tags=integration ./... -run TestRodCanLaunchNavigateAndEval -v`
- Output log: `projects/pulse/outputs/spikes/rod_smoke_test.log`
- Result: PASS

## Minimal PoC plan (next tasks)
1. Add a `pulse` CLI command to execute a single micro-flow TOML against a target URL (no impact-mapper yet).
2. Implement controller lifecycle: start browser (headless by default), open page, navigate, wait for DOM ready.
3. Inject a minimal runtime stub (`window.__pulse`) and prove round-trip RPC via `Eval`.
4. Keep the action surface tiny: `GetState`, `ListInteractive`, `Act(click/type)`, `Snapshot`.

## Environment / prerequisites
- A Chromium-compatible browser available for Rod to launch (Rod can download/use a bundled Chromium depending on config).
- CI/local: enable integration runs via `-tags=integration` (unit tests remain `go test ./...`).
