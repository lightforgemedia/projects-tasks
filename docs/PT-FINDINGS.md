# PT Workflow: Findings Summary

**Date**: 2024-12
**Context**: Analysis from SPY options tool development

---

## Documents

| Doc | Purpose | Stability |
|-----|---------|-----------|
| [WORKFLOW-PRINCIPLES.md](./WORKFLOW-PRINCIPLES.md) | Risk-first philosophy, mock discipline, phase gates | Stable |
| [PT-TOOL-REFERENCE.md](./PT-TOOL-REFERENCE.md) | PT commands, manifest schema, workarounds | Evolves |

---

## Key Findings

### Bugs Found

1. **gitIsDirty() cwd bug**: Runs in current dir, not repo root. Worktree commands fail from subdirs.
2. **PT_DB path resolution**: Relative paths resolve per-cwd, causing state confusion.
3. **Worktree start blocks parallel work**: Modifies tracked db, blocks second worktree.
4. **Validate in worktree writes wrong db**: Causes `worktree done` to fail.

### What Works

- Core state machine is sound
- Manifest sync with TOML is clean
- Workflow conductor visualization is helpful
- Gate evaluation logic works

### Architectural Insight

Store in repo root + git tracking is the root cause of worktree friction. Either:
- Move db outside tracking (`.pt/db.json` + `.gitignore`)
- Or use user-local store (`~/.pt/<project>/db.json`)

Both work today via `PT_DB` env var. No code changes needed.

---

## Philosophy Captured

The SPY options work revealed a better default workflow than "frontend-first":

**Risk-first**: Prove unknowns → Explore UX → Build (hardest first) → Integrate → Signoff

Key insight: Gates belong at phase boundaries, not every task. Let work flow within phases; enforce at exits.

**Mock discipline**: The Mock Permission Ladder with expiration and mandatory integration tasks prevents the common failure mode of tests passing against stale/invented mocks.

See [WORKFLOW-PRINCIPLES.md](./WORKFLOW-PRINCIPLES.md) for full philosophy.

---

## Improvement Manifest

Created `phases/pt-improvements.toml` with:
- Phase 1: Blocking bugs (gitIsDirty fix, db location)
- Phase 2: Workflow improvements (spike type, mock tracking, checkpoints)
- Phase 3: UX polish (porcelain mode, command shortcuts)
- Phase 4: Documentation updates

---

## Refinements to Consider

From review feedback:

1. **Mock expiration**: Add `expires_at` to force re-spike or integration
2. **Integration test cadence**: PR=unit, nightly=integration, release=hard gate
3. **Phase constraints for mocks**: Integration task must be in current/next phase, not just "exists"
4. **Agent mock awareness**: Machine-readable mock signals for automated consumers
5. **Retirement verification**: `pt mock retire` should fail if mock still imported outside tests

These are captured in the tool reference as proposed features.

---

*Summary document. Details in linked docs.*
