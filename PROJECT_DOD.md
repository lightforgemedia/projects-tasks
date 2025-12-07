# Project Definition of Done (root)

Use this file when `PT_PROJECT_DOD` is unset. It defines “done” for the repo as a whole:

1) Tests & Lint
   - `go test ./...` passes.

2) Task Hygiene
   - `pt ready` shows no open tasks.
   - All manifests in `phases/` are synced; new work is captured as tasks (or noted as TODOs).

3) Subproject DoD
   - Each subproject with deliverables has its own `PROJECT_DOD.md` (e.g., `projects/codexacp-client/PROJECT_DOD.md`).
   - `PT_PROJECT_DOD` can point to a subproject DoD when working there.

4) Docs & UX
   - README/AGENTS reflect current commands and flows.
   - Known gaps/risks are documented (e.g., hooks limitations, multi-project caveats).

5) Sign-off
   - A review/sign-off task is closed for the current effort, referencing the relevant DoD file and outcomes.
