# Handoff: PT Path Resolution Fixes

**Date:** 2025-12-13
**Author:** Claude (Agent)
**Scope:** Fix working directory bugs in PT task management CLI

---

## 1. Dependency & Integration Status (REVIEW THIS FIRST)

**Mocking is permitted ONLY when ALL conditions are met:**
1. ✅ Real behavior has been proven (spike ran against actual dependency)
2. ✅ Mock faithfully reproduces proven behavior (not assumed behavior)
3. ✅ Task exists in task system to return to full integration
4. ✅ User-facing indicators show when mocked data is in use
5. ✅ Integration tests against real dependency exist and pass

### External Dependencies

| Dependency | Real Behavior Proven? | Evidence | Mock Status |
|------------|----------------------|----------|-------------|
| Git CLI | ✅ Yes | Integration tests create real git repos | No mocks |
| Filesystem | ✅ Yes | Tests use real temp directories | No mocks |
| JSON store | ✅ Yes | Tests read/write actual `.pt.db.json` files | No mocks |

### Mock Registry

**No mocks introduced.** All tests operate against real git repos and real filesystem.

| Mock Location | What Real Behavior Was Proven | Proof Evidence | Return-to-Real Task | User Indicator |
|---------------|------------------------------|----------------|---------------------|----------------|
| N/A | N/A | N/A | N/A | N/A |

**⚠️ UNMOCKED DEPENDENCIES (integration tests required):**
- Git worktree operations: `cmd/pt/path_scenarios_test.go:TestPathScenarios_DiscoveryFromWorktree`
- Store file operations: `cmd/pt/path_scenarios_test.go:TestPathScenarios_StoreResolution`
- Git status/dirty detection: `cmd/pt/path_scenarios_test.go:TestPathScenarios_GitHelpers`

Last real integration run: 2025-12-13 (all tests pass)

### Reviewer MUST verify:
- [x] Every mock has corresponding proof of real behavior (N/A - no mocks)
- [x] Every mock has a tracked task for removal/integration (N/A - no mocks)
- [x] No silent mocks—user always knows when data is fake (N/A - no mocks)
- [x] Integration tests exist and are not skipped in CI

---

## 2. Risk Spike Status

| Risk Area | Spike Status | What Was Proven | What's Still Assumed |
|-----------|--------------|-----------------|----------------------|
| Git worktree discovery | ✅ Validated with real worktrees | `git rev-parse --git-common-dir` correctly identifies parent repo | Works across all git versions (tested on macOS git 2.x) |
| Path resolution race | ✅ Validated | `filepath.Abs()` at construction time prevents cwd drift | No concurrent test mutations of cwd |
| Git -C flag | ✅ Validated | All git commands work with explicit path | Git version supports -C (2.x+) |
| Store locking | ⚠️ Existing behavior | Not modified in this change | File locking works correctly (pre-existing) |

**Unproven risks the reviewer should scrutinize:**
- **Concurrent worktree operations:** Tests run sequentially. Parallel test runs with `os.Chdir` could still interfere (mitigated by using `t.TempDir()` and avoiding `os.Chdir` where possible).
- **Symlinked paths:** `filepath.EvalSymlinks` is used for comparison but not all code paths normalize symlinks consistently.

---

## 3. UX Exploration Summary

### What was shown to users (or user-proxy agents):
- [x] CLI input-output examples (existing PT commands unchanged)
- [x] Breadth-first options (discussed in planning)
- [x] Key decision points with user sign-off

### Exploration artifacts:

This was a **bug fix**, not a feature. UX is unchanged—the CLI behaves the same, but now works correctly when:

```
# Before (broken): Running from worktree used wrong store
cd /project
pt claim pt-5
pt worktree start pt-5    # Creates worktree at /worktrees/pt-5
cd /worktrees/pt-5
pt validate pt-5          # BUG: Created new store in worktree, lost track of task

# After (fixed): Store discovered from parent repo
cd /worktrees/pt-5
pt validate pt-5          # Correctly finds /project/.pt.db.json
```

