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

1.  **Discovery**: Run `pt ready` to find unblocked work. Filter by role if applicable (e.g., `pt ready --role=backend-dev`). Use `--verbose` to see blockers/assignee, `--sort` for ordering.
2.  **Claim**: Run `pt claim <id>` to lock the task (uses $USER or `--as` to specify). This assigns the issue to you and sets status to `in_progress`.
3.  **Execution**:
    *   Read the task details using `pt context init <id>` or inspect the issue description.
    *   Implement changes.
    *   **Important**: If the task has a `manual` check in its DoD, use `pt validate --yes <id>` to auto-confirm after you’ve performed the steps; the confirmed steps will be recorded in the review comment.
4.  **Verification**: Run `pt validate <id>` (or `--yes` for manual-confirmation automation).
    *   **Success**: Task moves to `needs_review` label, with a comment including manual steps if present. Notify the user.
    *   **Failure**: Analyze output. Attempt fixes. If stuck, use `pt release <id>` to unlock it for others.
5.  **Review**: If acting as a reviewer:
    *   Check tasks in `pt ready --role=<role> --verbose` or filter by label/state in the store.
    *   Verify the work.
    *   Run `pt approve <id>` to close it, or `pt reject <id> --reason="..."` to request changes.
