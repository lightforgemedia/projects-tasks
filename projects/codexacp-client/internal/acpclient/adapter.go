package acpclient

import (
	"encoding/json"

	"agenttools"
)

// RegistryToolSpec is a lightweight description of an agenttools registry entry.
// ACP v0.6.3 does not define client-provided tools, so we expose a helper spec
// for future adapters or logging.
type RegistryToolSpec struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// ToolsFromRegistry converts registry tools into generic specs.
func ToolsFromRegistry(r *agenttools.Registry) []RegistryToolSpec {
	var specs []RegistryToolSpec
	for _, t := range r.List() {
		specs = append(specs, RegistryToolSpec{
			Name:         t.Name(),
			Description:  t.Description(),
			InputSchema:  t.InputSchema(),
			OutputSchema: t.OutputSchema(),
		})
	}
	return specs
}
