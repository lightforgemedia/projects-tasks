# Repository Guidelines for Agents

## Project Structure
- **Root**: Documentation and config.
- **`pkg/pt`**: The Go SDK. Core logic lives here.
- **`cmd/pt`**: The CLI entrypoint.
- **`phases/`**: Example or active manifests.

## Development Standards
- **Go**: Use standard formatting (`gofmt`).
- **Tests**: Unit tests next to code (`_test.go`). No skipping.
- **Secrets**: Never commit secrets.

## Code Navigation (sg-agent default)
- Start orientation with: `sg-agent bundle --dir . --format json`
- Prefer contract-first reading:
  - `sg-agent read --file <path> --name <Sym> --depth signature --format sigil`
  - Escalate to bodies only when needed: `--depth full --format json`
- Prefer AST-aware navigation over grep:
  - Call graph: `sg-agent find --callers <Sym> --dir . --depth 2 --format json`
  - Callees: `sg-agent find --callees <Sym> --dir . --depth 2 --format sigil`
  - Type usages: `sg-agent find --usages <Type> --dir . --depth 2 --format json`
- Use `rg` only when sg-agent can’t help (docs/config/git hooks/plaintext string search).

## PT Dogfooding (mandatory)
- All work in this repo and subprojects must be managed via `pt` (sync/ready/claim/validate/approve). Do not bypass the SDLC flow.
- Log any pt issues or improvement ideas as pt tasks (via manifests or `pt add`).
- Keep task-level and project-level DoDs current (e.g., `PROJECT_DOD.md`); use pt review/sign-off tasks to enforce them.
- For new features/servers/CLIs, DoD must include real validation (run the feature end-to-end, not just unit tests). If a manual/standalone check is needed (e.g., start the server and call a tool), perform it and record the result in task comments.
- If you believe PT itself is incorrect or misleading, STOP project work and file a bug report via `pt submit-bug` (the tool is the workflow rail).

## Task Test Planning (mandatory)
- Every task (and each phase) must spell out an explicit test plan in the DoD: which commands to run (unit, integration, E2E), what to validate manually, and what evidence to capture (logs/output).
- When creating manifests (`phases/*.toml`), include specific test commands in `tests` and a concise manual checklist in `manual` (not just “go test ./...”).
- During validate/review, confirm the test plan was executed; if not, add a follow-up task before approving.

## Agent Workflows
Agents interacting with this repository or using the `pt` tool should follow this loop:

0.  **Conductor** (preferred): `pt next [--json]` to get the current best action (review → work → unblock → plan → done).
1.  **Discovery**: `pt ready --role=<role> --verbose` to find unblocked work and see blockers/assignee. Use `--sort` for ordering (use `--phase`/`--all-phases` when a workflow is active).
    * Reviewers: `pt list --status=needs_review --role=<role>` to find items awaiting review.
2.  **Claim**: `pt claim <id> [--as=$USER] [--draft] [--override-soft=REASON]` to lock and assign (status → `in_progress`); gates are enforced when workflows are active.
3.  **Execution**:
    *   Read task/context via `pt context init <id>` or the issue text.
    *   Prefer Context Contracts for no-context starts: `pt context init <id> --role=builder > context.builder.json` then `pt context validate context.builder.json` before coding.
    *   Use `pt show <id>` to view DoD/comments; `pt comment <id> "note"` to log progress or blockers.
    *   Implement changes.
    *   If DoD includes `manual` steps, only use `pt validate --yes <id>` after performing them; confirmations are recorded in the review comment.
4.  **Verification**: `pt validate <id>` (or `--yes` to auto-confirm manual steps).
    *   **Pass**: status → `needs_review`; manual notes stored.
    *   **Fail**: fix and retry, or `pt release <id>` to unblock others.
5.  **Review** (as reviewer):
    *   Inspect via `pt ready --role=<role> --verbose` or store filters.
    *   `pt approve <id>` to close, or `pt reject <id> --reason="..."` to send back.

