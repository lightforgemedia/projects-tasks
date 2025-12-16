# PT Quick Start (New Project)

This guide gets a brand-new repo from “nothing” → “tasks flowing” in ~5 minutes.

## 1) Build `pt` (repo-local)

From the repo root:

```bash
go build -o pt ./cmd/pt
```

`pt` stores state in `.pt/db.json` by default (gitignored). To override:

```bash
export PT_DB=/path/to/custom.db.json
```

## 2) Add a Project Definition of Done (DoD)

Create `PROJECT_DOD.md` (or set `PT_PROJECT_DOD`).

Example checklist:

- [ ] `go test ./...` passes
- [ ] Key user flows manually verified
- [ ] Docs updated (README/usage)

When there are no ready tasks, `pt ready` will prompt you to review this DoD and create follow-up tasks if it isn’t satisfied.

## 3) Create tasks (manifest-first)

Create `phases/init.toml`:

```toml
[[tasks]]
title = "Spike: validate external API access"
template = "spike"
role = "backend-dev"
artifact = "doc:spike-api"
[tasks.dod]
tests = ["echo ok"]
manual = "Prove access with a real request; save response in outputs/spike-api/response.json"
criteria = ["real data captured", "rate limits noted"]
```

Sync tasks into the store:

```bash
./pt sync phases/init.toml
```

Ad-hoc task (no manifest):

```bash
./pt add "Fix flaky test" --role backend-dev --template bug_fix --artifact code:pkg/foo/bar.go \
  --tests "go test ./..." --manual "Re-run the failing test 5x" --criteria "No flakes" --no-handoff-seed
```

## 4) Run the SDLC loop

```bash
./pt ready --verbose
./pt claim pt-1 --as agent
# …do the work…
./pt validate pt-1 --yes
./pt approve pt-1
```

To pick the next best action (the “conductor”):

```bash
./pt next
```

## 5) Returning with zero context

```bash
./pt context prime
./pt ready --verbose
./pt show pt-<id>
```

