# PT Workflow Architecture

This document explains the workflow system, sub-workflows, state machines, and skills integration in projects-tasks.

---

## Overview: Three-Layer Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                     ORCHESTRATION LAYER                              │
│  (workflow.go: phases, gates, next-task recommendations)            │
├─────────────────────────────────────────────────────────────────────┤
│                        STATE LAYER                                   │
│  (state_manager.go: task status, transitions, dependencies)          │
├─────────────────────────────────────────────────────────────────────┤
│                        WORK LAYER                                    │
│  (manifests, CLI, DoD: task definitions, tests, artifacts)          │
└─────────────────────────────────────────────────────────────────────┘
```

The system separates concerns:
- **Orchestration**: Decides what can happen next (gate evaluation, phase ordering)
- **State**: Tracks what has happened (status transitions, dependency resolution)
- **Work**: Defines what needs to happen (task specs, validation criteria)

---

## 1. Task State Machine

The core state machine in `pkg/pt/state_manager.go` controls individual task lifecycle:

```
                    ┌──────────────────────────────────────────────────┐
                    │                                                   │
                    ▼                                                   │
              ┌──────────┐                                             │
              │ Planned  │  Task created, may have unresolved deps     │
              └────┬─────┘                                             │
                   │                                                   │
                   │ (deps satisfied OR seeded ready)                  │
                   ▼                                                   │
              ┌──────────┐                                             │
              │  Ready   │  Available for claiming                     │
              └────┬─────┘                                             │
                   │                                                   │
                   │ pt claim <id> --as <assignee>                     │
                   ▼                                                   │
         ┌─────────────────┐                                           │
         │   In Progress   │◄──────────────────────────────────┐       │
         └────────┬────────┘                                   │       │
                  │                                            │       │
                  │ pt validate <id>                           │       │
                  ▼                                            │       │
         ┌─────────────────┐                                   │       │
         │ Needs Review    │                                   │       │
         └────────┬────────┘                                   │       │
                  │                                            │       │
        ┌─────────┴─────────┐                                  │       │
        │                   │                                  │       │
        ▼                   ▼                                  │       │
   pt approve          pt reject                               │       │
        │                   │                                  │       │
        ▼                   └──────────────────────────────────┘       │
   ┌──────────┐                                                        │
   │   Done   │────────────────────────────────────────────────────────┘
   └──────────┘         pt reopen <id>
        │
        └──► Unlocks dependent tasks (they move planned → ready)
