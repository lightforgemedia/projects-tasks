package agenttools

import "encoding/json"

// generateJSONSchema is a placeholder; in v1 we return an empty object to keep
// provider adapters stable. This can be replaced by codegen/reflection later.
func generateJSONSchema[T any]() json.RawMessage {
	return json.RawMessage(`{}`)
}
