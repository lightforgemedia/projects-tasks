package agenttools

import (
	"context"
	"strings"
	"testing"
)

func registerDemoTools() *Registry {
	r := NewRegistry()
	r.MustRegister(NewTool(ToolConfig[pingReq, pingResp]{Name: "echo", Description: "echo"}, func(ctx context.Context, req pingReq) (pingResp, error) {
		return pingResp{Msg: req.Msg}, nil
	}))
	r.MustRegister(NewTool(ToolConfig[pingReq, pingResp]{Name: "upper", Description: "upper"}, func(ctx context.Context, req pingReq) (pingResp, error) {
		return pingResp{Msg: strings.ToUpper(req.Msg)}, nil
	}))
	return r
}

func TestSelectToolScenarios(t *testing.T) {
	r := registerDemoTools()
	tests := []struct {
		name     string
		prompt   string
		expect   string
		payload  string
		expected string
	}{
		{
			name:     "explicit tool request",
			prompt:   "Please call tool upper to uppercase this string",
			expect:   "upper",
			payload:  `{"msg":"hi"}`,
			expected: `{"msg":"HI"}`,
		},
		{
			name:     "tools available hint",
			prompt:   "Tools are available; uppercase this text",
			expect:   "upper",
			payload:  `{"msg":"hello"}`,
			expected: `{"msg":"HELLO"}`,
		},
		{
			name:     "describe task only",
			prompt:   "Please echo back the payload exactly",
			expect:   "echo",
			payload:  `{"msg":"mirror"}`,
			expected: `{"msg":"mirror"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := SelectTool(tt.prompt, r)
			if !ok {
				t.Fatalf("no tool selected")
			}
			if tool.Name() != tt.expect {
				t.Fatalf("expected %s, got %s", tt.expect, tool.Name())
			}
			out, err := tool.Call(context.Background(), []byte(tt.payload))
			if err != nil {
				t.Fatalf("call error: %v", err)
			}
			if string(out) != tt.expected {
				t.Fatalf("unexpected output: %s", out)
			}
		})
	}
}
