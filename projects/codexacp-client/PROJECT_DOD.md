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
   - Gemini ACP: rejects `mcpServers: null` (expects an array). The wrappers now send `mcpServers: []` when unset, so `newSession` should succeed.
     - Evidence (local): see `outputs/pt-2/gemini.log` (old failure) vs `outputs/pt-15/gemini-fixed.log` (fixed).
   - MCP injection via `mcpServers` (Gemini): best-effort confirmed via `cmd/acp-mcp-smoke` (echo tool call observed in `outputs/pt-3/mcp.log`).

5) Risks & Gaps
   - Known gaps (e.g., terminal stubs, large raw payload routing, lack of persistent logs) are captured in README or TODOs.
   - Codex via `codex-acp`: adapter is known to ignore client-supplied MCP servers; tool calls may not be observable via MCP in that path.

Sign-off expectation: a reviewer verifies the above, records outcome in task comments, and closes the “Project review & sign-off” task.
