# Task Review Checklist

Use this checklist to verify a task is **handoff-ready** before an agent can claim it.
A task is handoff-ready when a new agent with no project context can start immediately.

## Required Fields

### 1. Context
**Question:** Does the task explain WHY this work matters?

- [ ] States the problem being solved or feature being added
- [ ] References any bugs, requirements, or prior decisions
- [ ] Agent understands motivation without external research

**Fix if missing:** `pt update <id> --context "..."`

### 2. Inputs
**Question:** Does the agent know WHERE to look?

- [ ] Lists specific files or directories to read/modify
- [ ] References existing patterns or implementations to follow
- [ ] No vague pointers like "the codebase" or "related files"

**Fix if missing:** `pt update <id> --inputs "file1.go,file2.go"`

### 3. Scope
**Question:** Does the task define boundaries?

- [ ] Clear IN-scope: what must be done
- [ ] Clear OUT-of-scope: what to avoid
- [ ] Agent won't accidentally over-engineer or miss requirements

**Fix if missing:** `pt update <id> --scope "IN: ... OUT: ..."`

### 4. Artifact
**Question:** What gets delivered?

- [ ] Specifies type: `code:`, `spec:`, `doc:`, `test:`
- [ ] Points to the primary output file/location
- [ ] Enables verification of completion

**Already required by manifest schema**

### 5. Definition of Done

#### Tests
- [ ] At least one automated test command
- [ ] Tests verify the criteria, not just "echo ok"
- [ ] Commands work in the project directory

#### Manual Steps
- [ ] Human verification steps if applicable
- [ ] Specific enough to confirm success
- [ ] Not just "check that it works"

#### Criteria
- [ ] Observable behaviors that define success
- [ ] Written as pass/fail statements
- [ ] Agent can self-validate completion

## Review Decision

After checking all items:

- **APPROVE**: All items pass → Agent can claim the task
- **NEEDS WORK**: Items fail → Update task, re-review

## Example: Good Task vs Bad Task

### Bad Task (not handoff-ready)
```
Title: Add user validation
Artifact: code:user.go
DoD: tests=[go test ./...] manual=test it criteria=[works correctly]
```

### Good Task (handoff-ready)
```
Title: Add user validation
Context: Users can submit invalid emails causing downstream failures.
         Need to validate email format on registration.
Inputs: [pkg/user/registration.go, pkg/user/validation_test.go]
Scope: IN: email format validation on Register(). OUT: no UI changes
Artifact: code:pkg/user/validation.go
DoD:
  tests: [go test ./pkg/user/... -run TestEmailValidation]
  manual: Register with invalid email, verify error returned
  criteria: [Invalid emails rejected with clear error, Valid emails pass, Existing tests still pass]
```
