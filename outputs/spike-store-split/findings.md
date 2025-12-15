# Spike: Store Split Architecture (Shared vs Worktree-Local)

**Goal:** Decide whether PT should keep a single shared store, or split state between a repo-level store and worktree-local state.

## Current model (today)

- Store: single JSON file (default `.pt.db.json`) + lock file.
- Worktrees: optional mapping recorded in the store; validation runs tests in worktree when a mapping exists.

**Pros:** simple mental model, easy queries (`pt list`, `pt next`), one source of truth.  
**Cons:** sharing via git is hard (file ignored by default; conflicts); worktree isolation is logical, not physical.

## Options

### Option A — Single shared store (recommended default)

```
repo/
  .pt.db.json        # tasks + history + deps + worktree mappings
  worktrees/
    pt-123/          # git worktree (code)
    pt-124/
```

- Keep all task state in one place.
- Worktrees are just an execution context; the store stays shared.

**Team sync:** use `pt export`/`pt snapshot` for artifacts; do not rely on git-merging the store.  
**CI access:** CI can read the store if it’s present (or restored from export).  
**Migration:** none.

### Option B — Split store: shared “project” store + worktree-local “session” store

```
repo/
  .pt.project.json   # tasks + deps + workflow + history
  worktrees/
    pt-123/
      .pt.session.json  # local notes, scratch, ephemeral state
```

Use cases for the local store:
- local-only notes and intermediate artifacts
- per-worktree scratch decisions
- local “in-progress” logs without polluting project history

**Risks:** state divergence (what’s authoritative?), more merge/consistency code, more commands need dual reads/writes.

### Option C — No worktree mapping in PT; derive from git

- PT never stores worktree mapping.
- `pt validate` would have to infer worktree paths via `git worktree list` and heuristics (task ID in branch/path).

**Pros:** less state in PT.  
**Cons:** brittle (naming conventions), less portable, weaker orchestration UX (“where is task X being worked on?”).

## Recommendation

**Recommend Option A (single shared store) as the stable default.**

Reasoning:
- PT’s value is orchestration and workflow guidance; a single store maximizes determinism for `pt next` and gate enforcement.
- Worktree-local state is attractive, but adds new failure modes (split-brain, “done locally but not globally”) that undermine the SDLC guarantees.
- For collaboration and durability, prefer explicit exports/snapshots over implicit git merges of the store.

## Follow-ups (if collaboration is needed later)

1. Add `pt export` / `pt import` workflows for sharing state snapshots.
2. Add `pt doctor` checks for stale/missing worktrees and store integrity.
3. If Option B becomes necessary, constrain it to **non-authoritative** data only (notes/proofs), never status/deps.

