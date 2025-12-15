package pt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Workflow represents a development process template with ordered phases.
type Workflow struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	PhaseAssignment PhaseAssignment `json:"phase_assignment"`
	Phases          []Phase         `json:"phases"`
}

// PhaseAssignment defines how tasks are assigned to phases.
type PhaseAssignment struct {
	DefaultPhase string            `json:"default_phase"`
	ByTemplate   map[string]string `json:"by_template,omitempty"` // template -> phase
	LabelPrefix  string            `json:"label_prefix"`          // e.g., "phase:"
}

// Phase represents a stage in the workflow.
type Phase struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Order       int         `json:"order"`
	Description string      `json:"description,omitempty"`
	Gate        Gate        `json:"gate,omitempty"`
	Checkpoint  *Checkpoint `json:"checkpoint,omitempty"`
	Proof       *Proof      `json:"proof,omitempty"`
}

// Gate defines conditions for advancing past a phase.
type Gate struct {
	Type          string `json:"type"`                     // "soft" or "hard"
	Condition     string `json:"condition"`                // e.g., "all_closed", "has_comment:user-approved"
	ReminderBefore string `json:"reminder_before,omitempty"`
	ReminderAfter  string `json:"reminder_after,omitempty"`
	BlockMessage   string `json:"block_message,omitempty"` // for hard gates
}

// Checkpoint defines a user interaction point in a phase.
type Checkpoint struct {
	Trigger string `json:"trigger"` // e.g., "first_task_complete"
	Prompt  string `json:"prompt"`
}

// Proof defines expected evidence for task completion.
type Proof struct {
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Hint        string `json:"hint,omitempty"`
}

// PhaseStatus represents the current state of a phase.
type PhaseStatus struct {
	Phase       Phase   `json:"phase"`
	Tasks       []Issue `json:"tasks"`
	TotalTasks  int     `json:"total_tasks"`
	ClosedTasks int     `json:"closed_tasks"`
	IsBlocked   bool    `json:"is_blocked"`
	BlockReason string  `json:"block_reason,omitempty"`
	IsCurrent   bool    `json:"is_current"`
}

// WorkflowStatus represents the overall workflow state.
type WorkflowStatus struct {
	Workflow     Workflow      `json:"workflow"`
	Phases       []PhaseStatus `json:"phases"`
	SuggestedNext *Issue       `json:"suggested_next,omitempty"`
	NextReason    string       `json:"next_reason,omitempty"`
}

// ParseWorkflow reads and validates a workflow file (TOML format).
func ParseWorkflow(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, fmt.Errorf("read workflow: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".toml" {
		return Workflow{}, fmt.Errorf("workflow must be .toml, got %q", ext)
	}

	return parseTOMLWorkflow(data)
}

