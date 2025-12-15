# PT Discovery Workflow: Architecture & State Machines

## Overview

The PT (Projects-Tasks) system includes a **discovery sub-workflow** designed to enforce rigorous UX exploration before implementation. This document explains the architecture, state machines, skill prompts, and orchestration model.

## Core Philosophy

**Problem:** Developers (human or AI) often jump straight to implementation, skipping the exploration phase that surfaces better designs.

**Solution:** A state machine that:
1. Forces breadth-first exploration (5+ options, 2+ approaches)
2. Gates transitions on quality criteria
3. Provides contextual skill prompts at each phase
4. Requires user approval before implementation
5. Saves wireframes as source of truth

---

## State Machine

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DISCOVERY STATE MACHINE                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌──────────┐     ┌──────────────┐     ┌────────────┐                     │
│   │   INIT   │────▶│ CAPABILITIES │────▶│ EXPLORING  │◀─┐                  │
│   └──────────┘     └──────────────┘     └────────────┘  │                  │
│        │                  │                   │         │                  │
│        │                  │                   ▼         │                  │
│        │                  │            ┌────────────┐   │                  │
│        │                  │            │SYNTHESIZED │───┤ (iterate)        │
│        │                  │            └────────────┘   │                  │
│        │                  │                   │         │                  │
│        │                  │                   ▼         │                  │
│        │                  │            ┌────────────┐   │                  │
│        │                  │            │ REVIEWING  │───┘                  │
│        │                  │            └────────────┘                      │
│        │                  │                   │                            │
│        │                  │          ┌───────┴───────┐                     │
│        │                  │          ▼               ▼                     │
│        │                  │   ┌──────────┐    ┌──────────┐                 │
│        │                  │   │ FEEDBACK │───▶│ APPROVED │                 │
│        │                  │   └──────────┘    └──────────┘                 │
│        │                  │                         │                      │
│        │                  │                         ▼                      │
│        │                  │                  ┌────────────┐                │
│        │                  │                  │  HANDOFF   │                │
│        │                  │                  └────────────┘                │
│        │                  │                         │                      │
│        │                  │                         ▼                      │
│        │                  │                  ┌────────────┐                │
│        │                  │                  │IMPLEMENTATION               │
│        │                  │                  └────────────┘                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### State Definitions

| State | Description | Entry Condition | Exit Condition |
|-------|-------------|-----------------|----------------|
| `init` | Discovery initialized | `pt discovery init` | Personas/capabilities gathered |
| `capabilities` | Requirements defined | At least 1 persona | `--confirm` flag with 3+ capabilities |
| `exploring` | Generating options | Capabilities confirmed | Gates passed (5+ options, 2+ approaches) |
| `synthesized` | Top 3 ranked | Gates passed | User reviews |
| `reviewing` | User reviewing options | Synthesis complete | User provides feedback or approval |
| `feedback` | Concerns recorded | User gave feedback | Concerns addressed |
| `approved` | User approved design | Explicit user approval | Wireframe saved |
| `handoff` | Implementation guidance | Wireframe exists | Implementation begins |

### Transition Gates

```
capabilities → exploring:
  ✓ 1+ persona defined
  ✓ 3+ capabilities listed
  ✓ User confirmed with --confirm flag

exploring → synthesized:
  ✓ 5+ options explored
  ✓ 2+ different approaches tried
  ✓ 3+ edge cases covered (empty, loading, error minimum)

synthesized → reviewing:
  ✓ Top 3 ranked by coverage
  ✓ Evaluation checklist passed (NEW - see below)

reviewing → approved:
  ✓ Explicit user selection
  ✓ No unaddressed feedback

approved → handoff:
  ✓ Wireframe file saved
  ✓ Mockup exists for selected option
```

---

## Skill Prompts System

At each state transition, a **skill prompt** is displayed. These prompts:
1. Explain the goal of the current phase
2. Provide concrete guidance and examples
3. List commands for the next step
4. Enforce quality standards

### Skill Prompt Catalog

| Phase | Prompt Name | Purpose |
|-------|-------------|---------|
| init → capabilities | GATHER USER CONTEXT | Discover personas and capabilities |
| capabilities → exploring | EXPLORE UX OPTIONS | Generate 5+ options autonomously |
| synthesized → reviewing | PRESENT OPTIONS TO USER | Show top 3 for user decision |
| reviewing (active) | FACILITATE USER REVIEW | Answer questions, record feedback |
| feedback → iterate | ADDRESS USER FEEDBACK | Revise options based on concerns |
| approved → handoff | IMPLEMENT APPROVED UX | Follow wireframe exactly |

### Skill Prompt Structure

Each prompt follows this structure:

