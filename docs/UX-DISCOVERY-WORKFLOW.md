# UX Discovery Workflow

A structured process for exploring and validating user experience before building.

---

## Problem Statement

Current workflow treats UX as a checkbox:
- "manual: Verify it displays readable in terminal"
- Single implementation presented (no alternatives)
- Feedback is pass/fail, not iterative
- No grounding in actual user needs

This leads to:
- Building the first idea that comes to mind
- Rework when users say "not quite what I meant"
- Features that technically work but miss the point

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| When to require UX loop | Any task with `ux` field | Explicit opt-in, not tied to template name |
| Use cases first | Required before options | Options without use cases are just aesthetics |
| Labeled choices | [A], [B], [C] format | Easy to reference, combine ("A+C"), discuss |
| Iteration budget | Default 3, configurable | Prevents infinite loops, forces escalation |
| UX types | cli, tui, web, api, doc | Different artifacts per type |
| Combination allowed | Yes, e.g. "A+C" | Real solutions often merge approaches |

---

## Full Task Lifecycle (with UX Loop)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                           PT TASK LIFECYCLE (with UX Loop)                               │
└─────────────────────────────────────────────────────────────────────────────────────────┘

    ┌──────────┐
    │  SYNC    │  pt sync manifest.toml
    └────┬─────┘
         │
         ▼
    ┌──────────┐
    │  CLAIM   │  pt claim pt-5
    └────┬─────┘
         │
         ▼
    ┌──────────────────────────────────────────────────────────────────────────────────┐
    │                                                                                   │
    │   ╔═══════════════════════════════════════════════════════════════════════════╗  │
    │   ║                        UX DISCOVERY LOOP                                   ║  │
    │   ║  (required when task has ux field)                                         ║  │
    │   ╚═══════════════════════════════════════════════════════════════════════════╝  │
    │                                                                                   │
    │   ┌─────────────────────────────────────────────────────────────────────────┐    │
    │   │ STEP 1: USE CASES                                                        │    │
    │   │ pt ux-cases pt-5                                                         │    │
    │   ├─────────────────────────────────────────────────────────────────────────┤    │
    │   │                                                                          │    │
    │   │  Agent proposes:                                                         │    │
    │   │  ┌────────────────────────────────────────────────────────────────────┐ │    │
    │   │  │ [UC-1] Quick scan for entry                                        │ │    │
    │   │  │   Actor: Trader with thesis                                        │ │    │
    │   │  │   Goal:  Find strike with target delta (.30-.40)                   │ │    │
    │   │  │                                                                    │ │    │
    │   │  │ [UC-2] Compare premium across strikes                              │ │    │
    │   │  │   Actor: Income seller                                             │ │    │
    │   │  │   Goal:  Maximize premium for risk budget                          │ │    │
    │   │  │                                                                    │ │    │
    │   │  │ [UC-3] Assess liquidity before large order                         │ │    │
    │   │  │   Actor: Size trader                                               │ │    │
    │   │  │   Goal:  Check OI and spread before filling 50+ contracts          │ │    │
    │   │  └────────────────────────────────────────────────────────────────────┘ │    │
    │   │                                                                          │    │
    │   │  User: [Y] confirm  [E] edit  [A] add more  [?] help                    │    │
    │   │                                     │                                    │    │
    │   └─────────────────────────────────────┼────────────────────────────────────┘    │
    │                                         │                                         │
    │                                         ▼                                         │
    │   ┌─────────────────────────────────────────────────────────────────────────┐    │
    │   │ STEP 2: UX OPTIONS                                                       │    │
    │   │ pt ux-explore pt-5                                                       │    │
    │   ├─────────────────────────────────────────────────────────────────────────┤    │
    │   │                                                                          │    │
    │   │  Agent generates options (each mapped to use cases):                     │    │
    │   │                                                                          │    │
    │   │  [A] Delta-focused layout ─────────────────────────── optimizes: UC-1   │    │
    │   │  ┌────────────────────────────────────────────────────────────────────┐ │    │
    │   │  │    Strike     Δ      Bid/Ask                                       │ │    │
    │   │  │  → $681     .55    2.66/2.69    ← arrow marks target range         │ │    │
    │   │  │    $682     .47    2.04/2.07                                       │ │    │
    │   │  └────────────────────────────────────────────────────────────────────┘ │    │
    │   │                                                                          │    │
    │   │  [B] Premium-focused layout ───────────────────────── optimizes: UC-2   │    │
    │   │  ┌────────────────────────────────────────────────────────────────────┐ │    │
    │   │  │    Strike    Bid     %OTM    RoR     ← return on risk calc         │ │    │
    │   │  │    $675     0.33    2.1%    0.8%                                   │ │    │
    │   │  └────────────────────────────────────────────────────────────────────┘ │    │
    │   │                                                                          │    │
    │   │  [C] Liquidity-focused layout ─────────────────────── optimizes: UC-3   │    │
    │   │  ┌────────────────────────────────────────────────────────────────────┐ │    │
    │   │  │    Strike     OI    Spread    Vol    ← spread as % of mid          │ │    │
    │   │  │    $680      752    $0.05    1.5%    [████░] liquidity score       │ │    │
    │   │  └────────────────────────────────────────────────────────────────────┘ │    │
    │   │                                                                          │    │
    │   │  User: [A] [B] [C] [A+C] combine  [?] "I need..."                        │    │
    │   │                                     │                                    │    │
    │   └─────────────────────────────────────┼────────────────────────────────────┘    │
    │                                         │                                         │
    │                         ┌───────────────┴───────────────┐                         │
    │                         ▼                               ▼                         │
    │                  ┌────────────┐                  ┌────────────┐                   │
    │                  │  SELECTED  │                  │  ITERATE   │                   │
    │                  │  pt ux-    │                  │  (refine)  │                   │
    │                  │  select    │                  │     │      │                   │
    │                  │  pt-5 A+C  │                  │     └──────┼─── loops back     │
    │                  └─────┬──────┘                  └────────────┘    to STEP 2      │
    │                        │                               ▲                          │
    │                        │                               │                          │
    │                        │         max 3 iterations ─────┘                          │
    │                        │         then escalate                                    │
    │                        ▼                                                          │
    │   ┌─────────────────────────────────────────────────────────────────────────┐    │
    │   │ STEP 3: UX APPROVED                                                      │    │
    │   ├─────────────────────────────────────────────────────────────────────────┤    │
    │   │                                                                          │    │
    │   │  Recorded in task:                                                       │    │
    │   │  • use_cases: [UC-1, UC-2, UC-3]                                        │    │
    │   │  • ux_selection: "A+C"                                                  │    │
    │   │  • ux_notes: "need delta scan and liquidity together"                   │    │
    │   │  • ux_iterations: 1                                                     │    │
    │   │                                                                          │    │
    │   │  DoD updated:                                                            │    │
    │   │  • criteria += ["UC-1 supported", "UC-2 supported", "UC-3 supported"]   │    │
    │   │                                                                          │    │
    │   └─────────────────────────────────────────────────────────────────────────┘    │
    │                                                                                   │
    └───────────────────────────────────────────────────────────────────────────────────┘
                                            │
                                            ▼
    ┌──────────────────────────────────────────────────────────────────────────────────┐
    │                                                                                   │
    │   ╔═══════════════════════════════════════════════════════════════════════════╗  │
    │   ║                           BUILD PHASE                                      ║  │
    │   ╚═══════════════════════════════════════════════════════════════════════════╝  │
    │                                                                                   │
    │   Agent implements based on:                                                      │
    │   • Selected UX layout (A+C merged)                                              │
    │   • Use cases as acceptance criteria                                             │
    │   • Task inputs/scope from manifest                                              │
    │                                                                                   │
    └───────────────────────────────────────────────────────────────────────────────────┘
                                            │
                                            ▼
    ┌──────────────────────────────────────────────────────────────────────────────────┐
    │                                                                                   │
    │   ╔═══════════════════════════════════════════════════════════════════════════╗  │
    │   ║                          VALIDATE PHASE                                    ║  │
    │   ║  pt validate pt-5                                                          ║  │
    │   ╚═══════════════════════════════════════════════════════════════════════════╝  │
    │                                                                                   │
    │   ┌─────────────────────────────────────────────────────────────────────────┐    │
    │   │ Automated tests:                                                         │    │
    │   │   $ go build ./cmd/sot && ./sot chain SPY                               │    │
    │   │   ✓ PASS                                                                 │    │
    │   └─────────────────────────────────────────────────────────────────────────┘    │
    │                                                                                   │
    │   ┌─────────────────────────────────────────────────────────────────────────┐    │
    │   │ Manual verification (against use cases):                                 │    │
    │   │                                                                          │    │
    │   │   [UC-1] Can locate .30-.40 delta in <5 seconds?     [y/n]: y           │    │
    │   │   [UC-2] Premium comparison visible at a glance?     [y/n]: y           │    │
    │   │   [UC-3] Liquidity (OI, spread) shown?               [y/n]: y           │    │
    │   │                                                                          │    │
    │   │   All criteria met ✓                                                     │    │
    │   └─────────────────────────────────────────────────────────────────────────┘    │
    │                                                                                   │
    └───────────────────────────────────────────────────────────────────────────────────┘
                                            │
                                            ▼
    ┌──────────┐
    │ APPROVE  │  pt approve pt-5
    └────┬─────┘
         │
         ▼
    ┌──────────┐
    │  DONE    │  status=closed
    └──────────┘