// parseTOMLWorkflow parses workflow TOML format.
func parseTOMLWorkflow(data []byte) (Workflow, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	workflow := Workflow{
		PhaseAssignment: PhaseAssignment{
			ByTemplate: make(map[string]string),
		},
	}
	var phases []Phase
	var currentPhase *Phase
	context := "root" // root | phase_assignment | by_template | phase | gate | checkpoint | proof

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := strings.TrimSpace(stripInlineComment(scanner.Text()))
		if raw == "" {
			continue
		}

		// Handle section headers
		if strings.HasPrefix(raw, "[[phases]]") {
			if currentPhase != nil {
				phases = append(phases, *currentPhase)
			}
			currentPhase = &Phase{}
			context = "phase"
			continue
		}
		if strings.HasPrefix(raw, "[phase_assignment.by_template]") {
			context = "by_template"
			continue
		}
		if strings.HasPrefix(raw, "[phase_assignment]") {
			context = "phase_assignment"
			continue
		}
		if strings.HasPrefix(raw, "[phases.gate]") {
			if currentPhase == nil {
				return Workflow{}, fmt.Errorf("line %d: gate before phase", lineNum)
			}
			context = "gate"
			continue
		}
		if strings.HasPrefix(raw, "[phases.checkpoint]") {
			if currentPhase == nil {
				return Workflow{}, fmt.Errorf("line %d: checkpoint before phase", lineNum)
			}
			if currentPhase.Checkpoint == nil {
				currentPhase.Checkpoint = &Checkpoint{}
			}
			context = "checkpoint"
			continue
		}
		if strings.HasPrefix(raw, "[phases.proof]") {
			if currentPhase == nil {
				return Workflow{}, fmt.Errorf("line %d: proof before phase", lineNum)
			}
			if currentPhase.Proof == nil {
				currentPhase.Proof = &Proof{}
			}
			context = "proof"
			continue
		}

		// Parse key-value
		key, val, err := parseKV(raw)
		if err != nil {
			return Workflow{}, fmt.Errorf("line %d: %w", lineNum, err)
		}

		switch context {
		case "root":
			switch key {
			case "name":
				workflow.Name = val.asString()
			case "description":
				workflow.Description = val.asString()
			}
		case "phase_assignment":
			switch key {
			case "default_phase":
				workflow.PhaseAssignment.DefaultPhase = val.asString()
			case "label_prefix":
				workflow.PhaseAssignment.LabelPrefix = val.asString()
			}
		case "by_template":
			workflow.PhaseAssignment.ByTemplate[key] = val.asString()
		case "phase":
			if currentPhase == nil {
				return Workflow{}, fmt.Errorf("line %d: phase value without context", lineNum)
			}
			switch key {
			case "id":
				currentPhase.ID = val.asString()
			case "name":
				currentPhase.Name = val.asString()
			case "order":
				order, err := strconv.Atoi(val.asString())
				if err != nil {
					return Workflow{}, fmt.Errorf("line %d: order must be integer", lineNum)
				}
				currentPhase.Order = order
			case "description":
				currentPhase.Description = val.asString()
			}
		case "gate":
			if currentPhase == nil {
				return Workflow{}, fmt.Errorf("line %d: gate value without phase", lineNum)
			}
			switch key {
			case "type":
				currentPhase.Gate.Type = val.asString()
			case "condition":
				currentPhase.Gate.Condition = val.asString()
			case "reminder_before":
				currentPhase.Gate.ReminderBefore = val.asString()
			case "reminder_after":
				currentPhase.Gate.ReminderAfter = val.asString()
			case "block_message":
				currentPhase.Gate.BlockMessage = val.asString()
			}
		case "checkpoint":
			if currentPhase == nil || currentPhase.Checkpoint == nil {
				return Workflow{}, fmt.Errorf("line %d: checkpoint value without context", lineNum)
			}
			switch key {
			case "trigger":
				currentPhase.Checkpoint.Trigger = val.asString()
			case "prompt":
				currentPhase.Checkpoint.Prompt = val.asString()
			}
		case "proof":
			if currentPhase == nil || currentPhase.Proof == nil {
				return Workflow{}, fmt.Errorf("line %d: proof value without context", lineNum)
			}
			switch key {
			case "required":
				currentPhase.Proof.Required = val.asString() == "true"
			case "description":
				currentPhase.Proof.Description = val.asString()
			case "hint":
				currentPhase.Proof.Hint = val.asString()
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Workflow{}, fmt.Errorf("scan workflow: %w", err)
	}

	if currentPhase != nil {
		phases = append(phases, *currentPhase)
	}

	// Sort phases by order
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Order < phases[j].Order
	})

	workflow.Phases = phases
	return workflow, validateWorkflow(workflow)
}

func validateWorkflow(w Workflow) error {
	if strings.TrimSpace(w.Name) == "" {
		return errors.New("workflow name is required")
	}
	if len(w.Phases) == 0 {
		return errors.New("workflow must have at least one phase")
	}
	ids := make(map[string]bool)
	for _, p := range w.Phases {
		if strings.TrimSpace(p.ID) == "" {
			return errors.New("phase id is required")
		}
		if ids[p.ID] {
			return fmt.Errorf("duplicate phase id %q", p.ID)
		}
		ids[p.ID] = true
		if p.Gate.Type != "" && p.Gate.Type != "soft" && p.Gate.Type != "hard" {
			return fmt.Errorf("phase %q: gate type must be 'soft' or 'hard'", p.ID)
		}
	}
	return nil
}