```

### State Definitions

| State | Meaning | Entry Condition |
|-------|---------|-----------------|
| `planned` | Task exists, dependencies may not be satisfied | Sync from manifest |
| `ready` | Dependencies satisfied, available for work | All deps done |
| `in_progress` | Claimed and being worked | `pt claim` |
| `needs_review` | Work complete, awaiting approval | `pt validate` passes |
| `done` | Approved and closed | `pt approve` |

### Key Insight: Dependency Cascading

When a task moves to `done`, the state manager checks all tasks that depend on it:
- If the dependent's other dependencies are also satisfied → moves to `ready`
- This creates a "waterfall" effect as tasks complete

---

## 2. Phase System (Workflow Layer)

Phases group tasks and control progression through the SDLC. Defined in `workflows/*.toml`:

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Prove     │───▶│   Explore   │───▶│    Build    │───▶│   Signoff   │
│  (spikes)   │    │  (discover) │    │   (impl)    │    │  (review)   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
      │                  │                  │                  │
      │    SOFT GATE     │    HARD GATE     │    HARD GATE     │
      └──────────────────┴──────────────────┴──────────────────┘
```

### Phase Definition (TOML)

```toml
[[phases]]
id = "explore"
name = "UX Discovery"
order = 2
description = "Explore UX options, get user approval"

[phases.gate]
type = "hard"                              # or "soft"
condition = "phase:prove complete"         # gate expression
block_message = "Spikes must complete before UX exploration"

[phases.checkpoint]
trigger = "first_task_complete"
prompt = "Ready to review explored options?"

[phases.proof]
required = true
description = "Wireframes with user approval"
hint = "Save wireframe to artifacts/"
```

### Task → Phase Assignment

Tasks get assigned to phases through three mechanisms (in priority order):

1. **Label**: `labels: ["phase:explore"]` → task goes to "explore" phase
2. **Template**: Template name maps to phase (e.g., `spike → prove`)
3. **Default**: Falls back to workflow's `default_phase`

```toml
[phase_assignment]
default_phase = "build"
label_prefix = "phase:"

[phase_assignment.by_template]
spike = "prove"
discovery = "explore"
backend_endpoint = "build"
```

---

## 3. Gate System

Gates control when tasks in later phases can proceed. Evaluated by `workflow.go:CheckGate()`.

### Gate Types

| Type | Behavior | When Blocked |
|------|----------|--------------|
| **Hard** | Blocks completely | Exit code 1, cannot proceed |
| **Soft** | Warning by default; can be made strict | Can override with `gate-override: <phase-id> <reason>` |

**CLI enforcement:** `pt claim` enforces gates. Hard gates always block. Soft gates require an explicit override:

```bash
pt claim <id> --override-soft="why proceeding is acceptable"
# writes a comment: gate-override: <blocking-phase> <reason>
```

### Gate Conditions (Grammar)

```
condition    := expression
expression   := term (OR term)*
term         := atom
atom         := all_closed
             | phase:ID complete
             | has_comment:TAG
             | phase:ID has_comment:TAG
             | discovery:approved:ID
```

**Examples:**

```toml
# All tasks in current phase must be done
condition = "all_closed"

# A specific prior phase must complete
condition = "phase:prove complete"

# A comment tag must exist in current phase
condition = "has_comment:user-approved"

# A comment tag must exist in a specific phase
condition = "phase:explore has_comment:user-picked"

# Discovery sub-workflow must approve a specific task
condition = "discovery:approved:pt-116"

# Either condition satisfies (OR)
condition = "phase:prove complete OR has_comment:skip-prove"
```

### Gate Evaluation Flow

```
CheckGate(task) returns (canProceed bool, hardBlock bool, blockingPhase string, message string)

1. Determine task's phase (via label/template/default)
2. Find all prior phases (order < task's phase order)
3. For each prior phase:
   a. Get phase's gate condition
   b. Evaluate condition against store
   c. If NOT satisfied:
      - Hard gate? → return (false, true, <phase>, block_message)
      - Soft gate? → return (false, false, <phase>, warning)
4. Check task's own phase gate (if any)
5. All gates satisfied → return (true, false, "", "")
```

---

## 4. Discovery Sub-Workflow

Discovery is a **nested state machine** within a task's in_progress state. It enforces breadth-first UX exploration before implementation.

### Discovery States

```
                                   ┌─────────────────────────────────┐
                                   │                                 │
                                   │ (feedback from user)            │
                                   ▼                                 │
┌──────────┐    ┌──────────────┐    ┌─────────────┐    ┌───────────┐ │
│   init   │───▶│ capabilities │───▶│  exploring  │───▶│synthesized│ │
└──────────┘    └──────────────┘    └─────────────┘    └─────┬─────┘ │
     │               │                    │                  │       │
     │               │                    │                  │       │
     │    CONFIRM    │      QUALITY       │    EVALUATION    │       │
     │     GATE      │       GATES        │      GATE        │       │
     │               │                    │                  │       │
     └───────────────┴────────────────────┘                  │       │
                                                             ▼       │
                                          ┌──────────────────────────┤
                                          │                          │
                                          ▼                          │
                                    ┌───────────┐    ┌────────────┐  │
                                    │ reviewing │───▶│  approval  │──┘
                                    └───────────┘    └─────┬──────┘
                                          │                │
                                          │                │
                                          │                ▼
                                          │          ┌──────────┐
                                          │          │ handoff  │
                                          │          └────┬─────┘
                                          │               │
                                          │               ▼
                                          │     ┌─────────────────┐
                                          │     │ implementation  │
                                          │     └─────────────────┘
                                          │               │
                                          │               └──► Task moves to "build" phase
                                          │
                                          └─────────────────────────────────────┐
                                                                                │
                                            (user requests changes)             │
                                                    │                           │
                                                    ▼                           │
                                          ┌─────────────────┐                   │
                                          │    addressing   │───────────────────┘
                                          │    concerns     │
                                          └─────────────────┘
                                                    │
                                                    └──► Back to exploring
```

### Quality Gates in Discovery

| Gate | Trigger | Requirement |
|------|---------|-------------|
| Confirm | capabilities → exploring | User confirms personas/requirements gathered |
| Breadth | exploring → synthesized | 5+ options, 2+ approaches, 3+ edge cases |
| Evaluation | synthesized → reviewing | Checklist: killer feature, capabilities covered, usability, user's words |
| Approval | reviewing → handoff | User explicitly approves an option |

### Discovery Commands

```bash
pt discovery init <task-id> --type <cli|web|api>
pt discovery capabilities <task-id> --confirm
pt discovery option <task-id> A --name "..." --desc "..."
pt discovery synthesize <task-id>
pt discovery approve <task-id> --select A
```

### Integration with Main Workflow

Discovery completion adds a structured comment to the task:
```
discovery-approved: pt-116
```

Other tasks can gate on this:
```toml
[phases.gate]
condition = "discovery:approved:pt-116"
```

This prevents implementation tasks from proceeding until UX is approved.

---

## 5. Skills Integration

Skills are **prompts displayed at phase transitions** that guide agents through the workflow.

### Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                         MAIN AGENT                                    │
│                                                                       │
│  - Runs PT commands directly (claim, validate, approve)              │
│  - Sees skill prompts at transitions                                 │
│  - Makes orchestration decisions                                      │
│  - Presents options to user                                          │
│  - NEVER delegates PT commands to subagents                          │
│                                                                       │
│     ┌─────────────────┐     ┌─────────────────┐                      │
│     │    Subagent     │     │    Subagent     │                      │
│     │   (Research)    │     │  (Wireframes)   │                      │
│     │                 │     │                 │                      │
│     │ Returns: facts  │     │ Returns: ASCII  │                      │
│     │ Never runs PT   │     │ Never runs PT   │                      │
│     └─────────────────┘     └─────────────────┘                      │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

### Skill Prompt Structure

Each skill prompt contains:
1. **Phase context**: What phase the user is entering
2. **Goal**: What needs to be accomplished
3. **Guidance**: Specific instructions and examples
4. **Next commands**: What PT commands to run next
5. **Quality criteria**: Standards that must be met

### Example: Exploring Phase Skill Prompt

```
== EXPLORE UX OPTIONS ==

Goal: Generate 5+ distinct options across 2+ approaches

Guidance:
- Each option must be concrete (wireframe, not description)
- Cover edge cases (error states, empty states, loading)
- Include at least one unconventional approach
- Document tradeoffs for each option

Quality checklist:
[ ] 5+ options generated
[ ] 2+ different approaches represented
[ ] 3+ edge cases covered
[ ] Each option has wireframe artifact

Next:
  pt discovery option <id> A --name "Option A" --desc "..."
```

### Key Principle: Separation of Orchestration vs Work

| Main Agent | Subagents |
|------------|-----------|
| Runs `pt claim`, `pt validate`, `pt approve` | Never runs PT commands |
| Sees skill prompts | Never sees skill prompts |
| Makes decisions | Returns artifacts |
| Presents options to user | Does research/implementation work |
| Evaluates subagent output | Accepts pushback if output insufficient |

**Why?** Prevents agents from racing through workflows. Main agent is accountable for quality gates; subagents focus on work execution.

---

## 6. End-to-End Task Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│ PHASE 1: MANIFEST SYNC                                                  │
│                                                                          │
│   phases/feature.toml  ──────►  pt sync  ──────►  Creates:              │
│                                                   - pt-100 (discovery)   │
│                                                   - pt-101 (implement)   │
│                                                   - pt-102 (test)        │
└────────────────────────────────────────────────────────────────────────┬┘
                                                                          │
                                                                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ PHASE 2: CLAIM & START                                                  │
│                                                                          │
│   pt claim pt-100 --as alice                                            │
│   (status: ready → in_progress, assignee: alice)                        │
│                                                                          │
│   pt discovery init pt-100 --type cli                                   │
│   → Displays GATHER CONTEXT skill prompt                                │
└────────────────────────────────────────────────────────────────────────┬┘
                                                                          │
                                                                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ PHASE 3: DISCOVERY SUB-WORKFLOW                                         │
│                                                                          │
│   Main agent gathers user context                                       │
│   pt discovery capabilities pt-100 --confirm                            │
│   → Displays EXPLORE OPTIONS skill prompt                               │
│                                                                          │
│   Main agent spawns subagent to create wireframes                       │
│   Subagent returns: "Option A: [ASCII wireframe]..."                    │
│                                                                          │
│   Main agent records options:                                           │
│   pt discovery option pt-100 A --name "Minimal" --desc "..."            │
│   pt discovery option pt-100 B --name "Full-featured" --desc "..."      │
│                                                                          │
│   pt discovery synthesize pt-100                                        │
│   → Evaluation gate checks quality                                      │
│   → Displays PRESENT OPTIONS skill prompt                               │
│                                                                          │
│   Main agent presents top 3 to user, user picks A                       │
│   pt discovery approve pt-100 --select A                                │
│   → Adds comment: "discovery-approved: pt-100"                          │
└────────────────────────────────────────────────────────────────────────┬┘
                                                                          │
                                                                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ PHASE 4: VALIDATION                                                     │
│                                                                          │
│   pt validate pt-100                                                    │
│   → Runs tests from DoD                                                 │
│   → Prompts for manual confirmations                                    │
│   → Records results in comment                                          │
│   (status: in_progress → needs_review)                                  │
└────────────────────────────────────────────────────────────────────────┬┘
                                                                          │
                                                                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ PHASE 5: APPROVAL                                                       │
│                                                                          │
│   pt approve pt-100                                                     │
│   (status: needs_review → done)                                         │
│   → Unlocks pt-101 (was blocked on discovery gate)                      │
└────────────────────────────────────────────────────────────────────────┬┘
                                                                          │
                                                                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ PHASE 6: NEXT TASK                                                      │
│                                                                          │
│   pt workflow next                                                      │
│   → Evaluates gates for all ready tasks                                 │
│   → Recommends pt-101 (implementation) with context                     │
│                                                                          │
│   pt claim pt-101 --as alice                                            │
│   → Implementation begins, constrained to approved wireframe            │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 7. Multi-Layer Dependency Model

Dependencies exist at multiple levels:

```
┌────────────────────────────────────────────────────────────────────┐
│                    TASK DEPENDENCIES                                │
│                                                                     │
│  pt-101 depends on pt-100                                          │
│  (State manager: pt-101 stays planned until pt-100 is done)        │
└────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌────────────────────────────────────────────────────────────────────┐
│                    PHASE DEPENDENCIES                               │
│                                                                     │
│  "build" phase gates on "explore" phase                            │
│  (Workflow: build tasks blocked until explore complete)            │
└────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌────────────────────────────────────────────────────────────────────┐
│                  DISCOVERY DEPENDENCIES                             │
│                                                                     │
│  Implementation gates on "discovery:approved:pt-100"               │
│  (Sub-workflow: impl blocked until UX approved)                    │
└────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌────────────────────────────────────────────────────────────────────┐
│                   SOFT GATE OVERRIDES                               │
│                                                                     │
│  Comment "gate-override: <phase-id> <reason>" bypasses soft gates   │
│  (Escape hatch for urgent work)                                    │
└────────────────────────────────────────────────────────────────────┘
```

---

## 8. File Locations Reference

| Component | Location |
|-----------|----------|
| Task state machine | `pkg/pt/state_manager.go` |
| Workflow/phase logic | `pkg/pt/workflow.go` |
| Gate evaluation | `pkg/pt/workflow.go:CheckGate()` |
| Workflow CLI | `cmd/pt/workflow.go` |
| Discovery sub-workflow | `docs/discovery-workflow.md` |
| Workflow templates | `workflows/*.toml` |
| Task manifests | `phases/*.toml` |
| Store (runtime) | `.pt.db.json` |

---

## Summary

The PT workflow system creates a **risk-ordered, quality-gated SDLC** through:

1. **Task state machine**: Individual task lifecycle (planned → ready → in_progress → needs_review → done)
2. **Phase system**: Groups tasks into ordered phases with gate conditions
3. **Gate system**: Hard/soft gates that evaluate conditions before allowing progression
4. **Discovery sub-workflow**: Nested state machine enforcing breadth-first UX exploration
5. **Skills integration**: Prompts that guide agents through phase transitions
6. **Multi-layer dependencies**: Task deps + phase deps + discovery deps + soft overrides

The key insight: **separation of orchestration (main agent) from work execution (subagents)** ensures quality gates are actually enforced rather than bypassed.
