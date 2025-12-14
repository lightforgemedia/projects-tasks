package pt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TaskMeta holds structured metadata for a task stored in the description.
type TaskMeta struct {
	Template string           `json:"template"`
	Role     string           `json:"role"`
	NextHint string           `json:"next_hint,omitempty"`
	Artifact string           `json:"artifact,omitempty"`
	DoD      DefinitionOfDone `json:"dod"`

	// Handoff fields - help agents understand the task without project context
	Context   string   `json:"context,omitempty"`   // WHY: problem being solved, motivation
	Inputs    []string `json:"inputs,omitempty"`    // WHERE: files/directories to read/modify
	Scope     string   `json:"scope,omitempty"`     // BOUNDS: IN-scope and OUT-of-scope
	Reference string   `json:"reference,omitempty"` // RELATED: links to docs, issues, prior work

	// UX discovery - optional exploration loop
	UX      *UXConfig `json:"ux,omitempty"`       // UX requirements from manifest
	UXState *UXState  `json:"ux_state,omitempty"` // Runtime UX exploration state

	// Spike-specific fields
	MaxHours int `json:"max_hours,omitempty"` // time-box for spike tasks
}

// UXState tracks the current state of UX exploration for a task.
type UXState struct {
	Status     string     `json:"status"`               // pending|cases|explore|selected|approved
	UseCases   []UseCase  `json:"use_cases,omitempty"`  // Confirmed use cases
	Options    []string   `json:"options,omitempty"`    // Generated options (labels: A, B, C...) - legacy
	Mockups    []UXMockup `json:"mockups,omitempty"`    // File-based mockups with fidelity tracking
	Coverage   UXCoverage `json:"coverage,omitempty"`   // Capability coverage per mockup
	Selection  string     `json:"selection,omitempty"`  // User's choice (e.g., "A+C")
	Note       string     `json:"note,omitempty"`       // User's notes on selection
	Iterations int        `json:"iterations"`           // Refinement count
	ApprovedAt string     `json:"approved_at,omitempty"`
}

// UXCoverage maps mockup label -> capability ID -> coverage status
// Status values: "full" (✓), "partial" (~), "none" (✗), "" (unmarked)
type UXCoverage map[string]map[string]string

// UseCase describes a user scenario that the UX must satisfy.
type UseCase struct {
	ID      string `json:"id"`                // Short identifier (e.g., "UC1")
	Actor   string `json:"actor"`             // Who is performing the action
	Goal    string `json:"goal"`              // What they want to achieve
	Context string `json:"context,omitempty"` // When/where this happens
}

