package agenttools

import (
	"context"
	"testing"
)

// simple tool for tests
type pingReq struct {
	Msg string `json:"msg"`
}
type pingResp struct {
	Msg string `json:"msg"`
}

func TestRegistryRegisterAndCall(t *testing.T) {
	r := NewRegistry()
	tool := NewTool(ToolConfig[pingReq, pingResp]{Name: "ping", Description: "ping"}, func(ctx context.Context, req pingReq) (pingResp, error) {
		return pingResp{Msg: req.Msg}, nil
	})
	if err := r.Register(tool); err != nil {
		t.Fatalf("register: %v", err)
	}
	out, err := r.Call(context.Background(), "ping", []byte(`{"msg":"hi"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if string(out) != `{"msg":"hi"}` {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRegistryDuplicate(t *testing.T) {
	r := NewRegistry()
	tool := NewTool(ToolConfig[pingReq, pingResp]{Name: "dup", Description: "dup"}, func(ctx context.Context, req pingReq) (pingResp, error) {
		return pingResp{Msg: req.Msg}, nil
	})
	if err := r.Register(tool); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(tool); err == nil {
		t.Fatalf("expected duplicate error")
	}
}