```

---

## UX Types

The `ux.type` field determines what artifacts are produced during exploration:

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                    UX TYPES                                              │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  type: cli                          type: tui                         type: web         │
│  ─────────────                      ─────────                         ─────────         │
│  ┌──────────────────┐               ┌──────────────────┐             ┌──────────────┐   │
│  │ $ sot chain SPY  │               │ ┌─────┬────────┐ │             │ ┌──────────┐ │   │
│  │ Strike  Bid  Ask │               │ │ Nav │ Chain  │ │             │ │  [Card]  │ │   │
│  │ $680   3.33 3.38 │               │ ├─────┼────────┤ │             │ │  $680    │ │   │
│  │ $681   2.66 2.69 │               │ │ Pos │ ██████ │ │             │ │  Δ: .62  │ │   │
│  └──────────────────┘               │ │ Log │        │ │             │ └──────────┘ │   │
│                                      │ └─────┴────────┘ │             │              │   │
│  Artifacts:                          └──────────────────┘             └──────────────┘   │
│  • Sample terminal output                                                                │
│  • --help text                       Artifacts:                       Artifacts:         │
│  • Flag combinations                 • ASCII wireframe                • HTML mockup      │
│  • Error message examples            • Key bindings table             • Component tree   │
│                                      • Navigation flow                • Responsive views │
│                                      • State diagram                  • Interaction flow │
│                                                                                          │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  type: api                          type: doc                         type: error       │
│  ─────────                          ─────────                         ───────────       │
│  ┌──────────────────┐               ┌──────────────────┐             ┌──────────────┐   │
│  │ POST /orders     │               │ # Getting Started│             │ Error: EFOO  │   │
│  │ {                │               │                  │             │              │   │
│  │   "symbol": "SPY"│               │ 1. Install...    │             │ What: ...    │   │
│  │   "qty": 5       │               │ 2. Configure...  │             │ Why: ...     │   │
│  │ }                │               │ 3. Run...        │             │ Fix: ...     │   │
│  └──────────────────┘               └──────────────────┘             └──────────────┘   │
│                                                                                          │
│  Artifacts:                          Artifacts:                       Artifacts:         │
│  • Request/response examples         • Outline structure              • Error catalog    │
│  • Error response shapes             • Section samples                • Recovery steps   │
│  • SDK usage examples                • Code snippet style             • User guidance    │
│                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Task Schema

### Manifest Definition

```toml
[[tasks]]
title = "Chain Display"
role = "dev"
artifact = "code:cmd/sot/chain.go"
context = "Users need to view SPY options chain with key data"
inputs = ["cmd/sot/chain.go", "pkg/tradier/options.go"]
scope = "IN: display chain in terminal. OUT: no order placement"