// GetTaskPhase determines which phase a task belongs to.
func (w *Workflow) GetTaskPhase(issue Issue, meta TaskMeta) string {
	// Priority 1: Explicit label
	if w.PhaseAssignment.LabelPrefix != "" {
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, w.PhaseAssignment.LabelPrefix) {
				return strings.TrimPrefix(label, w.PhaseAssignment.LabelPrefix)
			}
		}
	}

	// Priority 2: Template mapping
	if phase, ok := w.PhaseAssignment.ByTemplate[meta.Template]; ok {
		return phase
	}

	// Priority 3: Default
	if w.PhaseAssignment.DefaultPhase != "" {
		return w.PhaseAssignment.DefaultPhase
	}

	return "unassigned"
}

// GetPhaseByID returns a phase by its ID.
func (w *Workflow) GetPhaseByID(id string) *Phase {
	for i := range w.Phases {
		if w.Phases[i].ID == id {
			return &w.Phases[i]
		}
	}
	return nil
}

// EvaluateGate checks if a phase's gate condition is satisfied.
// Returns (satisfied, reason).
func (w *Workflow) EvaluateGate(phase Phase, phaseTasks []Issue, allIssues []Issue, comments map[string][]string) (bool, string) {
	if phase.Gate.Condition == "" {
		return true, ""
	}

	// Soft-gate override: if any task comment contains "gate-override:<phase-id>",
	// treat the gate as satisfied. This provides an explicit, auditable bypass.
	if phase.Gate.Type == "soft" && hasGateOverride(phase.ID, comments) {
		return true, fmt.Sprintf("soft gate overridden for phase %q", phase.ID)
	}

	condition := phase.Gate.Condition

	// Handle "all_closed" condition
	if condition == "all_closed" {
		openCount := 0
		for _, t := range phaseTasks {
			if t.Status != "closed" && t.Status != "done" {
				openCount++
			}
		}
		if openCount > 0 {
			return false, fmt.Sprintf("%d tasks still open in phase %q", openCount, phase.ID)
		}
		return true, ""
	}

	// Handle "phase:<id> complete" condition
	if strings.HasPrefix(condition, "phase:") && strings.HasSuffix(condition, " complete") {
		reqPhase := strings.TrimPrefix(condition, "phase:")
		reqPhase = strings.TrimSuffix(reqPhase, " complete")
		// Check if all tasks in required phase are closed
		for _, issue := range allIssues {
			meta, _ := parseTaskMeta(issue.Description)
			if w.GetTaskPhase(issue, meta) == reqPhase {
				if issue.Status != "closed" && issue.Status != "done" {
					return false, fmt.Sprintf("phase %q not complete", reqPhase)
				}
			}
		}
		return true, ""
	}

	// Handle "has_comment:<tag>" condition (searches current phase tasks)
	if strings.HasPrefix(condition, "has_comment:") {
		tag := strings.TrimPrefix(condition, "has_comment:")
		for _, t := range phaseTasks {
			if taskComments, ok := comments[t.ID]; ok {
				for _, c := range taskComments {
					if strings.Contains(strings.ToLower(c), strings.ToLower(tag)) {
						return true, ""
					}
				}
			}
		}
		return false, fmt.Sprintf("no comment containing %q found", tag)
	}

	// Handle "phase:<id> has_comment:<tag>" condition (searches specific phase)
	if strings.HasPrefix(condition, "phase:") && strings.Contains(condition, " has_comment:") {
		parts := strings.SplitN(condition, " has_comment:", 2)
		reqPhase := strings.TrimPrefix(parts[0], "phase:")
		tag := parts[1]
		for _, issue := range allIssues {
			meta, _ := parseTaskMeta(issue.Description)
			if w.GetTaskPhase(issue, meta) == reqPhase {
				if taskComments, ok := comments[issue.ID]; ok {
					for _, c := range taskComments {
						if strings.Contains(strings.ToLower(c), strings.ToLower(tag)) {
							return true, ""
						}
					}
				}
			}
		}
		return false, fmt.Sprintf("no comment containing %q found in phase %q", tag, reqPhase)
	}

	// Handle "discovery:approved:<component-id>" condition (searches current phase tasks)
	if strings.HasPrefix(condition, "discovery:approved:") {
		componentID := strings.TrimPrefix(condition, "discovery:approved:")
		expectedComment := fmt.Sprintf("discovery-approved: %s", componentID)
		for _, t := range phaseTasks {
			if taskComments, ok := comments[t.ID]; ok {
				for _, c := range taskComments {
					if strings.Contains(strings.ToLower(c), strings.ToLower(expectedComment)) {
						return true, ""
					}
				}
			}
		}
		return false, fmt.Sprintf("discovery not approved for component %q", componentID)
	}

	// Handle compound conditions with OR
	if strings.Contains(condition, " OR ") {
		parts := strings.Split(condition, " OR ")
		for _, part := range parts {
			subPhase := phase
			subPhase.Gate.Condition = strings.TrimSpace(part)
			satisfied, _ := w.EvaluateGate(subPhase, phaseTasks, allIssues, comments)
			if satisfied {
				return true, ""
			}
		}
		return false, "no OR condition satisfied"
	}

	// Unknown condition - fail closed with clear error
	return false, fmt.Sprintf("unknown gate condition %q (supported: all_closed, phase:X complete, has_comment:X, phase:X has_comment:Y, discovery:approved:X)", condition)
}