## Multi-Agent Guidance
- Identity: always set `--as` (or ensure `$USER` is correct) when claiming so ownership is auditable; `pt claim` fails if identity is empty.
- Respect blockers: do not claim blocked work; resolve deps first or choose another task.
- Manual blockers: use `pt blocked <id> <reason>` and `pt unblock <id>` (so `pt next` won’t recommend blocked work).
- No-context starts: run `pt context init <id>` to bootstrap requirements; read DoD before coding.
- Context Contracts: see `docs/CONTEXT_CONTRACTS.md` for the recommended builder/reviewer payload flow.
- Priming output: use `pt context prime [--json]` for a 10,000‑foot view; follow `DESIGN_PRIME_OUTPUT.md` when adding priming/discovery features (must report coverage; rankings must say “top N of M”; include stable `path:line` refs).
- Staleness: if paused or stuck, `pt release <id>` and leave a brief comment so others can continue.
- Next hints: manifests can include `next_hint`; `pt ready --verbose` will surface suggested follow-ups.
- Safety: use `pt snapshot` before risky changes if you want a quick backup of the store.

## Task Creation (for maintainers)
- Use manifests in `phases/`; each task should include `title`, `role`, `template`, `deps`, and DoD (`tests`, `manual`).
- Pick templates: APIs (`backend_endpoint`), UI (`frontend_component`), regressions (`bug_fix`), cleanup (`refactor`), schema (`migration`), SLO/alerts (`observability_hook`).
- Keep titles unique; reference deps by title or ID. Add concise manual steps so `pt validate --yes` can capture confirmations.
- Ad-hoc: use `pt add` for quick tasks; `pt comment` to log progress; `pt snapshot` to back up the store before risky changes. Multi-project helpers: `pt multi-ready` (read-only across DBs) and `pt propose` (diff only, no writes).
- Search/read: `pt search --query="text"` to find tasks by title/labels/description; `pt list`/`pt show` to inspect status/DoD/comments; `--db`/`--prefix` to target specific stores.

### Task Authoring Checklist (self-contained)
- Assume the next agent has zero context; every task must stand alone.
- Include artifact/spec link and affected files/modules.
- Spell out exact commands to run (tests, validation) and manual steps.
- Add acceptance criteria bullets (evidence/proof required).
- Note dependencies and next likely task if relevant.
- For bugs: capture repro steps/symptoms. For features: link UI/API contracts.
- If you stop early, leave a status comment so others can continue.

## Feedback Capture (reviews)
- Use `pt feedback --desc="<short-slug>"` to create a feedback note template under `~/.pt/feedback/<datetime>.<slug>.feedback.md` (override dir with `--dir` or `PT_FEEDBACK_DIR`).
- Template sections (keep concise):
  - Summary: one paragraph of what was reviewed and the verdict.
  - Findings: bullets by severity (Critical/Major/Minor) with file refs (path:line) and any data-model notes.
  - Gaps/Risks: short bullets (auth, wiring, build integration, tests, etc.).
  - Recommendations: numbered next actions.
  - Status: Ready / Blocked / Needs follow-up.

## PT Bug Reports (when the tool is wrong)
- If PT is behaving incorrectly (wrong gating, corrupted store, misleading `pt next`, etc.), STOP and file a bug report:
  - `pt submit-bug --label="short-slug" --description="..." --found_in="cmd/path/version" --repro="exact steps"`
- This writes a markdown report to `~/.pt/bugs/<YYYY-MM-DD>/bug-<label>.md` (override base dir with `--dir` or `PT_BUG_DIR`).

## Automation Hooks (planned)
- Configure hooks in repo `hooks.toml` or global `$HOME/.config/pt/hooks.toml` (env `PT_HOOKS` overrides). Events: pre/post sync/claim/validate, post release/approve/reject.
- Hooks receive env: `PT_EVENT`, `PT_ID`, `PT_TITLE`, `PT_ASSIGNEE`, `PT_ACTOR`, `PT_STATUS_FROM/TO`, `PT_ROLE`, `PT_DOD` (JSON); full payload arrives via stdin. Enable hook logs with `--hook-verbose` or `PT_HOOK_VERBOSE=1`. Bypass with `PT_SKIP_HOOKS=1`.
- Use cases: pre-claim policy/WIP checks; post-validate notify; post-approve deploy trigger. Hook outputs and command results are available via `--json`.
- Safety: hooks are shell commands. Review them before running; set timeouts/on_fail to avoid hanging flows.
- Multiple projects: keep separate working dirs or `PT_DB` per project; see `DESIGN_MULTI_PROJECT.md` for the proposed cross-project commands.
