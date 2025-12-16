# Context Contracts

Context Contracts make agent handoffs **explicit and verifiable**. Instead of starting from a vague task description, an agent produces a JSON “context payload” and validates it against a role-specific contract.

## Files

- Contracts: `contracts/*.toml` (example: `contracts/builder.toml`, `contracts/reviewer.toml`)
- Design notes: `DESIGN_CONTRACTS.md`

## Why

- Prevent “skipping ahead” by requiring the minimum inputs to be present.
- Make handoffs auditable (what was provided, when, and from where).
- Reduce drift: the payload becomes a stable, machine-checkable contract between orchestration and work.

## CLI usage

Generate a scaffold from a task (recommended for no-context starts):

```bash
pt context init <task-id> --role=builder > context.builder.json
```

Fill in missing details (especially `scope.files`) and validate:

```bash
pt context validate context.builder.json
```

Notes:
- `pt context validate` infers the contract from `role` (or `meta.role`) in the payload. Use `--contract=PATH` to override.
- Task roles like `backend-dev` describe who should do the work; Context Contract roles like `builder`/`reviewer` describe the **handoff schema**.

## Payload shape (minimal)

The `builder` contract currently expects:
- `goal.prompt` (string)
- `scope.files` (array of file paths; must exist once populated)
- `success.criteria` (array of strings)
- `provenance.inputs` + `provenance.issued_at` (freshness-checked)

## Reviewer handoff

After implementation, create a reviewer payload (example fields):
- `goal.original_prompt`
- `artifacts.diff`, `artifacts.test_output`
- `provenance.builder`

Validate it with:

```bash
pt context validate context.reviewer.json --contract=contracts/reviewer.toml
```

