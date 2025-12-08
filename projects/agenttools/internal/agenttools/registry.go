package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Registry holds tools by name.
type Registry struct {
	tools map[string]DynamicTool
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]DynamicTool)}
}

// Default is a global registry for convenience (used by init registration).
var Default = NewRegistry()

// Register adds a tool (error if duplicate).
func (r *Registry) Register(t DynamicTool) error {
	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// MustRegister panics on duplicate (used in init).
func (r *Registry) MustRegister(t DynamicTool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (DynamicTool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns tools sorted by name for deterministic output.
func (r *Registry) List() []DynamicTool {
	out := make([]DynamicTool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Call executes a tool by name with JSON input.
func (r *Registry) Call(ctx context.Context, name string, input []byte) ([]byte, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t.Call(ctx, json.RawMessage(input))
}