```
┌─ AGENT SKILL: <NAME> ─────────────────────────────────────────────────────┐
│                                                                           │
│ ⚠️  ORCHESTRATION: Run PT commands directly. Delegate WORK to agents.     │
│                                                                           │
│ GOAL: <What this phase achieves>                                          │
│                                                                           │
│ ═══════════════════════════════════════════════════════════════════════   │
│ STEP 1: <First action>                                                    │
│ ═══════════════════════════════════════════════════════════════════════   │
│                                                                           │
│ <Detailed guidance with examples>                                         │
│                                                                           │
│ GOOD EXAMPLE: <What good looks like>                                      │
│ BAD EXAMPLE: <What to avoid>                                              │
│                                                                           │
│ ═══════════════════════════════════════════════════════════════════════   │
│ DONE WHEN: <Exit criteria>                                                │
│ ═══════════════════════════════════════════════════════════════════════   │
│                                                                           │
│ Next command: <pt discovery ...>                                          │
└───────────────────────────────────────────────────────────────────────────┘
```

### Key Additions (Recent)

**1. Orchestration Warning (all prompts)**
```
⚠️  ORCHESTRATION: Run PT commands directly. Delegate WORK to agents.
```
Prevents agents from delegating PT commands themselves instead of the actual work.

**2. Evaluation Gate (before PRESENT OPTIONS)**
```
┌─ AGENT SKILL: EVALUATE OPTIONS AGAINST REQUIREMENTS ──────────────────────┐
│                                                                           │
│ CHECKLIST (answer honestly for EACH top option):                          │
│                                                                           │
│   1. KILLER FEATURE - Does it address what the user emphasized most?      │
│   2. CAPABILITY COVERAGE - Walk through each UC                           │
│   3. WOULD YOU USE IT? - Be brutally honest                               │
│   4. MATCHES USER'S WORDS - Their vision, not generic UX                  │
│                                                                           │
│ IF ALL TOP 3 FAIL:                                                        │
│   Don't settle. Push back on subagent work. Redo exploration.             │
└───────────────────────────────────────────────────────────────────────────┘
```

---

## Orchestration Model

### The Delegation Problem

**Wrong approach:**
```
User: "Design the trading dashboard"
Agent: → Spawns subagent: "Run pt discovery init, pt discovery explore..."
       → Subagent runs all PT commands
       → Agent never sees skill prompts
       → Workflow is bypassed
```

**Correct approach:**
```
User: "Design the trading dashboard"
Agent: → Runs: pt discovery init pt-116 --type web
       → SEES skill prompt: "GATHER USER CONTEXT"
       → Asks user about personas, capabilities
       → Runs: pt discovery capabilities pt-116 "cap1" "cap2"
       → SEES skill prompt: "EXPLORE UX OPTIONS"
       → Spawns subagent: "Create 5 ASCII wireframes for trading dashboard"
       → Subagent returns wireframes (WORK output)
       → Agent runs: pt discovery option pt-116 A --name "..." --desc "..."
       → Agent EVALUATES before presenting
       → Agent presents to user for approval
```

### Orchestration Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           MAIN AGENT                                    │
│  - Runs PT commands directly                                            │
│  - Sees and follows skill prompts                                       │
│  - Manages state transitions                                            │
│  - Evaluates subagent output                                            │
│  - Presents options to user                                             │
├─────────────────────────────────────────────────────────────────────────┤
│                          SUBAGENTS                                      │
│  - Do the WORK (research, wireframes, code)                             │
│  - Return artifacts to main agent                                       │
│  - Never run PT orchestration commands                                  │
│  - Can be pushed back on if output doesn't match requirements           │
└─────────────────────────────────────────────────────────────────────────┘
```

### Ownership Mindset

The main agent acts as a **tech lead**:
- Reviews subagent work before presenting to user
- Pushes back if work doesn't meet requirements
- Takes responsibility for quality
- Doesn't present "garbage to move forward"

---

## Integration with SDLC Workflow

### How Discovery Fits into PT Tasks

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         PT SDLC WORKFLOW                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. MANIFEST SYNC                                                       │
│     phases/tradeview.toml → Creates pt-116 (discovery template)         │
│                                                                         │
│  2. CLAIM TASK                                                          │
│     pt claim pt-116 → Shows discovery workflow guidance                 │
│                                                                         │
│  3. DISCOVERY SUB-WORKFLOW                                              │
│     pt discovery init → ... → pt discovery approve                      │
│     (See state machine above)                                           │
│                                                                         │
│  4. AUTO-COMMENT ON APPROVAL                                            │
│     When approved, adds: "discovery-approved: pt-116"                   │
│     This enables workflow gates to check discovery status               │
│                                                                         │
│  5. IMPLEMENTATION TASKS UNBLOCKED                                      │
│     Gate: "discovery:approved:pt-116" passes                            │
│     pt-117, pt-118, etc. can now proceed                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Workflow Gate Integration

In workflow TOML files:

```toml
[[phases]]
id = "implement"
name = "Implementation"
order = 2

