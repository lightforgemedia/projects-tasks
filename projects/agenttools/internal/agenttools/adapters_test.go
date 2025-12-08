package agenttools

import (
	"context"
	"testing"
)

type dummyReq struct {
	Name string `json:"name"`
}
type dummyResp struct {
	Greet string `json:"greet"`
}

func dummyTool() DynamicTool {
	return NewTool(ToolConfig[dummyReq, dummyResp]{
		Name:        "hello",
		Description: "says hello",
	}, func(ctx context.Context, req dummyReq) (dummyResp, error) {
		return dummyResp{Greet: "hi " + req.Name}, nil
	})
}

func TestAdaptersShapes(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(dummyTool())

	openai := OpenAI(r)
	if len(openai) != 1 || openai[0].Function.Name != "hello" || openai[0].Type != "function" {
		t.Fatalf("openai adapter unexpected: %+v", openai)
	}

	anth := Anthropic(r)
	if len(anth) != 1 || anth[0].Name != "hello" {
		t.Fatalf("anthropic adapter unexpected: %+v", anth)
	}

	acp := ACP(r)
	if len(acp) != 1 || acp[0].Name != "hello" {
		t.Fatalf("acp adapter unexpected: %+v", acp)
	}
}