### User decisions captured:
- **Decision:** Use `git -C <path>` pattern instead of `os.Chdir`
- **Why:** `os.Chdir` is process-global, causes race conditions in tests and concurrent operations
- **Alternatives rejected:**
  - Subprocess isolation (too complex)
  - Mutex around all git operations (doesn't fix the design flaw)

### UX gaps still open:
- No explicit "using store from: X" message when discovery finds a parent repo store (could add for transparency)

---

## 4. What Changed (Summary)

| File/Component | Before | After | Confidence |
|----------------|--------|-------|------------|
| `pkg/pt/store.go:NewStoreClient` | Stored path as-is (relative) | Resolves to absolute with `filepath.Abs()` | High |
| `pkg/pt/client.go:doDiscoverParentStore` | Only checked cwd for store | Checks repo root, then parent repo if in worktree | High |
| `pkg/pt/types.go:ExecRunner` | No Dir field | `Dir` field sets working directory for commands | High |
| `pkg/pt/validate.go:ValidationRunner` | No WorkDir support | `WorkDir` wired to default ExecRunner when Runner is nil | High |
| `cmd/pt/worktree.go` | Git helpers used cwd | All git helpers accept explicit path, use `git -C` | High |
| `cmd/pt/main.go:validateCmd` | Used `os.Chdir` to worktree | Passes `workDir` to ValidationRunner | High |
| `cmd/pt/path_scenarios_test.go` | N/A (new file) | Integration tests for all path scenarios | High |

**Confidence key:**
- **High:** Proven by integration tests against real git repos and filesystems

---

## 5. Intent & Approach

### Problem being solved:

PT (projects-tasks) CLI had multiple bugs where operations would fail or write to wrong locations when the working directory changed. This affected:
1. Store operations after `os.Chdir` (relative paths became invalid)
2. Git commands in worktrees (commands ran against wrong repo)
3. DoD validation (tests ran in wrong directory)
4. Store discovery (couldn't find parent repo's store from worktree)

### Approach taken:

**Path-at-construction resolution:** All path-dependent objects resolve to absolute paths when created, not when used. This makes them immune to subsequent `os.Chdir` calls.

**Explicit path parameters:** Git helpers now accept an optional path parameter and use `git -C <path>` to operate on specific repos without changing cwd.

**Worktree-aware discovery:** Store discovery uses `git rev-parse --git-common-dir` to find the main repo when running from a worktree.

### Alternatives I rejected:
- **Subprocess isolation:** Run each operation in isolated subprocess with its own cwd. Too complex, performance overhead.
- **Global cwd mutex:** Lock all operations that depend on cwd. Doesn't fix the design flaw, just hides it.
- **Remove worktree support:** Would break a valuable workflow. Not acceptable.

### What's intentionally deferred:
- **Lock file path normalization:** Lock files are created adjacent to store, already works. No changes needed.
- **Store migration to `.pt/db.json`:** Discovery supports both locations, migration is separate concern.

---

## 6. Stub & Dummy Data Inventory

**No stubs or dummy data introduced.** This was a bug fix to existing functionality.

| Location | What's Stubbed | User-Visible Indicator | Real Replacement Task | Blocked On |
|----------|----------------|------------------------|----------------------|------------|
| N/A | N/A | N/A | N/A | N/A |

---

## 7. User Checkpoint Map

### Requires user approval before proceeding:
- [ ] None for this change (bug fix, not feature)

### Can be delegated to specialized agent:
- [ ] Code review: Verify git -C usage is correct across all helpers
- [ ] Test review: Verify integration tests cover edge cases

### No checkpoint needed (routine/mechanical):
- Running existing test suite (automated)
- Building the binary (automated)

---

## 8. Review Focus Areas

Guide the reviewer's attention (ordered by risk):

### 1. **Store path resolution (pkg/pt/store.go:19-31)**
   - Location: `NewStoreClient` function
   - What could go wrong: `filepath.Abs()` could fail on edge-case paths
   - How to verify: Check error handling, run `TestPathScenarios_StoreResolution`

### 2. **Worktree discovery (pkg/pt/client.go:99-118)**
   - Location: `doDiscoverParentStore`, `findMainRepoRoot`
   - What could go wrong: Incorrect parsing of `git rev-parse` output, false positive worktree detection
   - How to verify: Run `TestPathScenarios_DiscoveryFromWorktree`, manually test from actual worktree

### 3. **Git helpers with explicit path (cmd/pt/worktree.go)**
   - Location: `gitRepoRoot`, `gitIsDirtyAt`, `gitBranchExistsAt`, etc.
   - What could go wrong: Missing `-C` flag in some code path, inconsistent behavior between helpers
   - How to verify: Run `TestPathScenarios_GitHelpers`, grep for `exec.Command("git"` to find all git invocations

### 4. **Validation working directory (cmd/pt/main.go ~line 1580-1620)**
   - Location: `validateCmd` in main.go
   - What could go wrong: WorkDir not passed correctly, tests run in wrong directory
   - How to verify: Claim a task with worktree, run `pt validate`, confirm tests execute in worktree

### Questions the reviewer must answer:
- [ ] Are all `exec.Command("git", ...)` calls using `-C` when they should? (grep the codebase)
- [ ] Does `findMainRepoRoot` correctly handle non-worktree repos? (returns empty string)
- [ ] Is `filepath.Abs` error handling appropriate? (falls back to original path)
- [ ] Do integration tests clean up properly? (use `t.TempDir()`)

---

## 9. How to Validate

### Run integration tests (MUST pass against real dependencies):
```bash
# All tests
go test ./...

# Path-specific tests with verbose output
go test ./cmd/pt/... -run TestPathScenarios -v
```

### Run the spike proofs:
```bash
# Create a temp repo with worktree and verify discovery
tmpdir=$(mktemp -d)
cd "$tmpdir"
git init main && cd main
git commit --allow-empty -m "init"
echo '{}' > .pt.db.json
git worktree add -b feature ../worktree
cd ../worktree
# Should find ../main/.pt.db.json
PT_DEBUG=1 pt list  # (if debug logging exists)
```

### See the UX as user would:
```bash
# Normal workflow - unchanged
pt list
pt show pt-1
pt claim pt-1

# Worktree workflow - now works correctly
pt worktree start pt-1
cd /path/to/worktree
pt validate pt-1  # Should work from worktree
```

### Exercise the stubbed paths:
N/A - no stubs

---

## 10. Context & References

### Original problem analysis:
The following friction points were identified during SPY options tool development:

1. `gitIsDirty()` used cwd, failed after `os.Chdir`
2. PT_DB relative paths broke after directory change
3. Worktree validation wrote to wrong store
4. `os.Chdir` is process-wide, affects all goroutines
5. Store discovery didn't understand worktrees

### Files to review:
- `pkg/pt/store.go` - Store client with absolute path resolution
- `pkg/pt/client.go` - Discovery logic with worktree support
- `pkg/pt/types.go` - ExecRunner with Dir field
- `pkg/pt/validate.go` - ValidationRunner with WorkDir
- `cmd/pt/worktree.go` - Git helpers with explicit paths
- `cmd/pt/main.go` - Validate command using WorkDir
- `cmd/pt/path_scenarios_test.go` - Integration tests

### Related documentation:
- `docs/WORKFLOW-PRINCIPLES.md` - Risk-first development philosophy
- `docs/PT-TOOL-REFERENCE.md` - PT command reference
- `phases/pt-improvements.toml` - Task manifest for improvements

### Test output (2025-12-13):
```
=== RUN   TestPathScenarios_StoreResolution
--- PASS: TestPathScenarios_StoreResolution (0.00s)
=== RUN   TestPathScenarios_GitHelpers
--- PASS: TestPathScenarios_GitHelpers (0.05s)
=== RUN   TestPathScenarios_DiscoveryFromWorktree
--- PASS: TestPathScenarios_DiscoveryFromWorktree (0.05s)
PASS
ok  	projects-tasks/cmd/pt	0.253s
```
