# Handoff: {TASK_TITLE}

**Date:** {DATE}
**Author:** {AUTHOR}
**Task ID:** {TASK_ID}
**Scope:** {ONE_LINE_SCOPE}

---

## 1. Dependency & Integration Status (REVIEW THIS FIRST)

**Mocking is permitted ONLY when ALL conditions are met:**
1. Real behavior has been proven (spike ran against actual dependency)
2. Mock faithfully reproduces proven behavior (not assumed behavior)
3. Task exists in task system to return to full integration
4. User-facing indicators show when mocked data is in use
5. Integration tests against real dependency exist and pass

### External Dependencies

| Dependency | Real Behavior Proven? | Evidence | Mock Status |
|------------|----------------------|----------|-------------|
| {DEP_NAME} | {YES/NO} | {LINK_OR_DESCRIPTION} | {No mocks / Mock in use} |

### Mock Registry

**{No mocks introduced / Mocks in use - see table}**

| Mock Location | What Real Behavior Was Proven | Proof Evidence | Return-to-Real Task | User Indicator |
|---------------|------------------------------|----------------|---------------------|----------------|
| {FILE:LINE} | {DESCRIPTION} | {TEST_OR_LOG} | {TASK_ID} | {MESSAGE_SHOWN} |

**Reviewer MUST verify:**
- [ ] Every mock has corresponding proof of real behavior
- [ ] Every mock has a tracked task for removal/integration
- [ ] No silent mocks—user always knows when data is fake
- [ ] Integration tests exist and are not skipped in CI

---

## 2. Risk Spike Status

| Risk Area | Spike Status | What Was Proven | What's Still Assumed |
|-----------|--------------|-----------------|----------------------|
| {AREA} | {Validated/Pending/Skipped} | {DESCRIPTION} | {ASSUMPTIONS} |

**Unproven risks the reviewer should scrutinize:**
- {BULLET_POINTS}

---

## 3. UX Exploration Summary

### What was shown to users (or user-proxy agents):
- [ ] CLI input-output examples
- [ ] Breadth-first options presented
- [ ] Key decision points with user sign-off

### Exploration artifacts:
{DESCRIPTION_OF_UX_WORK}

### User decisions captured:
- **Decision:** {DECISION}
- **Why:** {REASONING}
- **Alternatives rejected:** {LIST}

### UX gaps still open:
- {BULLET_POINTS}

---

## 4. What Changed (Summary)

| File/Component | Before | After | Confidence |
|----------------|--------|-------|------------|
| {PATH} | {PREVIOUS_STATE} | {NEW_STATE} | {High/Medium/Low} |

**Confidence key:**
- **High:** Proven by integration tests against real dependencies
- **Medium:** Unit tests pass, integration test pending
- **Low:** Manual verification only

---

## 5. Intent & Approach

### Problem being solved:
{DESCRIPTION}

### Approach taken:
{DESCRIPTION}

### Alternatives I rejected:
- **{ALT_1}:** {WHY_REJECTED}
- **{ALT_2}:** {WHY_REJECTED}

### What's intentionally deferred:
- {BULLET_POINTS}

---

## 6. Stub & Dummy Data Inventory

**{No stubs introduced / Stubs in use - see table}**

| Location | What's Stubbed | User-Visible Indicator | Real Replacement Task | Blocked On |
|----------|----------------|------------------------|----------------------|------------|
| {FILE:LINE} | {DESCRIPTION} | {UI_MESSAGE} | {TASK_ID} | {BLOCKER} |

---

## 7. User Checkpoint Map

### Requires user approval before proceeding:
- [ ] {CHECKPOINT_DESCRIPTION}

### Can be delegated to specialized agent:
- [ ] {DELEGATABLE_WORK}

### No checkpoint needed (routine/mechanical):
- {BULLET_POINTS}

---

## 8. Review Focus Areas

Guide the reviewer's attention (ordered by risk):

### 1. **{HIGHEST_RISK_AREA}**
   - Location: {FILE:LINE}
   - What could go wrong: {RISK}
   - How to verify: {COMMANDS_OR_STEPS}

### 2. **{SECOND_RISK_AREA}**
   - Location: {FILE:LINE}
   - What could go wrong: {RISK}
   - How to verify: {COMMANDS_OR_STEPS}

### Questions the reviewer must answer:
- [ ] {QUESTION_1}
- [ ] {QUESTION_2}

---

## 9. How to Validate

### Run integration tests (MUST pass against real dependencies):
```bash
{TEST_COMMANDS}
```

### Run the spike proofs:
```bash
{SPIKE_COMMANDS}
```

### See the UX as user would:
```bash
{UX_DEMO_COMMANDS}
```

### Exercise the stubbed paths:
{STUB_VERIFICATION_OR_NA}

---

## 10. Context & References

### Original problem analysis:
{DESCRIPTION}

### Files to review:
- {FILE_PATH} - {DESCRIPTION}

### Related documentation:
- {DOC_PATH} - {DESCRIPTION}

### Test output ({DATE}):
```
{TEST_OUTPUT_SAMPLE}
```
