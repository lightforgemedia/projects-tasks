# Workflow V2 Plan: Statechart Engine Migration

## Why V2

PT has grown into “a system of flows”: task lifecycle, phases, gates, checkpoints, discovery, review rails, demo. Today those behaviors are split across multiple commands (`pt next`, `pt workflow status`, `pt claim`, `pt validate`, hooks), which makes drift and skipped steps more likely.

V2 goal: **one engine** computes:
- what transitions are allowed (enforcement)
- what should happen next (guidance)
- what evidence is required (rails)

…from declarative workflow data.

## Non-Goals (V2)
- Rewrite storage format.
- Remove existing workflow TOML.
- Break existing CLI UX.

## Current Model (V1)

```
              ┌───────────────┐
              │ workflow.toml  │  phases + gates
              └──────┬────────┘
                     │
┌──────────────┐     ▼     ┌─────────────────────┐
│ pt claim/... │  scattered │ pt next/status/...  │
│ enforcement  │  logic in  │ recommendations     │
└──────────────┘  commands  └─────────────────────┘
                     │
                     ▼
              ┌───────────────┐
              │ store (.pt/db)│
              └───────────────┘
```

## Target Model (V2): Statechart Engine

Treat workflow as a **statechart**:
- **Events**: `claim`, `validate`, `approve`, `reject`, `reopen`, `review_written`, `demo_captured`
- **Guards**: conditions/gates (deps done, phase satisfied, checkpoint satisfied)
- **Actions**: add labels, write comments, generate recommendations, run hooks
- **Hierarchical states**: task lifecycle + workflow phase + rails (orthogonal regions)

```
┌─────────────────────────────── Engine (statechart) ───────────────────────────────┐
│ Region A: Task State       Region B: Workflow Phase      Region C: Rails/Demo      │
│ open→in_progress→...       prove→explore→...→signoff     none→kickoff→closeout→demo│
│           ▲                         ▲                           ▲                  │
│           └──────────── shared guards/actions/recommendations ───┘                  │
└────────────────────────────────────────────────────────────────────────────────────┘
```

## Workflow IR (Internal Representation)

V2 introduces a small internal IR compiled from existing `workflows/*.toml` (no user-facing changes required).

### IR Types (sketch)

```go
// pkg/pt/engine/ir.go
type IR struct {
  WorkflowName string
  Phases []PhaseIR
  // Optional: rail definitions (kickoff/closeout/demo)
}

type PhaseIR struct {
  ID string
  Order int
  Gate GateIR
  Checkpoints []CheckpointIR
  Proof *ProofIR
}

type GateIR struct {
  Type string        // soft|hard
  Expr GateExpr      // parsed AST for existing grammar
  ReminderBefore string
  ReminderAfter  string
  BlockMessage   string
}

type Event string // claim|validate|approve|...

type Recommendation struct {
  Mode string            // REVIEW|WORK|UNBLOCK|PLAN|DONE
  Why []string
  Commands []string
  Evidence []string      // demo paths, review links, etc
}
```

### Gate Expression Compatibility

Compile the existing grammar (already in `pkg/pt/workflow.go`) into an AST so:
- `pt claim` enforcement and `pt next` guidance share the same evaluation.
- Soft override rules are centralized.

## Extension Points (Custom Workflows)

Keep workflows as data (TOML), but allow controlled extensibility:

1) **Built-in guards** (default): deps, `phase:X complete`, `has_comment:tag`, discovery approvals, checkpoint completion.
2) **Exec-based guards** (optional): a gate can call an external command and interpret pass/fail + message.
   - This keeps PT “unix-y” without embedding plugins.
3) **Actions via hooks** (existing): `pre-claim`, `post-validate`, etc.

The engine should never require external dependencies by default; exec guards/hooks are opt-in.

## Migration Strategy (Lowest Risk)

### Step 0: Feature Flag + Conformance Harness
- Add `PT_ENGINE=v2` (default remains v1).
- Add conformance tests (golden fixtures) for key outputs:
  - `pt next --json` on representative stores
  - `pt workflow status --json`
  - gate enforcement on `pt claim` (hard/soft/override)

### Step 1: Replace `pt workflow status` backend (read-only)
- Route `cmd/pt/workflow.go` computation to engine under flag.
- CLI output stays the same; only computation changes.

### Step 2: Replace `pt next` backend (guidance)
- Route `cmd/pt/next.go` selection + “why” to engine under flag.
- Keep `--skip-checkpoints` escape hatch.

### Step 3: Replace enforcement (claim/validate/approve)
- Centralize “can I do this?” checks in the engine so commands don’t drift.

### Step 4: Promote V2 to default
- After dogfooding + conformance stability, flip default engine to v2.

## Rails + Demo Integration

Rails are deliberately **non-blocking** by default:
- kickoff/closeout/demo tasks exist and are recommended at the right time.
- user involvement is optional until signoff/demo checkpoints.

Demo conventions live in `docs/DEMO_DESIGN.md`. V2 should treat demo readiness as first-class evidence at signoff.

## Testing Plan (Must-Haves)

1) **IR compile tests**: workflow TOML → IR equality (table-driven).
2) **Gate eval tests**: existing grammar cases, including OR conditions and soft override behavior.
3) **Recommendation tests**: given a store snapshot, engine returns deterministic `Recommendation`.
4) **Conformance tests**: run both engines against the same fixtures until parity.

## Adoption Plan for New Projects

Short term:
- keep `workflows/*.toml` and `pt sync --generate-phase-reviews`.

Long term:
- `pt init` can scaffold:
  - `.pt/demo/` (see `docs/DEMO_DESIGN.md`)
  - `PROJECT_DOD.md`
  - `workflows/<template>.toml`
  - optional hook defaults

