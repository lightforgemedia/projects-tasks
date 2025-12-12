# PT Improvements Handoff - Review Request

## Overview

This handoff covers a set of improvements to the PT (Projects-Tasks) CLI focused on **agent-ready task quality** and **lower-friction SDLC flow** (Plan → Sync → Ready → Claim → Validate → Approve/Reject/Reopen), so work can continue with minimal “oral history”.

## Why These Changes

**Problem:** Tasks created via PT often lacked enough context for a “fresh” agent to start. The result was repeated clarifying questions like “where do I start?” / “what files should I touch?” and inconsistent acceptance criteria, which undermines task-driven development.

**Root causes identified:**
1. Task schema lacked fields for WHY/WHERE/BOUNDS context
2. No review gate before agents could claim tasks
3. `pt ready --verbose` output was cramped and hard to parse
4. `pt context prime` didn't show project-level context
5. No guidance for task authors on what makes a good task

## What Changed

### Schema Extensions (pkg/pt/manifest.go, pkg/pt/types.go)

**New Task fields:**
```toml
[[tasks]]
context = "WHY: problem being solved, motivation"
inputs = ["file1.go", "file2.go"]  # WHERE: files to modify
scope = "IN: what to do. OUT: what NOT to do"  # BOUNDS
reference = "https://..."  # RELATED: links to docs/issues
```

**New Manifest section:**
```toml
[project]
summary = "Brief description of what the project does"
structure = ["cmd/", "pkg/", "docs/"]  # Key directories
```

### Bug Fixes (pkg/pt/store.go, pkg/pt/transitioner.go)

1. **Release not clearing assignee:** Used "-" sentinel value instead of empty string
2. **History wrong transitions:** Capture old values before updating
3. **Validate in worktree:** Detect active worktree and chdir before running tests

### New Features (cmd/pt/main.go)

1. **Review task generation:** `pt sync --generate-reviews` creates blocking review tasks
2. **Task update flags:** `pt update <id> --context/--inputs/--scope/--reference`
3. **Help command:** `pt help task-authoring` with examples and checklist
4. **Improved ready output:** Multi-line block format in `--verbose` mode
5. **Context prime enhancement:** Shows project summary, enriched ready tasks

## Prime Contract (Next Work Anchor)

Priming output needs to be **complete-in-categories** and **honest about coverage**, so agents don’t infer “missing” functionality just because it wasn’t scanned. The current contract is captured in:

- `DESIGN_PRIME_OUTPUT.md` (text default, `--json` parity, DOT only for graphs; mandatory coverage + bounded rankings)

Planned integration target:

- `~/PROJECTS/treesitter-tools/` as the “capability layer” (granular discovery/query)
- `pt context prime` as the “guidance layer” (curation + next suggested commands)

## Files Changed

```
pkg/pt/manifest.go      - Added ProjectInfo struct, [project] parsing, handoff fields
pkg/pt/types.go         - Extended TaskMeta and UpdateOptions with handoff fields
pkg/pt/store.go         - Fixed UpdateIssue (assignee clear, history), added AddDependency
pkg/pt/transitioner.go  - Use "-" sentinel for release
cmd/pt/main.go          - All new features (help, ready format, context prime, etc.)
docs/REVIEW_TEMPLATE.md - Review checklist for task quality
```

## Test Coverage

New tests added:
- `TestAddDependency` - Verifies AddDependency method
- `TestSyncGeneratesReviews` - Verifies review task creation and deps
- `TestManifestProject` - Verifies [project] section parsing
- `TestContextPrimeAgent` - Verifies enhanced context prime output
- `TestHelpTaskAuthoring` - Verifies help command content
- `TestReleaseClearsAssignee` - Verifies "-" sentinel behavior
- `TestHistoryShowsCorrectTransitions` - Verifies history format
- `TestValidateWorktree` - Verifies worktree detection

All existing tests pass.

## Risks and Concerns

