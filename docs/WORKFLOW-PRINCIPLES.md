# Risk-First Development Principles

**Purpose**: Stable philosophy for building on solid ground. Tool-agnostic.

---

## Core Principle

**Don't build on sand.** Prove unknowns before investing in implementation.

---

## The Three Phases

### Phase 1: PROVE

Validate everything you don't control before writing production code.

**What to prove:**
- External APIs: Do they return what you expect? Edge cases? Rate limits?
- Algorithms: Does the math work with real data?
- Data shapes: What does the actual payload look like?
- Third-party behavior: Auth flows, error responses, timeouts

**Output**: Captured real responses, documented behavior, yes/no answers.

**Exit criteria**: Each unknown has a definitive answer + artifact.

### Phase 2: EXPLORE

Design with breadth before committing to depth.

**What to explore:**
- Multiple UX approaches (3-5 options minimum)
- CLI interaction patterns
- API contract alternatives
- Error handling strategies

**Output**: Options presented, user picks direction.

**Exit criteria**: User has explicitly chosen an approach (documented).

### Phase 3: BUILD

Implement with risk-ordering: hardest/vaguest first.

**Why hardest first:**
- Discover blockers early when pivoting is cheap
- Easy tasks can fill gaps; hard tasks need focus
- Integration surprises surface before deadline pressure

**Output**: Working feature with tests.

**Exit criteria**: Integration tests pass against real systems.

---

## Strict Mock Discipline

### The Problem

Mocks created before understanding real behavior = false confidence.

- Tests pass, integration fails
- Code drifts from reality
- "Everything works" until production

### The Mock Permission Ladder

| Level | State | Requirements |
|-------|-------|--------------|
| 0 | NO MOCK | Haven't touched real system |
| 1 | SPIKE DONE | Real system hit, responses captured, behavior documented |
| 2 | MOCK IN USE | Derived from real data, integration task exists + scheduled |
| 3 | INTEGRATED | Real integration tests passing |

### Non-Negotiable Rules

1. **No mock without spike**: Must hit real system first
2. **No mock without integration plan**: Task must exist in current or next phase
3. **Mock source must be real data**: Captured from actual system
4. **Mock usage must be visible**: Human and machine-readable indicators
5. **Integration tests are mandatory**: Same assertions run against mock and real
6. **Mocks expire**: Stale captures become lies; force re-spike or integration

### Mock Expiration

Captured responses drift from reality. Two mitigations:

1. **Expiration dates**: Force re-spike or integration by deadline
2. **Shared assertions**: Integration tests run identical checks against mock and real, catching drift

### Integration Test Cadence

Balance thoroughness against API quotas:

| Context | What Runs |
|---------|-----------|
| PR | Unit + mock tests only |
| Nightly | Integration against sandbox |
| Pre-release | Full integration gate (hard block) |

---

## User Feedback Points

Not every task needs user review. Strategic checkpoints:

| When | Question |
|------|----------|
| After spike | Did we learn what we needed? Pivot, workaround, or proceed? |
| After exploration | Which direction? (User picks from options) |
| Before integration | Does this work as expected with real data? |
| Before release | Ready for production? |

### Checkpoint Recording

User decisions must be captured, not just acknowledged:
- Comment with explicit choice: `user-picked: option-b`
- Pivot decisions documented: `pivot: API lacks X, using workaround Y`
- Acceptance noted: `user-approved: 2024-01-15`

---

## Risk-First Phase Gates

Gates are minimal but firm:

| Transition | Gate Type | Condition |
|------------|-----------|-----------|
| prove → explore | soft | All prove tasks closed |
| explore → build | soft→hard | User has picked direction (comment exists) |
| build → signoff | hard | All integration tasks closed, integration tests passing |

**Soft gates**: Warn, allow override with documented reason.
**Hard gates**: Block until condition met. No exceptions.

---

## Worktrees and Isolation

Worktrees (branch-per-task) remain valuable for:
- Multi-agent isolation (parallel work without conflicts)
- Clean rollback points
- Focused context per task

Keep optional. Not every project needs them.

---

## Agent Awareness

Agents consuming output need machine-readable signals:

```json
{
  "mock_mode": true,
  "mock_source": "spike-tradier-chain",
  "integration_task": "pt-12",
  "expires_at": "2024-04-15"
}
```

Agents should not make real decisions based on mock data without explicit acknowledgment.

---

## Summary

1. **Prove before build**: Unknowns first, implementation second
2. **Explore before commit**: Breadth, then depth
3. **Hardest first**: Risk-order the work
4. **Mock with discipline**: Real data, expiration, integration plan
5. **Strategic checkpoints**: User feedback at key gates, not every step
6. **Integration is mandatory**: No release without real system validation

---

*Philosophy is stable. Tooling evolves. This document should rarely change.*
