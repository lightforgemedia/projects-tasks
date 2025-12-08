package agenttools

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestMCPServerStdioEcho(t *testing.T) {
	s := server.NewMCPServer("stdio-e2e", "0.0.1", server.WithToolCapabilities(true))
	echo := mockEchoTool()
	s.AddTool(echo.Tool, echo.Handler)

	// In-process client/server using in-process transport
	c, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("in-process client: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("client start: %v", err)
	}
	// Initialize session
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "mcp-e2e-stdio",
				Version: "0.0.1",
			},
		},
	}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	resp, err := c.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "echo",
			Arguments: map[string]any{
				"text": "hi-stdio",
			},
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if len(resp.Content) == 0 {
		t.Fatalf("no content returned: %+v", resp)
	}
	text, ok := resp.Content[0].(mcp.TextContent)
	if !ok || text.Text != "hi-stdio" {
		t.Fatalf("unexpected content: %+v", resp.Content)
	}
}

func TestMCPServerHTTPEcho(t *testing.T) {
	s := server.NewMCPServer("http-e2e", "0.0.1", server.WithToolCapabilities(true))
	echo := mockEchoTool()
	s.AddTool(echo.Tool, echo.Handler)

	// Start streamable HTTP server on httptest
	httpSrv := server.NewStreamableHTTPServer(s)
	ts := httptest.NewServer(httpSrv)
	defer ts.Close()

	// Connect via HTTP client transport
	trans, err := transport.NewStreamableHTTP(ts.URL)
	if err != nil {
		t.Fatalf("http transport: %v", err)
	}
	if err := trans.Start(context.Background()); err != nil {
		t.Fatalf("http transport start: %v", err)
	}
	defer trans.Close()
	c := client.NewClient(trans)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("client start: %v", err)
	}
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "mcp-e2e-http",
				Version: "0.0.1",
			},
		},
	}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	resp, err := c.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "echo",
			Arguments: map[string]any{
				"text": "hi-http",
			},
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if len(resp.Content) == 0 {
		t.Fatalf("no content returned: %+v", resp)
	}
	text, ok := resp.Content[0].(mcp.TextContent)
	if !ok || text.Text != "hi-http" {
		t.Fatalf("unexpected content: %+v", resp.Content)
	}
}
