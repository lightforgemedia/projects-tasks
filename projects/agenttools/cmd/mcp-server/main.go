package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	transport := flag.String("transport", "stdio", "stdio or http")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	log.SetOutput(os.Stderr) // MCP stdio requires stdout clean

	s := server.NewMCPServer("Agenttools MCP", "0.1.0", server.WithToolCapabilities(true), server.WithRecovery())

	echoTool := mcp.NewTool("echo",
		mcp.WithDescription("Echo text back"),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to echo")),
	)
	s.AddTool(echoTool, echoHandler)

	switch *transport {
	case "stdio":
		log.Println("Starting MCP server on stdio")
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("stdio server error: %v", err)
		}
	case "http":
		log.Printf("Starting MCP StreamableHTTP server on %s\n", *addr)
		httpSrv := server.NewStreamableHTTPServer(s)
		if err := httpSrv.Start(*addr); err != nil {
			log.Fatalf("http server error: %v", err)
		}
	default:
		log.Fatalf("unknown transport %q (use stdio or http)", *transport)
	}
}

func echoHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := req.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("echo: %s", text)), nil
}
