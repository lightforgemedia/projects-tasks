package agenttools

import "encoding/json"

// OpenAITool is a minimal shape compatible with OpenAI tool declarations.
// This avoids importing provider SDKs while keeping structure familiar.
type OpenAITool struct {
	Type     string         `json:"type"` // "function"
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
}

// AnthropicTool mirrors the OpenAI-like schema used by Anthropic tools.
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ACPCapability is a simplified description suitable for ACP-style tool surfaces.
type ACPCapability struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// OpenAI converts registry tools into OpenAI-style tools.
func OpenAI(r *Registry) []OpenAITool {
	var out []OpenAITool
	for _, t := range r.List() {
		out = append(out, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.InputSchema(),
			},
		})
	}
	return out
}

// Anthropic converts registry tools into Anthropic-style tools.
func Anthropic(r *Registry) []AnthropicTool {
	var out []AnthropicTool
	for _, t := range r.List() {
		out = append(out, AnthropicTool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return out
}

// ACP converts registry tools into ACP capabilities.
func ACP(r *Registry) []ACPCapability {
	var out []ACPCapability
	for _, t := range r.List() {
		out = append(out, ACPCapability{
			Name:         t.Name(),
			Description:  t.Description(),
			InputSchema:  t.InputSchema(),
			OutputSchema: t.OutputSchema(),
		})
	}
	return out
}