[tasks.ux]
type = "cli"                          # cli | tui | web | api | doc | error
options_min = 2                       # Must present at least N options
iteration_max = 3                     # Max refinement rounds before escalate

[tasks.dod]
tests = ["go build ./cmd/sot && ./sot chain SPY --help"]
manual = "Verify use cases are supported"
criteria = ["Chain displays", "Data is readable"]
# Note: criteria are auto-extended with use cases after ux-select
```

### Use Cases (discovered during ux-cases)

```toml
# Stored in task metadata after pt ux-cases
[[tasks.ux.use_cases]]
id = "UC-1"
actor = "Trader with thesis"
goal = "Find strike with target delta (.30-.40)"
context = "Know direction, need specific contract fast"

[[tasks.ux.use_cases]]
id = "UC-2"
actor = "Income seller"
goal = "Maximize premium for acceptable risk"
context = "Selling puts at support, comparing strikes"

[[tasks.ux.use_cases]]
id = "UC-3"
actor = "Size trader"
goal = "Assess liquidity before large order"
context = "Need to fill 50+ contracts without slippage"
```

### UX Selection (recorded after ux-select)

```toml
# Stored in task metadata after pt ux-select
[tasks.ux.selection]
choice = "A+C"
note = "Need both delta scan and liquidity info"
iterations = 1
approved_at = "2025-12-13T10:30:00Z"
```

---

## Commands

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                  PT UX COMMANDS                                          │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  pt ux-cases <id>                    Propose/confirm use cases for task                 │
│    --add "description"                 Add a use case interactively                      │
│    --confirm                           Accept proposed use cases                         │
│                                                                                          │
│  pt ux-explore <id>                  Generate UX options mapped to use cases            │
│    --regenerate                        Generate new options (counts as iteration)        │
│                                                                                          │
│  pt ux-select <id> <choice>          Select option(s), unlock build phase               │
│    --note "feedback"                   Refinement note for next iteration                │
│    --iterate                           Request refinement (loops back to explore)        │
│                                                                                          │
│  pt ux-status <id>                   Show current UX state                              │
│    Output: cases, options, selection, iteration count                                   │
│                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Task States

```
         ┌─────────────────────────────────────────────────────────────────┐
         │                      TASK STATE MACHINE                          │
         └─────────────────────────────────────────────────────────────────┘

                                    ┌─────────┐
                                    │  open   │
                                    └────┬────┘
                                         │ pt claim
                                         ▼
                                    ┌─────────┐
                              ┌─────│ claimed │─────┐
                              │     └─────────┘     │
                              │                     │
                       has ux field?          no ux field
                              │                     │
                              ▼                     │
                        ┌──────────┐                │
                        │ ux:cases │                │
                        └────┬─────┘                │
                             │ pt ux-cases --confirm
                             ▼                      │
                       ┌───────────┐                │
              ┌───────▶│ux:explore │◀──────┐       │
              │        └─────┬─────┘       │       │
              │              │             │       │
              │        pt ux-select        │       │
              │              │         --iterate   │
              │              ▼             │       │
              │       ┌────────────┐       │       │
              │       │ux:selected │───────┘       │
              │       └──────┬─────┘               │
              │              │                     │
              │   max iterations exceeded?         │
              │              │                     │
              │         no   │   yes               │
              │              │    │                │
              └──────────────┘    ▼                │
                            ┌──────────┐           │
                            │ux:escalate│          │
                            └────┬─────┘           │
                                 │                 │
                                 ▼                 │
                          ┌─────────────┐          │
                          │ ux:approved │◀─────────┘
                          └──────┬──────┘
                                 │
                                 ▼
                           ┌──────────┐
                           │ building │
                           └────┬─────┘
                                │ pt validate
                                ▼
                         ┌─────────────┐
                         │needs_review │
                         └──────┬──────┘
                                │ pt approve
                                ▼
                           ┌────────┐
                           │ closed │
                           └────────┘