[phases.gate]
type = "hard"
condition = "discovery:approved:pt-116"
block_message = "UX discovery must be approved before implementation"
```

---

## File Structure

### Synthesis Storage

```
.pt/
├── synthesis/
│   └── pt-116.json           # Discovery state and data
│       ├── component_id
│       ├── ux_type
│       ├── status              # Current state
│       ├── capabilities[]      # UC1, UC2, ...
│       ├── personas[]          # P1, P2, ...
│       ├── options[]           # Top 3 (or all explored)
│       ├── rejected[]          # Options not in top 3
│       ├── feedback[]          # User concerns
│       ├── recommendation      # Selected option label
│       └── ...
│
└── discovery/
    └── pt-116/
        └── mockups/
            └── option-f.txt    # Approved wireframe (source of truth)
```

### Wireframe as Source of Truth

The approved wireframe file becomes the implementation spec:
- Component IDs ([A1], [B2.1]) map to React components
- Edge cases listed must all be implemented
- Deviation from wireframe requires re-approval

---

## Command Reference

### Phase: Init
```bash
pt discovery init <component-id> --type <cli|tui|web|api>
```

### Phase: Capabilities
```bash
pt discovery persona <id> --add "Name" --goals "..." --constraints "..."
pt discovery capabilities <id> --add "action: Do something"
pt discovery capabilities <id> --confirm
```

### Phase: Exploring
```bash
pt discovery option <id> <label> --name "..." --desc "..." --approach "..."
pt discovery edge-case <id> --cover empty --in A
pt discovery explore <id>  # Shows gate status
```

### Phase: Synthesize
```bash
pt discovery synthesize <id>
```

### Phase: Review
```bash
pt discovery review <id>
pt discovery feedback <id> "user concern"
pt discovery feedback <id> --component A3 "specific issue"
```

### Phase: Iterate
```bash
pt discovery iterate <id> --address F1
pt discovery synthesize <id>  # Re-rank after changes
```

### Phase: Approve
```bash
pt discovery approve <id> --select A
```

### Phase: Handoff
```bash
pt discovery mockup <id> <label> < wireframe.txt
pt discovery handoff <id>
```

---

## Quality Enforcement

### Gates That Exist

| Gate | What It Checks | When |
|------|----------------|------|
| Option count | 5+ options explored | Before synthesize |
| Approach diversity | 2+ different approaches | Before synthesize |
| Edge case coverage | empty, loading, error minimum | Before synthesize |
| Persona definition | 1+ persona defined | Before synthesize |
| Evaluation checklist | Killer feature, coverage, usability | Before presenting |
| Wireframe existence | Mockup file saved | Before handoff |

### What's NOT Enforced (Manual Discipline)

| Gap | Current Status | Workaround |
|-----|----------------|------------|
| Quality of options | Not validated | Agent must evaluate honestly |
| User's exact requirements | Not compared | Agent must check manually |
| Subagent output quality | Not validated | Agent must review and push back |
| Acknowledgment of prompts | Shown but not confirmed | Agent must follow prompts |

---

## Example Session Flow

```
1. User: "Build trading dashboard with tab workspaces"

2. Agent: pt claim pt-116
   → Sees: "🔍 This task uses the discovery workflow"
   → Sees: "⚠️ ORCHESTRATION NOTE: Run commands yourself..."

3. Agent: pt discovery init pt-116 --type web
   → Sees: GATHER USER CONTEXT skill prompt
   → Asks user about personas, capabilities

4. Agent: pt discovery capabilities pt-116 --add "UC1" --add "UC2" ...
   → User confirms: "looks good"

5. Agent: pt discovery capabilities pt-116 --confirm
   → Sees: EXPLORE UX OPTIONS skill prompt

6. Agent: Spawns subagent → "Create 5 wireframes for trading dashboard"
   → Subagent returns wireframes

7. Agent: EVALUATES wireframes against requirements
   → "Do these address the killer feature?"
   → "Would I use this?"
   → If NO: Push back, redo
   → If YES: Continue

8. Agent: pt discovery option pt-116 A --name "..." ...
   (adds each wireframe as option)

9. Agent: pt discovery synthesize pt-116
   → Sees: EVALUATE OPTIONS skill prompt
   → Sees: PRESENT OPTIONS skill prompt

10. Agent: Presents top 3 to user with mockups
    → User: "I like option F"

11. Agent: pt discovery approve pt-116 --select F
    → Sees: IMPLEMENT APPROVED UX skill prompt

12. Agent: Saves wireframe
    → pt discovery mockup pt-116 F < wireframe.txt

13. Agent: pt discovery handoff pt-116
    → Gets implementation guidance
    → Proceeds to implementation following wireframe exactly
```

---

## Summary

The discovery workflow is a **quality enforcement layer** that:

1. **Forces exploration** - Can't implement until 5+ options explored
2. **Provides guidance** - Skill prompts at each phase
3. **Requires user approval** - No autonomous implementation decisions
4. **Creates artifacts** - Wireframes as source of truth
5. **Integrates with SDLC** - Gates block implementation until approved

The key insight is **separation of orchestration and work**:
- Main agent: Runs PT commands, sees prompts, manages state
- Subagents: Do research, create wireframes, write code
- User: Makes decisions, provides approval

This prevents the common failure mode where agents race through a workflow without actually following the guidance it provides.
