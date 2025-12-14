# Spike: CWD/Path Dependency Audit

## Summary

**Status: MOSTLY FIXED**

The path resolution issues identified in previous sessions have been addressed. The codebase is now largely CWD-independent for operations after initialization.

## Audit Results

### 1. os.Chdir Usage

| Location | Type | Status |
|----------|------|--------|
| `cmd/pt/path_scenarios_test.go:31,33` | Test | OK - tests subdir behavior |
| `cmd/pt/path_scenarios_test.go:220,222` | Test | OK - tests worktree behavior |
| `cmd/pt/hooks_test.go:160` | Test | OK - test setup |
| `cmd/pt/main.go:1324` | Comment | OK - documents NOT using os.Chdir |

**Finding:** No production code uses `os.Chdir`. Only tests use it to simulate running from different directories.

### 2. Git Helper Functions

| Function | Location | CWD-Dependent? | Status |
|----------|----------|----------------|--------|
| `gitRepoRoot(path)` | worktree.go:442 | Only if path="" | FIXED - uses `-C` flag |
| `gitIsDirtyAt(repoPath)` | worktree.go:458 | Only if path="" | FIXED - uses `-C` flag |
| `gitBranchExistsAt(repoPath, name)` | worktree.go:479 | Only if path="" | FIXED - uses `-C` flag |
| `gitWorktreeAddAt(...)` | worktree.go:501 | Only if path="" | FIXED - uses `-C` flag |
| `gitWorktreeRemoveAt(...)` | worktree.go:529 | Only if path="" | FIXED - uses `-C` flag |
| `gitBranchDeleteAt(...)` | worktree.go:567 | Only if path="" | FIXED - uses `-C` flag |

**Callers pass explicit paths:**
- Line 101: `gitIsDirtyAt(repoRoot)`
- Line 108: `gitBranchExistsAt(repoRoot, branchName)`
- Line 200, 209, 371, 381: All use `wtInfo.Path`

### 3. Store Path Construction

| Location | Behavior | Status |
|----------|----------|--------|
| `pkg/pt/store.go:55` | `filepath.Abs(path)` at construction | FIXED |

**Finding:** Store paths are resolved to absolute at construction time, making all subsequent operations CWD-independent.

### 4. Store Discovery (pkg/pt/client.go)

| Function | CWD-Dependent? | Intentional? |
|----------|----------------|--------------|
| `findGitRepoRoot()` | Yes | YES - discover from current context |
| `findMainRepoRoot()` | Yes | YES - discover from current context |
| `DiscoverStorePath()` | Yes | YES - cached once at startup |

**Finding:** Store discovery is intentionally CWD-dependent because:
1. We want `pt` commands to find the store for the "current project"
2. Discovery runs once at startup (cached via `sync.Once`)
3. Result is resolved to absolute path before any potential os.Chdir

### 5. os.Getwd Usage

| Location | Purpose | Status |
|----------|---------|--------|
| `cmd/pt/hooks.go:278` | Set hook working directory | OK - intentional |
| `cmd/pt/main.go:121` | Resolve PROJECT_DOD.md path | OK - startup only |

## Bug Reproduction

### Tested Scenarios

| Scenario | Command | Result |
|----------|---------|--------|
| Repo root | `./pt list` | PASS |
| Subdirectory `cmd/pt/` | `../../pt list` | PASS |
| Subdirectory `pkg/` | `../pt show pt-101` | PASS |

### Previously Reported Bug (FIXED)

**Issue:** `gitIsDirty()` used CWD instead of repo root, causing worktree commands to fail from subdirectories.

**Fix:** Commit `7590026` - Changed to `gitIsDirtyAt(repoRoot)` with explicit path.

## Remaining Items

### Low Priority (pt-103, pt-104)

The deprecated wrapper functions still exist but are unused:
- `gitIsDirty()` → delegates to `gitIsDirtyAt("")`
- `gitBranchExists()` → delegates to `gitBranchExistsAt("", name)`

These can be removed once all callers are verified to use the `*At` variants.

### Design Decision: pkg/pt/client.go

The store discovery functions (`findGitRepoRoot`, `findMainRepoRoot`) deliberately use CWD because:
1. They answer "which project am I in?"
2. Adding a path parameter would require callers to know the answer before asking
3. The pattern works: discover once at startup, cache the result

**Recommendation:** Leave as-is. Document that discovery is CWD-dependent by design.

## Acceptance Criteria Check

- [x] All cwd-dependent code identified
- [x] Bug reproduction documented (was fixed in prior commit)
- [x] Fix approach outlined (already implemented)

## Test Verification

```bash
$ go test ./cmd/pt/... -run TestPath -v
=== RUN   TestPathScenarios_StoreResolution
--- PASS: TestPathScenarios_StoreResolution (0.00s)
=== RUN   TestPathScenarios_GitHelpers
--- PASS: TestPathScenarios_GitHelpers (0.05s)
=== RUN   TestPathScenarios_DiscoveryFromWorktree
--- PASS: TestPathScenarios_DiscoveryFromWorktree (0.06s)
PASS
```