// UXMockup represents a visual mockup for a UX option.
type UXMockup struct {
	Label       string `json:"label"`       // "A", "B", "C"
	Description string `json:"description"` // Brief text description
	Fidelity    string `json:"fidelity"`    // ascii|html|styled
	Path        string `json:"path"`        // Relative to .pt/ux/{id}/
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// ============================================================================
// SYSTEM DISCOVERY TYPES - Project-level architecture before component UX
// ============================================================================

// SystemMap captures the complete enumeration of system components.
// Stored at project level in .pt/sysmap.json
type SystemMap struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Version     string      `json:"version"`
	Components  []Component `json:"components"`
	Edges       []Edge      `json:"edges"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
}

// Component represents a discrete unit in the system.
type Component struct {
	ID          string   `json:"id"`                    // e.g., "chain", "order"
	Name        string   `json:"name"`                  // Human-readable name
	Type        string   `json:"type"`                  // screen|command|api|service|store
	Category    string   `json:"category,omitempty"`    // view|action|data|util
	Description string   `json:"description,omitempty"` // What this component does
	Nouns       []string `json:"nouns,omitempty"`       // Domain objects involved
	Verbs       []string `json:"verbs,omitempty"`       // Actions performed
	Owner       string   `json:"owner,omitempty"`       // Role responsible
	TaskID      string   `json:"task_id,omitempty"`     // PT task ID for implementation
}

// Edge represents a dependency between components.
type Edge struct {
	From     string `json:"from"`              // Source component ID
	To       string `json:"to"`                // Target component ID
	Relation string `json:"relation"`          // calls|uses|triggers|provides|requires
	Label    string `json:"label,omitempty"`   // Optional description (e.g., data passed)
}

// UserJourney describes a complete flow through the system.
type UserJourney struct {
	ID        string        `json:"id"`                  // e.g., "J1"
	Name      string        `json:"name"`                // e.g., "Quick Trade Entry"
	Persona   string        `json:"persona,omitempty"`   // Who takes this journey
	Goal      string        `json:"goal"`                // What they want to achieve
	Trigger   string        `json:"trigger,omitempty"`   // What initiates this journey
	Steps     []JourneyStep `json:"steps"`
	Outcome   string        `json:"outcome,omitempty"`   // Success state description
	Frequency string        `json:"frequency,omitempty"` // daily|weekly|rare|error-case
	Priority  int           `json:"priority,omitempty"`  // 1=critical, 3=nice-to-have
}

// JourneyStep represents one step in a user journey.
type JourneyStep struct {
	Order       int      `json:"order"`                 // Sequence number
	Action      string   `json:"action"`                // What the user does
	Component   string   `json:"component"`             // Component ID from SystemMap
	Expectation string   `json:"expectation,omitempty"` // What user expects to see
	Branches    []Branch `json:"branches,omitempty"`    // Alternative paths
}

// Branch represents an alternative path from a step.
type Branch struct {
	Condition string `json:"condition"`  // When this branch is taken
	NextStep  int    `json:"next_step"`  // Step order to jump to
	Component string `json:"component"`  // Component for this branch
}

// ComponentScope defines the boundaries for a single component.
// Created per-task when claiming work on a component.
type ComponentScope struct {
	ComponentID   string      `json:"component_id"`            // From SystemMap
	TaskID        string      `json:"task_id"`                 // PT task this scope belongs to
	LiteMode      bool        `json:"lite_mode,omitempty"`     // Skip full discovery for simple tasks
	Inputs        []ScopeIO   `json:"inputs,omitempty"`        // What this component receives
	Outputs       []ScopeIO   `json:"outputs,omitempty"`       // What this component produces
	Preconditions []Condition `json:"preconditions,omitempty"` // Must be true before use
	OutOfScope    []Exclusion `json:"out_of_scope,omitempty"`  // Explicitly handled elsewhere
	Journeys      []string    `json:"journeys,omitempty"`      // Journey IDs this participates in
	Upstream      []string    `json:"upstream,omitempty"`      // Components that call/trigger this
	Downstream    []string    `json:"downstream,omitempty"`    // Components this calls/provides to
	CreatedAt     string      `json:"created_at"`
	ApprovedAt    string      `json:"approved_at,omitempty"`
}

// ScopeIO describes an input or output.
type ScopeIO struct {
	Name        string `json:"name"`                  // e.g., "symbol"
	Type        string `json:"type"`                  // string|int|struct|list|event
	Source      string `json:"source,omitempty"`      // Where it comes from
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Example     string `json:"example,omitempty"`     // Concrete example value
}

// Condition describes a precondition.
type Condition struct {
	Description string `json:"description"`
	Verifiable  bool   `json:"verifiable,omitempty"`  // Can be checked programmatically?
	CheckedBy   string `json:"checked_by,omitempty"`  // Component that validates this
}

// Exclusion explicitly states what is NOT in scope.
type Exclusion struct {
	Description string `json:"description"`          // What is excluded
	HandledBy   string `json:"handled_by,omitempty"` // Which component handles this
	Reason      string `json:"reason,omitempty"`     // Why it's excluded
}

// ============================================================================
// DISCOVERY WORKFLOW TYPES - State machine for UX exploration
// ============================================================================

// DiscoveryStatus represents phases in the discovery workflow.
// State machine: init → discovery → capabilities → exploring → synthesized → reviewing → approved
type DiscoveryStatus string

const (
	StatusInit         DiscoveryStatus = "init"         // Project initialized
	StatusDiscovery    DiscoveryStatus = "discovery"    // System/journey mapping
	StatusCapabilities DiscoveryStatus = "capabilities" // Use cases identified
	StatusExploring    DiscoveryStatus = "exploring"    // Options being generated
	StatusSynthesized  DiscoveryStatus = "synthesized"  // Top 3 ready for review
	StatusReviewing    DiscoveryStatus = "reviewing"    // User reviewing
	StatusFeedback     DiscoveryStatus = "feedback"     // User gave feedback, agent iterating
	StatusApproved     DiscoveryStatus = "approved"     // Ready for implementation
)

// ValidTransitions defines allowed state transitions in the discovery workflow.
var ValidTransitions = map[DiscoveryStatus][]DiscoveryStatus{
	StatusInit:         {StatusDiscovery},
	StatusDiscovery:    {StatusCapabilities},
	StatusCapabilities: {StatusExploring},
	StatusExploring:    {StatusSynthesized},
	StatusSynthesized:  {StatusReviewing},
	StatusReviewing:    {StatusFeedback, StatusApproved},
	StatusFeedback:     {StatusExploring, StatusSynthesized}, // iterate or re-synthesize
	StatusApproved:     {},                                    // terminal state
}

// CanTransition checks if a status transition is valid.
func (s DiscoveryStatus) CanTransition(to DiscoveryStatus) bool {
	allowed, ok := ValidTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// MockupComponent represents a labeled element in a mockup for precise feedback.
type MockupComponent struct {
	ID             string `json:"id"`                        // e.g., "A1", "B3.2"
	Type           string `json:"type"`                      // header, input, button, list, table, etc.
	Content        string `json:"content"`                   // Display text/description
	Implementation string `json:"implementation,omitempty"`  // e.g., "<Select> from shadcn/ui"
	Notes          string `json:"notes,omitempty"`           // Design rationale
}

// SynthesisOption represents one of the top 3 recommended options.
type SynthesisOption struct {
	Label       string            `json:"label"`       // "A", "B", "C"
	Name        string            `json:"name"`        // Short descriptive name
	Description string            `json:"description"` // What this option does
	Mockup      string            `json:"mockup"`      // Path to mockup file
	Components  []MockupComponent `json:"components"`  // Labeled components
	Coverage    int               `json:"coverage"`    // Capabilities covered
	Total       int               `json:"total"`       // Total capabilities
	Rationale   string            `json:"rationale"`   // Why this option
	Gaps        []string          `json:"gaps"`        // Uncovered capabilities
	Strengths   []string          `json:"strengths"`   // Key advantages
	Tradeoffs   []string          `json:"tradeoffs"`   // Known limitations
}

// RejectedOption documents an option that was considered but not recommended.
type RejectedOption struct {
	Name        string   `json:"name"`                  // What was considered
	Description string   `json:"description,omitempty"` // Brief description
	Reason      string   `json:"reason"`                // Why it was rejected
	Patterns    []string `json:"patterns,omitempty"`    // Patterns/approaches tried
}

// FeedbackItem tracks user feedback on specific components.
type FeedbackItem struct {
	ID          string `json:"id"`                     // Feedback ID (e.g., "F1")
	ComponentID string `json:"component_id,omitempty"` // Target component (e.g., "A7")
	OptionLabel string `json:"option_label,omitempty"` // Which option (e.g., "A")
	Feedback    string `json:"feedback"`               // User's comment
	Addressed   bool   `json:"addressed"`              // Has it been addressed?
	AddressedIn string `json:"addressed_in,omitempty"` // Which iteration addressed it
	CreatedAt   string `json:"created_at"`
}

// UXSynthesis is the comprehensive artifact for user review.
// Stored at .pt/synthesis/{component_id}.json
type UXSynthesis struct {
	ComponentID    string            `json:"component_id"`           // From SystemMap
	TaskID         string            `json:"task_id,omitempty"`      // PT task if linked
	UXType         string            `json:"ux_type"`                // cli|tui|web|api
	Status         DiscoveryStatus   `json:"status"`                 // Current phase
	Capabilities   []UseCase         `json:"capabilities"`           // What must be supported
	Recommendation string            `json:"recommendation"`         // Which option is recommended
	Options        []SynthesisOption `json:"options"`                // Top 3 options
	Rejected       []RejectedOption  `json:"rejected"`               // What was considered/rejected
	Exploration    ExplorationLog    `json:"exploration"`            // Full exploration history
	Feedback       []FeedbackItem    `json:"feedback,omitempty"`     // User feedback
	Iterations     int               `json:"iterations"`             // Refinement count
	CreatedAt      string            `json:"created_at"`
	SynthesizedAt  string            `json:"synthesized_at,omitempty"`
	ApprovedAt     string            `json:"approved_at,omitempty"`
}

// ExplorationLog tracks everything explored before synthesis.
type ExplorationLog struct {
	TotalOptions   int      `json:"total_options"`           // How many were explored
	Approaches     []string `json:"approaches"`              // Different approaches tried
	PatternsUsed   []string `json:"patterns_used,omitempty"` // Patterns from guidance
	TimeSpent      string   `json:"time_spent,omitempty"`    // Human-readable duration
	GuidanceUsed   string   `json:"guidance_used,omitempty"` // Which guidance file
}

// ExplorationGate defines minimum requirements before synthesis.
type ExplorationGate struct {
	MinOptions       int  `json:"min_options"`        // Default: 5
	MinApproaches    int  `json:"min_approaches"`     // Default: 2
	RequireIDs       bool `json:"require_ids"`        // Components must have IDs
	RequireCoverage  bool `json:"require_coverage"`   // Must track capability coverage
	RequireErrorCase bool `json:"require_error_case"` // Must show error handling
}

// DefaultExplorationGate returns standard exploration requirements.
func DefaultExplorationGate() ExplorationGate {
	return ExplorationGate{
		MinOptions:       5,
		MinApproaches:    2,
		RequireIDs:       true,
		RequireCoverage:  true,
		RequireErrorCase: true,
	}
}

// ImplementationGuidance provides type-specific implementation advice.
type ImplementationGuidance struct {
	UXType      string            `json:"ux_type"`      // cli|tui|web|api
	Libraries   []string          `json:"libraries"`    // Recommended libraries
	Patterns    map[string]string `json:"patterns"`     // Component -> pattern mapping
	Examples    map[string]string `json:"examples"`     // Component -> code example
	Constraints []string          `json:"constraints"`  // Must-follow rules
}

// HistoryEvent captures lifecycle events for a task.
type HistoryEvent struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"` // e.g., created, claimed, released, validated, approved, rejected, commented, synced
	Note   string    `json:"note,omitempty"`
}

