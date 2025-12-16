# Pulse — Project Definition of Done (MVP)

MVP is complete when:

- `go test ./...` passes from `projects/pulse/`.
- A local demo page + at least **3 micro-flows** run reliably:
  - “P0 smoke” flow(s) always run.
  - Impact filtering runs the correct tagged flows for a sample `git diff`.
- `pulse run` prints a clear per-step report (pass/fail + reason) and produces a `Snapshot` artifact on failure.
- Fingerprints exist and drift policy is enforced:
  - Structural drift hard-fails with actionable output.
  - Cosmetic drift is tolerated within configured bounds.
- Self-healing fallback works end-to-end for one drift scenario:
  - Runner detects fingerprint mismatch.
  - Runner emits a suggested fingerprint update file (`*.fp.toml`) for review.

Evidence:
- Test output for unit + integration tests.
- A sample run log for `pulse run` (success + one intentional failure).
- Captured artifacts under `projects/pulse/outputs/` (snapshots / reports).

