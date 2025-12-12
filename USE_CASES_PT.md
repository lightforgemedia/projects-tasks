# PT CLI – Practical Use Cases

Ten common scenarios where PT’s manifest + task graph shine (all use the built-in store; swap `pt` commands as needed):

1) **Feature Sprint (Backend + Frontend)**  
   - Define tasks with deps: API spec → backend endpoint → frontend component.  
   - Use `pt graph` to visualize blockers; `pt ready --role` to feed dev vs. UI tracks.

2) **Bug Fix with Repro and Guardrails**  
   - Task includes artifact (`bug:1234`) and DoD: repro script, unit test, manual check.  
   - `pt validate` runs repro + tests before review to prevent regressions.

3) **Schema Migration with Safety**  
   - Tasks: design ADR → migration script → rollout/verify.  
   - Deps enforce order; DoD includes `go test` + manual DB sanity; hooks can block claim if tests fail.

4) **Observability Hook**  
   - Add alert/telemetry tasks with artifact (`obs:latency`) and criteria (alert fires on bad input).  
   - `pt show` surfaces criteria for reviewers.

5) **Integration with External API**  
   - Tasks: capture real API fixture → client lib → UI consumption.  
   - DoD includes “real call succeeds” + contract criteria; deps prevent UI before fixture.

6) **Docs & Runbook Update**  
   - Task links to artifact (`doc:runbook`), criteria: “steps tested end-to-end”.  
   - DoD: `tests` (lint/build) + manual validation of doc steps.

7) **Release Hardening**  
   - Phase manifest of “gap” tasks (tests, lint, hooks).  
   - `pt ready --verbose` shows blockers; `pt validate --yes` runs test suite before sign-off.

8) **Refactor with Safety Nets**  
   - Task artifact (`code:module/foo`), criteria: “behavior unchanged; metrics same”.  
   - DoD: targeted tests + manual sanity; `pt comment` captures risk notes.

9) **Agent Onboarding / Context Init**  
   - Use `pt context init` to fetch role-specific payload; tasks carry artifact and DoD so agents know specs/tests.  
   - `pt show` gives full context (criteria/tests/manual steps) without opening the DB.

10) **Ad-hoc Spike / Investigation**  
   - Create via `pt add` with artifact (`adr:spike-x`), DoD: what to measure, how to report.  
   - `pt approve` only when criteria (findings recorded, next steps logged) are met.
   - Template: use `spike` to enforce “assumptions tested + outcomes recorded.”

11) **Multi-project Coordination (read-only)**  
   - `pt multi-ready --dbs=a.json,b.json` to aggregate ready work across stores while keeping per-project DoD and artifacts intact.

Tips:
- Always include `artifact`, `tests`, `manual`, and `criteria` in DoD; `pt add` and manifests enforce this.  
- Use labels (`role:<role>`, `artifact:<id>`, `template:<name>`) for quick filters.  
- `pt snapshot` before risky edits; `pt hooks` to inspect pre/post validations.
