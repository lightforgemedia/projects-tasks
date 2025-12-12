# Project Definition of Done — codexacp-client

This checklist must be satisfied before considering the Codex ACP client “done”:

1) Tests and Lint
   - `go test ./...` completes without failures.

2) CLI UX
   - `codexacp-client --help`/flags are documented in README.
   - Flags: `--codex-acp-path`, `--cwd`, `--prompt`, `--debug` present and described.

3) Tool-Event Logging
   - Tool calls/updates emit `[tool-event] { ... }` JSON with session id, tool_call_id, status, raw input/output.
   - Behavior is documented in README.

4) Adapter Smoke (best-effort)
   - Attempt a smoke run against `codex-acp` (or document if unavailable in this environment). Expected flow: initialize -> newSession -> prompt; stream agent text; see tool-event logs if tools fire.
   - Gemini ACP (2025-12-07): `gemini --experimental-acp` returns `newSession` error when `mcpServers` is null/omitted. Current mitigation: run with `--mcp-args "" --mcp-cmd ""` to avoid null; MCP injection remains unverified because agents may ignore/require explicit empty arrays.

5) Risks & Gaps
   - Known gaps (e.g., terminal stubs, large raw payload routing, lack of persistent logs) are captured in README or TODOs.
   - MCP server injection via `mcpServers` is not yet confirmed with Gemini ACP (agent rejects null mcpServers); codex-acp adapter is known to ignore client-supplied MCP. Tool calls are not observable via MCP in this environment.

Sign-off expectation: a reviewer verifies the above, records outcome in task comments, and closes the “Project review & sign-off” task.
