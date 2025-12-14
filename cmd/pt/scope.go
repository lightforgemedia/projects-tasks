package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

// scopesDir returns the path to the scopes directory
func scopesDir() string {
	return filepath.Join(".pt", "scopes")
}

// scopeFile returns the path to a specific scope file
func scopeFile(taskID string) string {
	return filepath.Join(scopesDir(), taskID+".json")
}

// loadScope loads a scope from disk
func loadScope(taskID string) (*pt.ComponentScope, error) {
	data, err := os.ReadFile(scopeFile(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s pt.ComponentScope
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// saveScope persists a scope to disk
func saveScope(s *pt.ComponentScope) error {
	if err := os.MkdirAll(scopesDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(scopeFile(s.TaskID), data, 0644)
}

// cmdScope handles the scope command
func cmdScope(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pt scope [init|input|output|exclude|show|approve] <task-id>")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "init":
		return cmdScopeInit(subArgs)
	case "input":
		return cmdScopeInput(subArgs)
	case "output":
		return cmdScopeOutput(subArgs)
	case "exclude":
		return cmdScopeExclude(subArgs)
	case "show":
		return cmdScopeShow(subArgs)
	case "approve":
		return cmdScopeApprove(subArgs)
	case "ready":
		return cmdScopeReady(subArgs)
	default:
		return fmt.Errorf("unknown scope subcommand: %s\nUsage: pt scope [init|input|output|exclude|show|approve|ready]", subCmd)
	}
}

// cmdScopeInit initializes a scope for a task
func cmdScopeInit(args []string) error {
	fs := flag.NewFlagSet("scope init", flag.ExitOnError)
	componentID := fs.String("component", "", "Component ID from system map")
	lite := fs.Bool("lite", false, "Lite mode: skip full discovery, auto-approve")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt scope init <task-id> [--component <id>] [--lite]")
	}

	taskID := fs.Arg(0)

	// Check if scope already exists
	existing, _ := loadScope(taskID)
	if existing != nil {
		fmt.Printf("Scope already exists for %s\n", taskID)
		fmt.Printf("Use 'pt scope show %s' to view it.\n", taskID)
		return nil
	}

	// Load system map to get upstream/downstream
	sm, _ := loadSystemMap()
	var upstream, downstream []string
	compID := *componentID

	if sm != nil && compID != "" {
		// Find edges
		for _, e := range sm.Edges {
			if e.To == compID {
				upstream = append(upstream, e.From)
			}
			if e.From == compID {
				downstream = append(downstream, e.To)
			}
		}
	}

	// Find journeys that include this component
	journeys, _ := loadJourneys()
	var journeyIDs []string
	for _, j := range journeys {
		for _, step := range j.Steps {
			if step.Component == compID {
				journeyIDs = append(journeyIDs, j.ID)
				break
			}
		}
	}

	scope := &pt.ComponentScope{
		ComponentID: compID,
		TaskID:      taskID,
		LiteMode:    *lite,
		Upstream:    upstream,
		Downstream:  downstream,
		Journeys:    journeyIDs,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	// Auto-approve in lite mode
	if *lite {
		scope.ApprovedAt = time.Now().Format(time.RFC3339)
	}

	if err := saveScope(scope); err != nil {
		return err
	}

	if *lite {
		fmt.Printf("✓ Scope initialized (LITE MODE) for %s\n", taskID)
		fmt.Printf("  Auto-approved - skipping full discovery\n")
		fmt.Printf("\nProceed directly to UX:\n")
		fmt.Printf("  pt ux-cases %s\n", taskID)
	} else {
		fmt.Printf("✓ Scope initialized for %s\n", taskID)
		if compID != "" {
			fmt.Printf("  Component: %s\n", compID)
			if len(upstream) > 0 {
				fmt.Printf("  Upstream: %s\n", strings.Join(upstream, ", "))
			}
			if len(downstream) > 0 {
				fmt.Printf("  Downstream: %s\n", strings.Join(downstream, ", "))
			}
			if len(journeyIDs) > 0 {
				fmt.Printf("  Journeys: %s\n", strings.Join(journeyIDs, ", "))
			}
		}

		fmt.Println("\nDefine boundaries:")
		fmt.Printf("  pt scope input --name 'symbol' --type string --required %s\n", taskID)
		fmt.Printf("  pt scope output --name 'order_id' --type string %s\n", taskID)
		fmt.Printf("  pt scope exclude --desc 'Chain fetching' --handled-by chain %s\n", taskID)
	}
	return nil
}

// cmdScopeInput adds an input to a scope
func cmdScopeInput(args []string) error {
	fs := flag.NewFlagSet("scope input", flag.ExitOnError)
	name := fs.String("name", "", "Input name")
	inputType := fs.String("type", "string", "Type: string|int|float|bool|struct|list")
	source := fs.String("source", "", "Source: user|component-id|config")
	desc := fs.String("desc", "", "Description")
	required := fs.Bool("required", false, "Is this input required?")
	example := fs.String("example", "", "Example value")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt scope input --name 'x' --type string [--required] <task-id>")
	}

	taskID := fs.Arg(0)

	scope, err := loadScope(taskID)
	if err != nil || scope == nil {
		return fmt.Errorf("scope not found for %s; run 'pt scope init %s' first", taskID, taskID)
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	input := pt.ScopeIO{
		Name:        *name,
		Type:        *inputType,
		Source:      *source,
		Description: *desc,
		Required:    *required,
		Example:     *example,
	}

	scope.Inputs = append(scope.Inputs, input)

	if err := saveScope(scope); err != nil {
		return err
	}

	reqStr := ""
	if *required {
		reqStr = " (required)"
	}
	fmt.Printf("✓ Added input: %s (%s)%s\n", *name, *inputType, reqStr)
	return nil
}

// cmdScopeOutput adds an output to a scope
func cmdScopeOutput(args []string) error {
	fs := flag.NewFlagSet("scope output", flag.ExitOnError)
	name := fs.String("name", "", "Output name")
	outputType := fs.String("type", "string", "Type: string|int|float|bool|struct|list")
	desc := fs.String("desc", "", "Description")
	example := fs.String("example", "", "Example value")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt scope output --name 'x' --type string <task-id>")
	}

	taskID := fs.Arg(0)

	scope, err := loadScope(taskID)
	if err != nil || scope == nil {
		return fmt.Errorf("scope not found for %s; run 'pt scope init %s' first", taskID, taskID)
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	output := pt.ScopeIO{
		Name:        *name,
		Type:        *outputType,
		Description: *desc,
		Example:     *example,
	}

	scope.Outputs = append(scope.Outputs, output)

	if err := saveScope(scope); err != nil {
		return err
	}

	fmt.Printf("✓ Added output: %s (%s)\n", *name, *outputType)
	return nil
}

// cmdScopeExclude adds an exclusion to a scope
func cmdScopeExclude(args []string) error {
	fs := flag.NewFlagSet("scope exclude", flag.ExitOnError)
	desc := fs.String("desc", "", "What is excluded")
	handledBy := fs.String("handled-by", "", "Which component handles this")
	reason := fs.String("reason", "", "Why it's excluded")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt scope exclude --desc 'X' --handled-by <component> <task-id>")
	}

	taskID := fs.Arg(0)

	scope, err := loadScope(taskID)
	if err != nil || scope == nil {
		return fmt.Errorf("scope not found for %s; run 'pt scope init %s' first", taskID, taskID)
	}

	if *desc == "" {
		return fmt.Errorf("--desc is required")
	}

	exclusion := pt.Exclusion{
		Description: *desc,
		HandledBy:   *handledBy,
		Reason:      *reason,
	}

	scope.OutOfScope = append(scope.OutOfScope, exclusion)

	if err := saveScope(scope); err != nil {
		return err
	}

	fmt.Printf("✓ Excluded: %s", *desc)
	if *handledBy != "" {
		fmt.Printf(" (→ %s)", *handledBy)
	}
	fmt.Println()
	return nil
}

// cmdScopeShow displays a scope
func cmdScopeShow(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: pt scope show <task-id>")
	}

	taskID := args[0]

	scope, err := loadScope(taskID)
	if err != nil || scope == nil {
		return fmt.Errorf("scope not found for %s", taskID)
	}

	modeStr := ""
	if scope.LiteMode {
		modeStr = " (LITE)"
	}
	fmt.Printf("Component Scope: %s%s\n", taskID, modeStr)
	if scope.ComponentID != "" {
		fmt.Printf("Component: %s\n", scope.ComponentID)
	}
	fmt.Println(strings.Repeat("═", 60))

	// Inputs
	if len(scope.Inputs) > 0 {
		fmt.Println("\nINPUTS:")
		for _, in := range scope.Inputs {
			req := ""
			if in.Required {
				req = " *"
			}
			fmt.Printf("  • %s (%s)%s", in.Name, in.Type, req)
			if in.Source != "" {
				fmt.Printf(" ← %s", in.Source)
			}
			fmt.Println()
			if in.Description != "" {
				fmt.Printf("    %s\n", in.Description)
			}
		}
	}

	// Outputs
	if len(scope.Outputs) > 0 {
		fmt.Println("\nOUTPUTS:")
		for _, out := range scope.Outputs {
			fmt.Printf("  • %s (%s)\n", out.Name, out.Type)
			if out.Description != "" {
				fmt.Printf("    %s\n", out.Description)
			}
		}
	}

	// Out of scope
	if len(scope.OutOfScope) > 0 {
		fmt.Println("\nOUT OF SCOPE:")
		for _, ex := range scope.OutOfScope {
			fmt.Printf("  ✗ %s", ex.Description)
			if ex.HandledBy != "" {
				fmt.Printf(" → %s", ex.HandledBy)
			}
			fmt.Println()
			if ex.Reason != "" {
				fmt.Printf("    Reason: %s\n", ex.Reason)
			}
		}
	}

	// Adjacent components
	if len(scope.Upstream) > 0 || len(scope.Downstream) > 0 {
		fmt.Println("\nADJACENT:")
		if len(scope.Upstream) > 0 {
			fmt.Printf("  ← Upstream: %s\n", strings.Join(scope.Upstream, ", "))
		}
		if len(scope.Downstream) > 0 {
			fmt.Printf("  → Downstream: %s\n", strings.Join(scope.Downstream, ", "))
		}
	}

	// Journeys
	if len(scope.Journeys) > 0 {
		fmt.Printf("\nJOURNEYS: %s\n", strings.Join(scope.Journeys, ", "))
	}

	// Approval status
	fmt.Println()
	if scope.ApprovedAt != "" {
		fmt.Printf("✓ Approved: %s\n", scope.ApprovedAt)
	} else {
		fmt.Println("⚠ Not approved. Run: pt scope approve " + taskID)
	}

	return nil
}

// cmdScopeApprove locks a scope
func cmdScopeApprove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: pt scope approve <task-id>")
	}

	taskID := args[0]

	scope, err := loadScope(taskID)
	if err != nil || scope == nil {
		return fmt.Errorf("scope not found for %s", taskID)
	}

	if scope.ApprovedAt != "" {
		fmt.Printf("Scope already approved at %s\n", scope.ApprovedAt)
		return nil
	}

	// Validate scope has minimum content
	issues := []string{}
	if len(scope.Inputs) == 0 {
		issues = append(issues, "No inputs defined")
	}
	if len(scope.Outputs) == 0 {
		issues = append(issues, "No outputs defined")
	}

	if len(issues) > 0 {
		fmt.Println("⚠ Scope validation warnings:")
		for _, issue := range issues {
			fmt.Printf("  - %s\n", issue)
		}
		fmt.Println("\nApprove anyway? Add --force to approve with warnings")
		// For now, allow approval with warnings
	}

	scope.ApprovedAt = time.Now().Format(time.RFC3339)

	if err := saveScope(scope); err != nil {
		return err
	}

	fmt.Printf("✓ Scope approved for %s\n", taskID)
	fmt.Println("You can now proceed to UX discovery:")
	fmt.Printf("  pt ux-cases %s\n", taskID)
	return nil
}

