package agenttools

import "strings"

// SelectTool is a simple heuristic router for tests/demos. It inspects a prompt
// and attempts to pick a tool from the registry.
// Rules:
// 1) If prompt explicitly names a tool (e.g., "call tool <name>" or "`<name>`"), use it if present.
// 2) If keywords hint at uppercase/shout -> "upper"; echo/echo back -> "echo".
// 3) Fallback: first tool in the registry.
func SelectTool(prompt string, r *Registry) (DynamicTool, bool) {
	p := strings.ToLower(prompt)
	// explicit reference
	for _, t := range r.List() {
		name := strings.ToLower(t.Name())
		if strings.Contains(p, "call tool "+name) || strings.Contains(p, "`"+name+"`") || strings.Contains(p, "use "+name) {
			return t, true
		}
	}
	// keyword hints
	if strings.Contains(p, "uppercase") || strings.Contains(p, "upper-case") || strings.Contains(p, "shout") {
		if t, ok := r.Get("upper"); ok {
			return t, true
		}
	}
	if strings.Contains(p, "echo") || strings.Contains(p, "repeat back") {
		if t, ok := r.Get("echo"); ok {
			return t, true
		}
	}
	// fallback
	list := r.List()
	if len(list) == 0 {
		return nil, false
	}
	return list[0], true
}