// CommandRunner abstracts external command execution; allows mocking in tests.
type CommandRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner executes commands using os/exec.
type ExecRunner struct {
	// Dir is the working directory for commands. If empty, uses current working directory.
	Dir string
}

// Run executes a command with context.
func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no command provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	return cmd.CombinedOutput()
}

// Issue represents a task in the store.
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Assignee    string   `json:"assignee"`
	Priority    int      `json:"priority"`
	IssueType   string   `json:"issue_type"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	NextHint    string   `json:"next_hint,omitempty"`
}

// Dependency represents a blocking relationship.
type Dependency struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// UpdateOptions specifies which fields to update on a task.
// Only non-empty fields are applied.
type UpdateOptions struct {
	Title    string `json:"title,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Priority *int   `json:"priority,omitempty"`
	NextHint string `json:"next_hint,omitempty"`

	// Handoff fields - use special value "-" to clear
	Context   string   `json:"context,omitempty"`
	Inputs    []string `json:"inputs,omitempty"`
	Scope     string   `json:"scope,omitempty"`
	Reference string   `json:"reference,omitempty"`
}

// BlockedInfo tracks why a task is blocked.
type BlockedInfo struct {
	Reason    string `json:"reason"`
	BlockedBy string `json:"blocked_by,omitempty"` // optional: who/what blocked it
	BlockedAt string `json:"blocked_at,omitempty"` // timestamp
}

