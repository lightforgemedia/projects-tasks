# Design: Context Contracts for `projects-tasks`

## Problem Statement
Agents often fail because their input context is vague, incomplete, or stale. A task might say "Fix the bug", but without the specific file paths, reproduction steps, or success criteria, the agent hallucinates or applies fixes to the wrong place.

## Solution: Context Contracts
A **Context Contract** is a strict schema that defines exactly what information an agent needs *before* it starts working. It acts as a gatekeeper at phase boundaries.

### The Mental Model
```
[Manifest/Task] --> [Context Builder] --(payload)--> [Validator] --(verified context)--> [Agent]
                                                        ^
                                                        |
                                               [Contract Definition]
```

## 1. Contract Definition (TOML)
Contracts live in `contracts/<role>.toml`. They define the required structure of the context.

### Schema
- **Meta**: Versioning and target role.
- **Requirements**: List of mandatory fields.
- **Fields**: Type definitions (string, array, object), validation rules (regex, max_len), and semantic checks (allow_glob).
- **Freshenss**: Max age of provenance data.

## 2. Runtime Payload (JSON)
The actual data passed to the agent. It mirrors the contract structure but contains real values and a `provenance` section tracking where data came from.

### Core Sections
- **Goal**: What needs to be done (Prompt, User Story).
- **Scope**: Where to work (File paths, Repos, URLs).
- **Constraints**: What NOT to do (No breaking changes, specific linter rules).
- **Success**: How to verify (Test commands, success criteria).
- **Provenance**: Meta-data about the input (Source SHA, Timestamp, Origin).

## 3. Library: `pkg/contract`
A Go package to enforce these contracts.

**API:**
```go
type Contract struct { ... }
type Payload struct { ... }

func Load(path string) (*Contract, error)
func Validate(payload []byte, contract *Contract) error
func VerifyProvenance(payload *Payload) error // Checks file hashes/existence
```

## 4. CLI Integration
New commands for `pt` to manage context.

- `pt context init <task_id> --role=builder`: Generates a skeleton `context.json` based on the Task definition and the `builder.toml` contract.
- `pt context validate <context.json> --contract=builder`: Validates the payload. Used in CI or pre-run hooks.
- `pt context update <context.json> --key="scope.files" --value="..."`: Helper to safely modify context.

## 5. Workflow Integration

### Phase 1: Handoff to Builder (Coder)
1.  User/Architect creates a Task in `bd`.
2.  System runs `pt context init <id> --role=builder`.
3.  System fills `goal` from Task Description and `success` from DoD.
4.  System validates against `contracts/builder.toml`.
5.  **Gate**: If valid, Builder Agent starts. If invalid, it requests more info.

### Phase 2: Handoff to Reviewer
1.  Builder Agent finishes work.
2.  Builder updates `context.json` with `artifacts` (diffs, logs).
3.  System validates against `contracts/reviewer.toml`.
4.  **Gate**: If valid, Reviewer Agent starts.

---

## Example: Builder Contract (`contracts/builder.toml`)
```toml
[meta]
role = "builder"
version = "1.0"

[requirements]
must_have = ["goal.description", "scope.files", "success.tests"]

[fields.goal]
description = { type = "string", min_len = 10 }

[fields.scope]
files = { type = "array:string", allow_glob = false, must_exist = true }

[fields.success]
tests = { type = "array:string", min_len = 1 }
```
