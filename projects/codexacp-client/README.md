# codexacp-client

Go ACP client that drives Codex via Zed's `codex-acp` Rust adapter. It auto-approves permissions, streams agent message chunks, and emits structured tool events as JSON (`[tool-event] {...}`).

- Uses `github.com/coder/acp-go-sdk` v0.6.3
- Spawns `codex-acp` (stdio) and issues `initialize -> newSession -> prompt`
- Filesystem + terminal capabilities enabled; tool raw input/output logged

## Prereqs
- Go 1.22+
- `codex-acp` binary installed and on PATH (https://github.com/zed-industries/codex-acp/releases)
- `OPENAI_API_KEY` or `CODEX_API_KEY` exported for the adapter

## Install deps
```bash
cd projects/codexacp-client
go mod tidy
```

## Run
```bash
cd projects/codexacp-client
go run ./cmd/codexacp-client \
  --codex-acp-path codex-acp \
  --cwd "$(pwd)" \
  --prompt "Summarize this workspace" \
  --debug
```
- Flags: `--codex-acp-path` (adapter path), `--cwd` (abs/relative OK), `--prompt` (text), `--debug` (enable slog output).
- The client prints streamed text from the agent and `[tool-event] { ... }` lines for tool calls/updates (includes status, raw input/output).

## Testing
```bash
cd projects/codexacp-client
go test ./...
```

## Project Definition of Done
See `projects/codexacp-client/PROJECT_DOD.md` for the project-level checklist (tests, CLI UX, tool-event logging, smoke run notes, known gaps).

## Notes
- Terminal handling is stubbed for now; adjust `CodexClient` if you need true interactive shells.
- Tool raw payloads are round-tripped to maps for logging; if adapters emit large payloads, route them to a file/log sink before production use.