### Medium Risk

1. **Backward compatibility of "-" sentinel:** If any task has "-" as a legitimate assignee name, `release` will clear it. Unlikely but possible.

2. **Review task proliferation:** `--generate-reviews` creates N review tasks for N implementation tasks. Large manifests could become cluttered with review tasks.

3. **AddDependency on StoreClient only:** The `generateReviewTasks` function type-asserts to `*pt.StoreClient`. Won't work with other Client implementations (if any exist).

### Low Risk

4. **Context prime without manifest:** If `--manifest` not provided, project summary won't show. Users must remember to pass it.

5. **Verbose output assumes terminal:** Multi-line format may not render well in non-terminal contexts (CI logs, piped output).

6. **Help text hardcoded:** `pt help task-authoring` content is in Go source. Changes require rebuild.

## Review Checklist

### Functional Review
- [ ] Run `go test ./...` - all tests pass
- [ ] Test `pt sync --generate-reviews` on a sample manifest
- [ ] Test `pt release` clears assignee correctly
- [ ] Test `pt history` shows correct old->new format
- [ ] Test `pt validate` runs in worktree when present
- [ ] Test `pt ready --verbose` output is readable
- [ ] Test `pt context prime --manifest=...` shows project info
- [ ] Test `pt help task-authoring` is helpful
 - [ ] Read `DESIGN_PRIME_OUTPUT.md` and confirm it matches current `pt context prime` output (text + `--json`)

### Code Review
- [ ] Check TaskMeta/UpdateOptions field additions are consistent
- [ ] Check "-" sentinel handling in UpdateIssue is safe
- [ ] Check generateReviewTasks dependency wiring is correct
- [ ] Check issueRole fallback to labels is appropriate
- [ ] Review help text for accuracy and completeness

### Integration Review
- [ ] Verify existing manifests still sync correctly
- [ ] Verify existing workflows (claim/validate/approve) unchanged
- [ ] Check JSON output formats include new fields

## Next Steps (Recommended)

1. Treesitter-backed priming: augment `pt context prime` with coverage/inventory that includes **all files**, not only parseable ones (see `DESIGN_PRIME_OUTPUT.md`).
2. Add bounded modes: `pt context prime --summary` (structure+coverage only) vs `--full` (rankings), plus time/file budgets that set `partial=true`.
3. Hook integration: optional pre/post hooks for prime/lint that clearly print “what ran + result” and block when configured to fail.

## Demo Commands

```bash
# Build fresh
go build -o pt ./cmd/pt

# Test help
./pt help task-authoring

# Test ready verbose
./pt ready --db=/tmp/dotfiles-cli/.pt.db.json --verbose

# Test context prime
./pt context prime --db=/tmp/dotfiles-cli/.pt.db.json --manifest=/tmp/dotfiles-cli/phases/pt-improvements.toml

# Test sync with reviews
./pt sync --db=/tmp/test.db.json --generate-reviews phases/example_project.toml

# Test release clears assignee
./pt claim --db=/tmp/test.db.json --as=bob pt-1
./pt release --db=/tmp/test.db.json pt-1
./pt show --db=/tmp/test.db.json pt-1  # assignee should be empty
```

## Rollback Plan

If issues found:
1. Revert commits touching pkg/pt/store.go, pkg/pt/types.go, pkg/pt/manifest.go
2. Revert cmd/pt/main.go changes (or selectively keep non-breaking ones)
3. Existing data is forward-compatible; new fields are optional/omitempty

## Questions for Reviewer

1. Is the "-" sentinel for clearing assignee the right approach? Alternative: explicit `ClearAssignee` method.

2. Should review tasks be opt-in (current) or default behavior? Current: `--generate-reviews` flag.

3. Is the verbose output format good? Could add `--format=compact|block|json`.

4. Should `pt help` support more topics? Currently only `task-authoring`.

5. Should project metadata be stored in the PT store instead of requiring `--manifest`?
