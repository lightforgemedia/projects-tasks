# Git Commit Noise: `command not found: pulse`

**Date:** 2025-12-16  
**Scope:** Investigate intermittent `git commit` output like `zsh:6: command not found: pulse` while commits still succeed.

## Symptom
- During `git commit`, a message appears:
  - `zsh:6: command not found: pulse`
- The commit still completes successfully.

## What we verified in this repo
- Repo hooks present: `.git/hooks/pre-commit`, `.git/hooks/post-merge`.
- No local git configuration sets `core.hooksPath`.
- Could not reproduce the message with:
  - `GIT_TRACE=1 GIT_TRACE_HOOKS=1 git commit --allow-empty -m "trace-hook"`

## Why this can still happen
If the message occurs, it almost certainly comes from **a different hook/script path** than the ones currently in `.git/hooks/`:
- A **global hooks path** (`core.hooksPath`) set in another git config scope.
- A **tool-invoked hook** (e.g., a hook running a command that itself shells out to `zsh`).
- A **wrapper script** executed during commit that uses `#!/bin/zsh` and calls `pulse` at/around line 6.

## How to isolate (fast)
1. Identify which hooks actually ran:
   - `GIT_TRACE_HOOKS=1 git commit -m "x" 2>&1 | sed -n '1,120p'`
2. If it reproduces, capture full trace:
   - `GIT_TRACE=1 GIT_TRACE_SETUP=1 GIT_TRACE_HOOKS=1 git commit -m "x" 2>&1 | tee /tmp/git-trace.txt`
3. Confirm active hook paths across scopes:
   - `git config --show-origin --get core.hooksPath || true`
   - `git config --global --show-origin --get core.hooksPath || true`
   - `git config --system --show-origin --get core.hooksPath || true`
4. Enumerate non-sample hooks:
   - `ls -la .git/hooks | rg -v '\\.sample$'`
5. If a trace shows a `zsh` script being executed, open it and check line 6.

## If you want to silence it immediately
- Re-run the commit with hooks disabled to confirm it’s hook-related:
  - `git commit --no-verify -m "x"`
- If that removes the message, the next step is to identify which hook path is invoking `zsh` and update/remove it.