// cmdScopeReady checks if a scope meets exit criteria for proceeding to UX
func cmdScopeReady(args []string) error {
	fs := flag.NewFlagSet("scope ready", flag.ExitOnError)
	strict := fs.Bool("strict", false, "Exit with error code 1 if not ready (for CI)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt scope ready <task-id> [--strict]")
	}

	taskID := fs.Arg(0)

	scope, err := loadScope(taskID)
	if err != nil || scope == nil {
		if *strict {
			fmt.Printf("✗ Scope not found for %s\n", taskID)
			os.Exit(1)
		}
		return fmt.Errorf("scope not found for %s", taskID)
	}

	// Lite mode has relaxed criteria
	if scope.LiteMode {
		fmt.Printf("Scope Exit Criteria (%s) [LITE MODE]:\n", taskID)
		fmt.Println(strings.Repeat("─", 50))
		fmt.Println("  ✓ Lite mode - full discovery skipped")
		fmt.Println("  ✓ Auto-approved")
		fmt.Println()
		fmt.Println("✓ READY: Lite scope approved. Proceed to UX capabilities.")
		fmt.Printf("  Next: pt ux-cases %s\n", taskID)
		return nil
	}

	// Exit criteria for SCOPE phase (full mode)
	type scopeCriteria struct {
		name   string
		met    bool
		detail string
	}

	criteria := []scopeCriteria{
		{
			name:   "At least 1 input defined",
			met:    len(scope.Inputs) >= 1,
			detail: fmt.Sprintf("found %d", len(scope.Inputs)),
		},
		{
			name:   "At least 1 output defined",
			met:    len(scope.Outputs) >= 1,
			detail: fmt.Sprintf("found %d", len(scope.Outputs)),
		},
		{
			name:   "Linked to component in system map",
			met:    scope.ComponentID != "",
			detail: scope.ComponentID,
		},
		{
			name:   "Scope is approved",
			met:    scope.ApprovedAt != "",
			detail: "",
		},
	}

	// Report
	allMet := true
	fmt.Printf("Scope Exit Criteria (%s):\n", taskID)
	fmt.Println(strings.Repeat("─", 50))
	for _, c := range criteria {
		status := "✓"
		if !c.met {
			status = "✗"
			allMet = false
		}
		fmt.Printf("  %s %s", status, c.name)
		if c.detail != "" && c.met {
			fmt.Printf(" (%s)", c.detail)
		}
		fmt.Println()
	}

	fmt.Println()
	if allMet {
		fmt.Println("✓ READY: Scope complete. Proceed to UX capabilities.")
		fmt.Printf("  Next: pt ux-cases %s\n", taskID)
	} else {
		fmt.Println("✗ NOT READY: Address issues above before proceeding.")
		if *strict {
			os.Exit(1)
		}
	}

	return nil
}