```

---

## Example Session

```bash
# 1. Claim task
$ pt claim pt-5
Claimed pt-5: CLI - Chain Display
This task requires UX exploration. Run: pt ux-cases pt-5

# 2. Discover use cases
$ pt ux-cases pt-5

Analyzing task context...

Proposed Use Cases:
┌────────────────────────────────────────────────────────────────────┐
│ [UC-1] Quick scan for entry point                                  │
│   Actor: Trader with directional thesis                            │
│   Goal:  Find strike with target delta (.30-.40) quickly           │
│   Context: Knows direction, needs to identify specific contract    │
├────────────────────────────────────────────────────────────────────┤
│ [UC-2] Premium comparison for income strategy                      │
│   Actor: Premium seller                                            │
│   Goal:  Compare risk/reward across strikes                        │
│   Context: Selling options, wants best premium at support/resist   │
├────────────────────────────────────────────────────────────────────┤
│ [UC-3] Liquidity check before sizing in                            │
│   Actor: Size trader                                               │
│   Goal:  Verify OI and spread before committing to large order     │
│   Context: Needs 50+ contracts, can't afford slippage              │
└────────────────────────────────────────────────────────────────────┘

[Y] Confirm  [E] Edit  [A] Add another  [?] Help: Y

Use cases confirmed. Run: pt ux-explore pt-5

