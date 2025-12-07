# Multi-Project Access (Design)

## Problem
- We want agents/humans to reference or propose changes to tasks across multiple projects without “switching” working directories. Current `pt` assumes a single store (`.pt.db.json`) per working dir. There is no way to read/write another project’s store explicitly, nor to propose changes safely without direct writes.
- We also want a PR-like proposal flow (produce a patch/diff for review) rather than unconditional writes, and a safe read-only aggregation of ready work across multiple projects.

## Goals
- Read tasks across multiple projects/stores without changing directories.
- Write in two modes:
  1) Proposal (no mutation): generate a manifest diff/patch for a target store.
  2) Targeted write: allow explicit `--db`/`--prefix` per command with flock-based safety.
- Avoid deletes; respect existing prefixes/IDs; minimize cross-project coupling.
- Keep `pt` store format unchanged; hooks remain per-project.

## Non-goals (for now)
- No multi-host replication or remote backend.
- No auth/permissions layer (trust-based access).
- No automatic conflict resolution; flock covers same-host concurrency only.

## Proposed Features
1) **Targeted DB/PREFIX overrides (explicit)**
   - All write commands accept `--db <path>` and `--prefix <pfx>` to target another project’s store without changing cwd.
   - Flock already protects same-host writes; we rely on shared filesystem semantics.

2) **Proposal/PR flow**
   - New command: `pt propose --db <path> --manifest <file>` (or `--tasks <file>`) that:
     - Parses tasks and computes the would-be changes vs. target store (creates/updates, deps).
     - Outputs JSON (or file) with proposed additions/updates, no mutations.
     - Suitable for review/PR; a human runs `pt sync --db <path> <manifest>` to apply.

3) **Multi-project ready aggregation (read-only)**
   - New command: `pt multi-ready --dbs a.json,b.json [--role=...] [--json]` to list ready work across stores (read-only).
   - No writes; obeys flock on read open where applicable.

4) **Safety/guardrails**
   - No deletes across stores.
   - Respect target store’s prefix when suggesting IDs; do not override prefix unless explicitly passed.
   - Clear messaging when writing to non-default store (`--db` required for writes outside cwd).

## CLI Sketch
- `pt ready --db <path> --json` (extends existing commands to accept db override explicitly).
- `pt claim <id> --db <path> --as user`
- `pt propose --db <path> --manifest <file> [--json | --out file]`
- `pt multi-ready --dbs a.json,b.json [--role=...] [--json]`

## Data/Format
- Store format unchanged; flock already added.
- Proposal output: JSON structure with:
  ```json
  {
    "target_db": "...",
    "adds": [{"title": "...", "deps": [...], "role": "...", "dod": {...}}],
    "updates": [{"id": "proj-1", "fields": {"title": "...", "deps": [...]}}],
    "notes": ["prefix will be proj", "no deletes"]
  }
  ```

## Risks / Open Questions
- Concurrency across hosts still unsolved.
- Trust model: explicit `--db` required; proposal flow recommended for cross-project contributions.
- Large manifests: ensure propose is read-only and fast (no hook execution).

## Next Steps
- Implement db/prefix flags on write commands with clear messaging.
- Implement `pt propose` (no-op apply, JSON diff output) and `pt multi-ready` (read-only aggregation).
- Update docs (README, AGENTS) with cross-project guidance and safety notes.
