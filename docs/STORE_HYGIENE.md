# PT Store Hygiene (Cleaning Projects)

PT stores task state in a single JSON file (default: `.pt/db.json`, gitignored). To keep projects clean and avoid “mixed tracks”:

## Recommended defaults

- **One store per repo**: keep `.pt/db.json` at the repo root and let worktrees share it.
- **Archive before big changes**: always snapshot before bulk edits or resets.
- **Separate stores for subprojects (optional)**: if you truly need independent backlogs inside one git repo, set `PT_DB` per subproject.

## Archive the current store

```bash
mkdir -p .pt/archives
./pt snapshot --out ".pt/archives/$(date -u +%Y%m%d-%H%M%S).snap.json"
```

## Reset to a clean store (no tasks)

Create an empty store file and replace the current DB:

```bash
cat > /tmp/pt-empty-store.json <<'JSON'
{
  "next_id": 0,
  "issues": {},
  "labels": {},
  "deps": {},
  "comments": {},
  "title_map": {},
  "history": {},
  "blocked": {},
  "worktrees": {},
  "mocks": {}
}
JSON

./pt import --mode=replace --backup=true /tmp/pt-empty-store.json
```

After reset, `pt ready` will guide you to the project DoD (`PROJECT_DOD.md`) and prompt you to create tasks to reach it.

## Multiple projects at once

- Use separate DBs (one per project) and aggregate read-only views:
  - `./pt multi-ready --dbs=a/.pt/db.json,b/.pt/db.json --json`
- For a subproject store, set:
  - `export PT_DB="$PWD/.pt/db.json"`

