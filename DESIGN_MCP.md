# MCP + ACP Integration Plan

## Goal
Expose agenttools-managed tools to ACP agents by hosting them via MCP (stdio + HTTP) and registering MCP servers when creating ACP sessions. Document current codex-acp limitation.

## Components
- **MCP server (mcp-go):** stdio/HTTP transports, tools registered via mcp-go (`server.NewMCPServer` + `AddTool`).
- **ACP client (acp-go-sdk):** `session/new` with `mcpServers` populated to point at MCP server(s).
- **codex-acp adapter:** Today, does *not* consume client-provided MCP servers; E2E tool routing is blocked unless adapter/agent adds support.

## Plan
1) **MCP server** in `projects/agenttools/cmd/mcp-server`:
   - stdio and streamable HTTP flags
   - example tool (echo)
   - tests for server construction
2) **ACP wiring** in `projects/codexacp-client`:
   - Include `mcpServers` in `NewSessionRequest` (stdio preferred)
   - Document that codex-acp ignores client MCP today
3) **Future E2E**:
   - When adapter/agent supports client MCP, add tests to assert tool calls route through MCP (e.g., “hooks” tool).

## Constraints & Notes
- ACP v0.6.3 (acp-go-sdk) supports `mcpServers` on session create/load.
- codex-acp (Rust adapter) currently does not expose client-supplied tools; MCP registration is a no-op.
- MCP stdio servers must keep stdout clean; log to stderr.
- HTTP MCP works with mcp-go via `NewStreamableHTTPServer`.

## Action Items
- Add/maintain `phases/agenttools.toml` and `phases/codexacp_mcp.toml` tasks.
- Keep limitations documented in task comments until adapter/agent supports client MCP servers.
