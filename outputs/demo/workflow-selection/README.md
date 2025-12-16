# Demo: Workflow Selection UX (claim/ready/next)

This demo shows:
- Auto-discovery errors are **clear** when multiple `workflows/*.toml` exist.
- `pt claim` and `pt ready` accept `--workflow=PATH` to disambiguate (no silent gate skipping).
- Workflow discovery is **scoped to the project store** (`PT_DB`), not the caller’s repo/cwd.

## Run

From the repo root:

```bash
go build -o pt ./cmd/pt
./outputs/demo/workflow-selection/run.sh
```

Artifacts are written under `outputs/demo/workflow-selection/out/`.