# 3. Explore UX options
$ pt ux-explore pt-5

Generating options for 3 use cases...

[A] Delta-focused ─────────────────────────────────── optimizes: UC-1
┌─────────────────────────────────────────────────────────────────────┐
│   Strike      Δ     Bid/Ask     IV                                  │
│ → $681      .55    2.66/2.69   8.6%    ← arrow marks target range   │
│   $682      .47    2.04/2.07   8.4%                                 │
│   $683      .39    1.51/1.52   8.0%                                 │
│                                                                     │
│ Features: Sorted by delta, visual marker for range, minimal cols    │
└─────────────────────────────────────────────────────────────────────┘

[B] Premium grid ──────────────────────────────────── optimizes: UC-2
┌─────────────────────────────────────────────────────────────────────┐
│   Strike    Bid    %OTM   MaxLoss   RoR                             │
│   $670     0.15    3.2%   $67,000   0.2%                            │
│   $675     0.33    2.1%   $67,500   0.5%    ← best risk/reward      │
│   $680     1.19    0.9%   $68,000   1.8%                            │
│                                                                     │
│ Features: Return on risk calc, max loss shown, highlights best      │
└─────────────────────────────────────────────────────────────────────┘

[C] Liquidity view ────────────────────────────────── optimizes: UC-3
┌─────────────────────────────────────────────────────────────────────┐
│   Strike     OI    Spread   Sprd%   Liquidity                       │
│   $680      752    $0.05    1.5%    [████████░░] Good               │
│   $681      721    $0.03    1.1%    [█████████░] Excellent          │
│   $682      688    $0.03    1.5%    [████████░░] Good               │
│                                                                     │
│ Features: OI + spread %, visual liquidity score, sortable           │
└─────────────────────────────────────────────────────────────────────┘

Select: [A] [B] [C] [A+B] [A+C] [B+C] [A+B+C] or describe: A+C

$ pt ux-select pt-5 "A+C" --note "Primary view is delta-focused, but need liquidity visible too"

UX Selection recorded:
  Choice: A+C (Delta-focused + Liquidity view merged)
  Note: Primary view is delta-focused, but need liquidity visible too
  Iteration: 1 of 3

Proceed to build? [Y/n]: Y

