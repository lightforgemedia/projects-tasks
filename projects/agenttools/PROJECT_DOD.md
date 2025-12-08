# Project Definition of Done — Agent Tools

1) Tests
   - `go test ./projects/agenttools/...` passes.

2) CLI
   - `go run ./projects/agenttools/cmd/agent-tools list` shows registered tools.
   - `go run ./projects/agenttools/cmd/agent-tools call echo '{"text":"hi"}'` returns echoed payload.

3) Adapters
   - Registry adapters produce OpenAI-like, Anthropic-like, and ACP-like tool specs without panics; golden tests cover shapes.

4) Registry/Tools
   - Tools self-register via init/blank imports; duplicate registration is prevented.
   - Schema generator placeholder documented (ready for future codegen).

5) Sign-off
   - Review task closed with comments summarizing any gaps or follow-ups.
