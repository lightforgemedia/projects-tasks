# Example Project

A minimal example to dogfood `pt`:

- Simple CLI prints a greeting.
- Project-level DoD lives in `example-project/PROJECT_DOD.md`.
- Tasks are tracked via `phases/example_project.toml`.

## Run
```bash
go run ./example-project
```

Expected output:
```
example-project: hello from pt demo
```

## Test
```bash
go test ./example-project/...
```

## Done Criteria
See `example-project/PROJECT_DOD.md`.