// WorktreeInfo tracks a git worktree associated with a task.
type WorktreeInfo struct {
	TaskID    string `json:"task_id"`
	Path      string `json:"path"`       // worktree directory path
	Branch    string `json:"branch"`     // branch name in the worktree
	CreatedAt string `json:"created_at"` // timestamp
}

// buildDescription embeds metadata for later retrieval.
func buildDescription(task Task) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Template: %s\nRole: %s\n", task.Template, task.Role)
	if strings.TrimSpace(task.Artifact) != "" {
		fmt.Fprintf(&b, "Artifact: %s\n", task.Artifact)
	}
	if len(task.Params) > 0 {
		fmt.Fprintf(&b, "Params: %+v\n", task.Params)
	}
	fmt.Fprintf(&b, "DoD:")
	if len(task.DoD.Tests) > 0 {
		fmt.Fprintf(&b, " tests=%v", task.DoD.Tests)
	}
	if task.DoD.ValidationCmd != "" {
		fmt.Fprintf(&b, " validation_cmd=%s", task.DoD.ValidationCmd)
	}
	if task.DoD.Manual != "" {
		fmt.Fprintf(&b, " manual=%s", task.DoD.Manual)
	}
	if len(task.DoD.Criteria) > 0 {
		fmt.Fprintf(&b, " criteria=%v", task.DoD.Criteria)
	}
	if task.DoD.OnFailure != "" {
		fmt.Fprintf(&b, " on_failure=%s", task.DoD.OnFailure)
	}
	meta := TaskMeta{
		Template:  task.Template,
		Role:      task.Role,
		NextHint:  task.NextHint,
		Artifact:  task.Artifact,
		DoD:       task.DoD,
		Context:   task.Context,
		Inputs:    task.Inputs,
		Scope:     task.Scope,
		Reference: task.Reference,
		UX:        task.UX,
		MaxHours:  task.MaxHours,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "\n<!-- pt-meta: %s -->", metaJSON)
	return b.String(), nil
}

// parseTaskMeta extracts TaskMeta from description marker.
func parseTaskMeta(desc string) (TaskMeta, error) {
	start := strings.Index(desc, "<!-- pt-meta:")
	end := strings.Index(desc, "-->")
	if start == -1 || end == -1 || end <= start {
		return TaskMeta{}, errors.New("pt-meta not found in description")
	}
	payload := strings.TrimSpace(desc[start+len("<!-- pt-meta:") : end])
	var meta TaskMeta
	if err := json.Unmarshal([]byte(payload), &meta); err != nil {
		return TaskMeta{}, fmt.Errorf("parse pt-meta: %w", err)
	}
	return meta, nil
}

// ParseTaskMeta is an exported wrapper around parseTaskMeta for consumers that
// need to extract metadata from an issue description.
func ParseTaskMeta(desc string) (TaskMeta, error) {
	return parseTaskMeta(desc)
}

// ContextWithTimeout is a helper to create a context with a sensible default timeout.
func ContextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 10 * time.Second
	}
	return context.WithTimeout(parent, d)
}

// RunCommand is a convenience wrapper for ExecRunner; used by CommandRunner interface.
// It mirrors ExecRunner.Run but kept for clarity.
func RunCommand(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no command provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	return cmd.CombinedOutput()
}