// CheckGate evaluates a gate for a specific task.
// Returns: (canProceed, isHardBlock, blockingPhaseID, message)
func (w *Workflow) CheckGate(taskID string, issue Issue, meta TaskMeta, allIssues []Issue, comments map[string][]string) (bool, bool, string, string) {
	phaseID := w.GetTaskPhase(issue, meta)
	phase := w.GetPhaseByID(phaseID)
	if phase == nil {
		return true, false, "", "" // No phase, no gate
	}

	// Find the phase index
	phaseIdx := -1
	for i, p := range w.Phases {
		if p.ID == phaseID {
			phaseIdx = i
			break
		}
	}

	// Check all prior phases' gates
	for i := 0; i < phaseIdx; i++ {
		priorPhase := w.Phases[i]

		// Collect tasks in prior phase
		var phaseTasks []Issue
		for _, iss := range allIssues {
			m, _ := parseTaskMeta(iss.Description)
			if w.GetTaskPhase(iss, m) == priorPhase.ID {
				phaseTasks = append(phaseTasks, iss)
			}
		}

		satisfied, reason := w.EvaluateGate(priorPhase, phaseTasks, allIssues, comments)
		if !satisfied {
			isHard := priorPhase.Gate.Type == "hard"
			msg := priorPhase.Gate.ReminderBefore
			if isHard && priorPhase.Gate.BlockMessage != "" {
				msg = priorPhase.Gate.BlockMessage
			}
			if msg == "" {
				msg = reason
			}
			return false, isHard, priorPhase.ID, msg
		}
	}

	// Also check the current phase's own gate as an entry gate (e.g., "phase:frontend has_comment:user-approved").
	// IMPORTANT: For the first phase (index 0), the phase gate is treated as an *exit gate* that blocks later phases,
	// not as an entry gate that blocks work within the phase itself.
	if phaseIdx > 0 && phase.Gate.Condition != "" {
		// Collect tasks in current phase
		var currentPhaseTasks []Issue
		for _, iss := range allIssues {
			m, _ := parseTaskMeta(iss.Description)
			if w.GetTaskPhase(iss, m) == phaseID {
				currentPhaseTasks = append(currentPhaseTasks, iss)
			}
		}

		satisfied, reason := w.EvaluateGate(*phase, currentPhaseTasks, allIssues, comments)
		if !satisfied {
			isHard := phase.Gate.Type == "hard"
			msg := phase.Gate.ReminderBefore
			if isHard && phase.Gate.BlockMessage != "" {
				msg = phase.Gate.BlockMessage
			}
			if msg == "" {
				msg = reason
			}
			return false, isHard, phase.ID, msg
		}
	}

	return true, false, "", ""
}

func hasGateOverride(phaseID string, comments map[string][]string) bool {
	if strings.TrimSpace(phaseID) == "" || len(comments) == 0 {
		return false
	}

	needle1 := strings.ToLower("gate-override:" + phaseID)
	needle2 := strings.ToLower("gate-override: " + phaseID)

	for _, cs := range comments {
		for _, c := range cs {
			lc := strings.ToLower(c)
			if strings.Contains(lc, needle1) || strings.Contains(lc, needle2) {
				return true
			}
		}
	}
	return false
}
