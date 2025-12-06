# Repository Guidelines for Agents

## Project Structure
- **Root**: Documentation and config.
- **`pkg/pt`**: The Go SDK. Core logic lives here.
- **`cmd/pt`**: The CLI entrypoint.
- **`phases/`**: Example or active manifests.

## Development Standards
- **Go**: Use standard formatting (`gofmt`).
- **Tests**: Unit tests next to code (`_test.go`). No skipping.
- **Secrets**: Never commit secrets.

## Agent Workflows
Agents interacting with this repository or using the `pt` tool should follow this loop:

1.  **Discovery**: Run `pt ready` to find unblocked work. Filter by role if applicable (e.g., `pt ready --role=backend-dev`).
2.  **Claim**: Run `pt claim <id>` to lock the task. This assigns the issue to you and sets status to `in_progress`.
3.  **Execution**:
    *   Read the task details using `bd show <id>`.
    *   Implement changes.
    *   **Important**: If the task has a `manual` check in its DoD, be prepared to pipe "y" to the validation command or ensure the condition is met.
4.  **Verification**: Run `pt validate <id>`.
    *   **Success**: Task moves to `needs_review` label. Notify the user.
    *   **Failure**: Analyze output. Attempt fixes. If stuck, use `pt release <id>` to unlock it for others.
5.  **Review**: If acting as a reviewer:
    *   Check tasks with `bd list --label state:needs_review`.
    *   Verify the work.
    *   Run `pt approve <id>` to close it, or `pt reject <id> --reason="..."` to request changes.