Task pt-5 ready for implementation.
Use cases will be added to DoD criteria.

# 4. Build (normal implementation)
# ... agent implements based on A+C merged design ...

# 5. Validate against use cases
$ pt validate pt-5

Running tests...
  $ go build ./cmd/sot && ./sot chain SPY --help
  ✓ PASS

Manual verification:
  [UC-1] Can locate .30-.40 delta strike in <5 seconds? [y/n]: y
  [UC-2] Premium comparison visible at a glance? [y/n]: y
  [UC-3] Liquidity (OI, spread) clearly shown? [y/n]: y

All criteria met ✓
Task pt-5 moved to needs_review

# 6. Approve
$ pt approve pt-5
Task pt-5 closed.
```

---

## Iteration Flow

When user selects `--iterate` instead of approving:

```bash
$ pt ux-select pt-5 --iterate --note "Delta column is good but I need theta decay visible too"

Iteration 2 of 3. Regenerating options with feedback...
Run: pt ux-explore pt-5

$ pt ux-explore pt-5

Incorporating feedback: "Delta column is good but I need theta decay visible too"

[A'] Delta + Theta focused ────────────────────────── optimizes: UC-1
┌─────────────────────────────────────────────────────────────────────┐
│   Strike      Δ       θ      Bid/Ask     IV                         │
│ → $681      .55    -.12     2.66/2.69   8.6%                        │
│   $682      .47    -.10     2.04/2.07   8.4%                        │
└─────────────────────────────────────────────────────────────────────┘

[B'] Premium + Greeks ─────────────────────────────── optimizes: UC-2
...

Select: [A'] [B'] or describe:
```

---

## Escalation

When max iterations reached without convergence:

```bash
$ pt ux-select pt-5 --iterate --note "Still not right..."

⚠ Maximum iterations (3) reached.

Options:
  1. [F] Force approve current selection
  2. [R] Reset and start over (clears all UX state)
  3. [E] Escalate to user for direct guidance
  4. [X] Abandon task

Select: E

Escalating to user...
Please provide direct guidance for the UX design, or sketch what you need.
```

---

## Integration with Existing Workflow

### Templates That Should Have UX

Any template involving user-facing output should include `ux` field:

| Template | UX Type | Rationale |
|----------|---------|-----------|
| `frontend_component` | cli/tui/web | Direct user interaction |
| `backend_endpoint` | api | API response shape affects consumers |
| `discovery` | doc | Findings need clear presentation |
| `refactor` | - | Usually no UX (internal change) |
| `bugfix` | error | Error messages are UX |

### Skipping UX (when appropriate)

```toml
[[tasks]]
title = "Fix null pointer in parser"
template = "bugfix"
# No [tasks.ux] field = no UX loop required
```

### Minimal UX (quick confirmation)

```toml
[[tasks]]
title = "Add --verbose flag"

[tasks.ux]
type = "cli"
options_min = 1          # Just show one option for confirmation
iteration_max = 1        # One shot
```

---

## Benefits

1. **Grounded in use cases** - Options aren't arbitrary; each solves a real problem
2. **Breadth before depth** - See alternatives before committing
3. **Labeled choices** - Easy to discuss, combine, reference
4. **Iteration budget** - Prevents infinite loops, forces decisions
5. **Traceable** - Selection recorded in task metadata
6. **Validates against intent** - DoD checks use cases, not just "does it run"

---

## Implementation Checklist

- [ ] Add `ux` field to task schema (`pkg/pt/manifest.go`)
- [ ] Add UX state to task metadata (`pkg/pt/types.go`)
- [ ] Implement `pt ux-cases` command
- [ ] Implement `pt ux-explore` command
- [ ] Implement `pt ux-select` command
- [ ] Implement `pt ux-status` command
- [ ] Update `pt claim` to check for UX requirement
- [ ] Update `pt validate` to include use case criteria
- [ ] Add UX state transitions to task state machine
- [ ] Update workflow docs
