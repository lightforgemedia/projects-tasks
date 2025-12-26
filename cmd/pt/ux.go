package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pt "projects-tasks/pkg/pt"
)

func cmdUXCases(args []string) error {
	fs := flag.NewFlagSet("ux-cases", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	addCase := fs.String("add", "", "add capability: 'category: description'")
	uxType := fs.String("type", "", "enable UX by setting type: cli|tui|web|api|doc|error")
	confirm := fs.Bool("confirm", false, "confirm and proceed to explore")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-cases <id>")
		fmt.Println("\nDiscover capabilities needed for a UX-enabled task.")
		fmt.Println("Use cases surface ALL features, not just obvious ones.")
		fmt.Println("\nFlags:")
		fmt.Println("  --add 'capability'          Add capability")
		fmt.Println("  --add 'category: cap'       Add with category (action/info/error/pattern)")
		fmt.Println("  --type <type>               Enable UX by setting type (cli|tui|web|api|doc|error)")
		fmt.Println("  --confirm                   Confirm and proceed to explore")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-cases --add 'action: Place 1-4 leg order' pt-42")
		fmt.Println("  pt ux-cases --add 'error: Handle partial fills' pt-42")
		fmt.Println("  pt ux-cases --type web pt-42     # enable UX for a task created via pt add")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	// Ensure UX is enabled (manifest-driven or explicitly enabled via --type).
	// This avoids a common footgun: tasks created via `pt add` have no [tasks.ux] field.
	meta, changed, err := ensureUXEnabled(id, meta, *uxType)
	if err != nil {
		return err
	}
	if changed {
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		fmt.Printf("Enabled UX exploration for %s (type=%s)\n", id, meta.UX.Type)
	}

	// Non-interactive add
	if *addCase != "" {
		if meta.UXState == nil {
			meta.UXState = &pt.UXState{Status: "pending"}
		}
		// Parse optional category:capability format
		category := ""
		capability := *addCase
		if idx := strings.Index(*addCase, ":"); idx > 0 {
			category = strings.TrimSpace((*addCase)[:idx])
			capability = strings.TrimSpace((*addCase)[idx+1:])
		}
		ucID := fmt.Sprintf("UC%d", len(meta.UXState.UseCases)+1)
		meta.UXState.UseCases = append(meta.UXState.UseCases, pt.UseCase{
			ID:    ucID,
			Actor: category,
			Goal:  capability,
		})
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		if category != "" {
			fmt.Printf("Added [%s]: %s: %s\n", ucID, category, capability)
		} else {
			fmt.Printf("Added [%s]: %s\n", ucID, capability)
		}
		return nil
	}

	// Non-interactive confirm
	if *confirm {
		if meta.UXState == nil || len(meta.UXState.UseCases) == 0 {
			return errors.New("no use cases to confirm; add at least one first")
		}
		meta.UXState.Status = "cases"
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		fmt.Println("Use cases confirmed. Next: pt ux-explore", id)
		return nil
	}

	fmt.Printf("UX Discovery: %s\n", iss.Title)
	fmt.Printf("UX Type: %s\n", meta.UX.Type)
	fmt.Println(strings.Repeat("─", 50))

	// Initialize UX state if needed
	if meta.UXState == nil {
		meta.UXState = &pt.UXState{Status: "pending"}
	}

	// Show existing use cases or prompt for new ones
	if len(meta.UXState.UseCases) > 0 {
		fmt.Println("\nDiscovered Capabilities:")
		for _, uc := range meta.UXState.UseCases {
			if uc.Actor != "" {
				fmt.Printf("  [%s] %s: %s\n", uc.ID, uc.Actor, uc.Goal)
			} else {
				fmt.Printf("  [%s] %s\n", uc.ID, uc.Goal)
			}
		}
		fmt.Println("\n  Discovery prompts (what's missing?):")
		fmt.Println("  • What other actions are needed?")
		fmt.Println("  • What information must be displayed?")
		fmt.Println("  • What errors must be handled?")
		fmt.Println("  • What's repetitive that could be templated?")
		fmt.Println("\nOptions: [A]dd, [D]iscover (guided), [C]ontinue, [Q]uit")
	} else {
		fmt.Println("\nUse cases surface the features & functionality needed.")
		fmt.Println("Goal: Discover ALL capabilities, not just obvious ones.")
		fmt.Println("\nFormat: <category>: <capability>")
		fmt.Println("Examples:")
		fmt.Println("  action: Place 1-4 leg order")
		fmt.Println("  info: Preview Greeks before submit")
		fmt.Println("  error: Handle partial fills")
		fmt.Println("  pattern: Save strategy as template")
		fmt.Println("\nOptions: [A]dd, [D]iscover (guided prompts), [Q]uit")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nChoice: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(strings.ToLower(choice))

	switch choice {
	case "a", "add":
		return addUseCase(client, ctx, id, meta)
	case "d", "discover":
		return discoverUseCases(client, ctx, id, iss, meta)
	case "c", "continue":
		if len(meta.UXState.UseCases) == 0 {
			return errors.New("cannot continue without use cases; add at least one")
		}
		meta.UXState.Status = "cases"
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		fmt.Println("\nUse cases confirmed. Next: pt ux-explore", id)
		return nil
	case "q", "quit":
		return nil
	default:
		return fmt.Errorf("unknown choice: %s", choice)
	}
}

func ensureUXEnabled(id string, meta pt.TaskMeta, requestedType string) (pt.TaskMeta, bool, error) {
	if meta.UX != nil {
		return meta, false, nil
	}

	// Explicit enable: user sets the type.
	if requestedType != "" {
		meta.UX = &pt.UXConfig{Type: requestedType}
		return meta, true, nil
	}

	// Auto-enable for discovery/doc tasks (common for spec work created via `pt add`).
	if meta.Template == "discovery" && strings.HasPrefix(meta.Artifact, "doc:") {
		meta.UX = &pt.UXConfig{Type: "doc"}
		return meta, true, nil
	}

	return meta, false, fmt.Errorf(
		"task %s does not have UX exploration enabled.\n\n"+
			"To enable UX for this task:\n"+
			"  pt ux-cases --type web %s\n"+
			"  # or: --type cli|tui|api|doc|error\n\n"+
			"If this task comes from a manifest, add a [tasks.ux] block (example):\n"+
			"  [tasks.ux]\n"+
			"  type = \"web\"\n",
		id,
		id,
	)
}

func addUseCase(client pt.Client, ctx context.Context, id string, meta pt.TaskMeta) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\nFormat: <category>: <capability>")
	fmt.Println("Categories: action, info, error, pattern (or leave blank)")
	fmt.Print("\nCapability: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return errors.New("capability is required")
	}

	// Parse category:capability format
	category := ""
	capability := input
	if idx := strings.Index(input, ":"); idx > 0 {
		category = strings.TrimSpace(input[:idx])
		capability = strings.TrimSpace(input[idx+1:])
	}

	ucID := fmt.Sprintf("UC%d", len(meta.UXState.UseCases)+1)
	uc := pt.UseCase{
		ID:    ucID,
		Actor: category, // Repurpose Actor field for category
		Goal:  capability,
	}
	meta.UXState.UseCases = append(meta.UXState.UseCases, uc)
	meta.UXState.Status = "cases"

	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	if category != "" {
		fmt.Printf("\nAdded: [%s] %s: %s\n", ucID, category, capability)
	} else {
		fmt.Printf("\nAdded: [%s] %s\n", ucID, capability)
	}
	fmt.Println("Run 'pt ux-cases", id, "' again to add more or continue.")
	return nil
}

func discoverUseCases(client pt.Client, ctx context.Context, id string, iss pt.Issue, meta pt.TaskMeta) error {
	reader := bufio.NewReader(os.Stdin)

	categories := []struct {
		name     string
		prompt   string
		examples string
	}{
		{"action", "What actions must the user perform?", "Place order, Cancel order, Modify position"},
		{"info", "What information must be displayed?", "Current price, Greeks, P&L, Risk metrics"},
		{"error", "What errors must be handled?", "Invalid input, API failure, Partial fill"},
		{"pattern", "What's repetitive that could be templated?", "Save strategy, Reuse settings, Quick-order"},
	}

	fmt.Println("\n┌─────────────────────────────────────────────────────────┐")
	fmt.Println("│ Guided Discovery: Surface all features needed           │")
	fmt.Println("└─────────────────────────────────────────────────────────┘")
	fmt.Printf("\nTask: %s\n", iss.Title)

	for _, cat := range categories {
		fmt.Printf("\n%s\n", strings.Repeat("─", 50))
		fmt.Printf("Category: %s\n", strings.ToUpper(cat.name))
		fmt.Printf("Prompt: %s\n", cat.prompt)
		fmt.Printf("Examples: %s\n", cat.examples)
		fmt.Println()

		for {
			fmt.Printf("%s (or Enter to skip): ", cat.name)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				break
			}

			ucID := fmt.Sprintf("UC%d", len(meta.UXState.UseCases)+1)
			uc := pt.UseCase{
				ID:    ucID,
				Actor: cat.name,
				Goal:  input,
			}
			meta.UXState.UseCases = append(meta.UXState.UseCases, uc)
			fmt.Printf("  Added: [%s] %s: %s\n", ucID, cat.name, input)
		}
	}

	meta.UXState.Status = "cases"
	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	fmt.Printf("\n%d capabilities discovered.\n", len(meta.UXState.UseCases))
	fmt.Println("Run 'pt ux-cases", id, "' to review or continue.")
	return nil
}

func cmdUXExplore(args []string) error {
	fs := flag.NewFlagSet("ux-explore", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	addOption := fs.String("add-option", "", "non-interactive: add a custom option")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-explore <id>")
		fmt.Println("\nGenerate and display UX options mapped to confirmed use cases.")
		fmt.Println("\nFlags:")
		fmt.Println("  --add-option 'description'  Add a custom option")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UX == nil {
		return fmt.Errorf(
			"task %s does not have UX exploration enabled.\n\n"+
				"Enable it first:\n"+
				"  pt ux-cases --type web %s\n",
			id,
			id,
		)
	}

	if meta.UXState == nil || len(meta.UXState.UseCases) == 0 {
		return fmt.Errorf("no use cases defined; run 'pt ux-cases %s' first", id)
	}

	// Non-interactive add option
	if *addOption != "" {
		meta.UXState.Options = append(meta.UXState.Options, *addOption)
		label := string(rune('A' + len(meta.UXState.Options) - 1))
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		fmt.Printf("Added option [%s]: %s\n", label, *addOption)
		return nil
	}

	fmt.Printf("UX Exploration: %s\n", iss.Title)
	fmt.Printf("UX Type: %s | Iteration: %d/%d\n", meta.UX.Type, meta.UXState.Iterations+1, max(meta.UX.IterationMax, 3))
	fmt.Println(strings.Repeat("─", 50))

	fmt.Println("\nCapabilities:")
	for _, uc := range meta.UXState.UseCases {
		if uc.Actor != "" {
			fmt.Printf("  [%s] %s: %s\n", uc.ID, uc.Actor, uc.Goal)
		} else {
			fmt.Printf("  [%s] %s\n", uc.ID, uc.Goal)
		}
	}

	// Show mockups or legacy options
	if len(meta.UXState.Mockups) > 0 {
		fmt.Println("\nMockups:")
		for _, m := range meta.UXState.Mockups {
			fmt.Printf("  [%s] %s (%s)\n", m.Label, m.Description, m.Fidelity)
		}
	} else if len(meta.UXState.Options) > 0 {
		fmt.Println("\nOptions:")
		for i, opt := range meta.UXState.Options {
			label := string(rune('A' + i))
			fmt.Printf("  [%s] %s\n", label, opt)
		}
	} else {
		fmt.Println("\nNo mockups yet. Create with:")
		fmt.Printf("  pt ux-mockup %s A < mockup.txt\n", id)
	}

	// Display coverage matrix
	displayCoverageMatrix(meta.UXState)

	fmt.Println("\nNext steps:")
	if len(meta.UXState.Mockups) == 0 {
		fmt.Println("  pt ux-mockup", id, "A         # Create mockup A")
	}
	fmt.Println("  pt ux-cover", id, "A UC1 full  # Mark coverage")
	fmt.Println("  pt ux-select", id, "\"A\"        # Select option")

	meta.UXState.Status = "explore"
	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	return nil
}

func generateOptionPlaceholders(uxType string, useCases []pt.UseCase) []string {
	switch uxType {
	case "cli":
		return []string{
			"Interactive wizard with step-by-step prompts",
			"CLI flags only (fully scriptable, no prompts)",
			"Hybrid: flags set defaults, prompts fill gaps",
		}
	case "tui":
		return []string{
			"Full-screen TUI with keyboard navigation",
			"Simple menu-based selection",
			"Split-pane view with details",
		}
	case "web":
		return []string{
			"Single-page form with inline validation",
			"Multi-step wizard with progress indicator",
			"Dashboard view with drill-down details",
		}
	case "api":
		return []string{
			"REST endpoints with standard CRUD",
			"GraphQL with flexible queries",
			"Simple RPC-style endpoints",
		}
	default:
		return []string{
			"Option A: {TODO: describe approach}",
			"Option B: {TODO: describe alternative}",
			"Option C: {TODO: describe hybrid}",
		}
	}
}

func cmdUXSelect(args []string) error {
	fs := flag.NewFlagSet("ux-select", flag.ContinueOnError)
	iterate := fs.Bool("iterate", false, "request another iteration instead of selecting")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-select <id> <choice> [--iterate]")
		fmt.Println("\nRecord UX selection and unlock building.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-select pt-5 \"A\"       # Select option A")
		fmt.Println("  pt ux-select pt-5 \"A+C\"     # Combine options A and C")
		fmt.Println("  pt ux-select pt-5 --iterate # Request refinement")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UX == nil {
		return fmt.Errorf(
			"task %s does not have UX exploration enabled.\n\n"+
				"Enable it first:\n"+
				"  pt ux-cases --type web %s\n",
			id,
			id,
		)
	}

	if meta.UXState == nil {
		return fmt.Errorf("UX exploration not started; run 'pt ux-cases %s' first", id)
	}

	maxIterations := meta.UX.IterationMax
	if maxIterations == 0 {
		maxIterations = 3
	}

	if *iterate {
		meta.UXState.Iterations++
		if meta.UXState.Iterations >= maxIterations {
			fmt.Printf("WARNING: Reached max iterations (%d). Consider escalating.\n", maxIterations)
			meta.UXState.Status = "escalate"
		} else {
			meta.UXState.Status = "cases" // Go back to explore
			meta.UXState.Options = nil    // Clear options for fresh generation
		}
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		fmt.Printf("Iteration %d requested. Run 'pt ux-explore %s' to generate new options.\n", meta.UXState.Iterations, id)
		return nil
	}

	if fs.NArg() < 2 {
		return errors.New("missing choice argument (e.g., 'A' or 'A+C')")
	}
	choice := fs.Arg(1)

	// Validate choice format
	choice = strings.ToUpper(strings.TrimSpace(choice))
	if choice == "" {
		return errors.New("choice cannot be empty")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Notes on this selection (optional): ")
	note, _ := reader.ReadString('\n')
	note = strings.TrimSpace(note)

	meta.UXState.Selection = choice
	meta.UXState.Note = note
	meta.UXState.Status = "selected"
	meta.UXState.ApprovedAt = time.Now().Format(time.RFC3339)

	// Add use cases to DoD criteria
	for _, uc := range meta.UXState.UseCases {
		criterion := fmt.Sprintf("Satisfies %s: %s can %s", uc.ID, uc.Actor, uc.Goal)
		if !containsCriterion(meta.DoD.Criteria, criterion) {
			meta.DoD.Criteria = append(meta.DoD.Criteria, criterion)
		}
	}

	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	fmt.Printf("\nUX Selection recorded: %s\n", choice)
	if note != "" {
		fmt.Printf("Note: %s\n", note)
	}
	fmt.Println("\nUse cases added to DoD criteria.")
	fmt.Println("Task is now ready for building. Run: pt claim", id)
	return nil
}

func containsCriterion(criteria []string, criterion string) bool {
	for _, c := range criteria {
		if c == criterion {
			return true
		}
	}
	return false
}

func cmdUXStatus(args []string) error {
	fs := flag.NewFlagSet("ux-status", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-status <id> [--json]")
		fmt.Println("\nShow current UX exploration state for a task.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UX == nil {
		return fmt.Errorf(
			"task %s does not have UX exploration enabled.\n\n"+
				"Enable it first:\n"+
				"  pt ux-cases --type web %s\n",
			id,
			id,
		)
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"id":       id,
			"title":    iss.Title,
			"ux":       meta.UX,
			"ux_state": meta.UXState,
		})
	}

	fmt.Printf("UX Status: %s\n", iss.Title)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("Type: %s\n", meta.UX.Type)
	fmt.Printf("Options Min: %d\n", max(meta.UX.OptionsMin, 2))
	fmt.Printf("Iteration Max: %d\n", max(meta.UX.IterationMax, 3))

	if meta.UXState == nil {
		fmt.Println("\nStatus: Not started")
		fmt.Println("Next: pt ux-cases", id)
		return nil
	}

	fmt.Printf("\nStatus: %s\n", meta.UXState.Status)
	fmt.Printf("Iterations: %d\n", meta.UXState.Iterations)

	if len(meta.UXState.UseCases) > 0 {
		fmt.Println("\nCapabilities:")
		for _, uc := range meta.UXState.UseCases {
			if uc.Actor != "" {
				fmt.Printf("  [%s] %s: %s\n", uc.ID, uc.Actor, uc.Goal)
			} else {
				fmt.Printf("  [%s] %s\n", uc.ID, uc.Goal)
			}
		}
	}

	// Show mockups (new file-based) or legacy options
	if len(meta.UXState.Mockups) > 0 {
		fmt.Println("\nMockups:")
		for _, m := range meta.UXState.Mockups {
			fidelityIcon := "📝"
			if m.Fidelity == "html" {
				fidelityIcon = "🌐"
			} else if m.Fidelity == "styled" {
				fidelityIcon = "🎨"
			}
			fmt.Printf("  [%s] %s %s (%s)\n", m.Label, fidelityIcon, m.Description, m.Fidelity)
		}
	} else if len(meta.UXState.Options) > 0 {
		fmt.Println("\nOptions (legacy):")
		for i, opt := range meta.UXState.Options {
			label := string(rune('A' + i))
			fmt.Printf("  [%s] %s\n", label, opt)
		}
	}

	if meta.UXState.Selection != "" {
		fmt.Printf("\nSelection: %s\n", meta.UXState.Selection)
		if meta.UXState.Note != "" {
			fmt.Printf("Note: %s\n", meta.UXState.Note)
		}
		if meta.UXState.ApprovedAt != "" {
			fmt.Printf("Approved: %s\n", meta.UXState.ApprovedAt)
		}
	}

	// Show next action
	fmt.Println()
	switch meta.UXState.Status {
	case "pending", "cases":
		if len(meta.UXState.UseCases) == 0 {
			fmt.Println("Next: pt ux-cases", id)
		} else {
			fmt.Println("Next: pt ux-explore", id)
		}
	case "explore":
		fmt.Println("Next: pt ux-select", id, "\"A\"")
	case "selected", "approved":
		fmt.Println("Ready for building: pt claim", id)
	case "escalate":
		fmt.Println("Escalation needed: max iterations reached")
	}

	return nil
}

// updateTaskMeta updates the task description with new metadata.
func updateTaskMeta(client pt.Client, ctx context.Context, id string, meta pt.TaskMeta) error {
	return client.UpdateMeta(ctx, id, meta)
}

func printJSONString(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// uxMockupDir returns the directory for mockup files: .pt/ux/{task-id}/
func uxMockupDir(taskID string) string {
	return filepath.Join(".pt", "ux", taskID)
}

// ensureMockupDir creates the mockup directory if it doesn't exist.
func ensureMockupDir(taskID string) error {
	dir := uxMockupDir(taskID)
	return os.MkdirAll(dir, 0755)
}

// mockupPath returns the path for a mockup file.
func mockupPath(taskID, label, fidelity string) string {
	label = strings.ToLower(label)
	switch fidelity {
	case "html":
		return filepath.Join(uxMockupDir(taskID), fmt.Sprintf("option-%s.html", label))
	case "styled":
		return filepath.Join(uxMockupDir(taskID), fmt.Sprintf("option-%s", label), "index.html")
	default: // ascii
		return filepath.Join(uxMockupDir(taskID), fmt.Sprintf("option-%s.txt", label))
	}
}

func cmdUXMockup(args []string) error {
	fs := flag.NewFlagSet("ux-mockup", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fileFlag := fs.String("file", "", "read mockup content from file")
	descFlag := fs.String("desc", "", "description of the mockup")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-mockup <id> [label] [--file FILE] [--desc DESC]")
		fmt.Println("\nCreate, update, or view ASCII mockups for UX options.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-mockup pt-42                # List all mockups")
		fmt.Println("  pt ux-mockup pt-42 A              # View mockup A")
		fmt.Println("  pt ux-mockup pt-42 A --file x.txt # Create from file")
		fmt.Println("  pt ux-mockup pt-42 A < mockup.txt # Create from stdin")
		fmt.Println("\nFlags:")
		fmt.Println("  --file FILE   Read mockup content from file")
		fmt.Println("  --desc DESC   Description for the mockup")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	label := ""
	if fs.NArg() >= 2 {
		label = strings.ToUpper(fs.Arg(1))
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UXState == nil {
		meta.UXState = &pt.UXState{Status: "pending"}
	}

	// No label = list all mockups
	if label == "" {
		return listMockups(iss, meta, id)
	}

	// Check if we're creating/updating or viewing
	hasInput := *fileFlag != "" || !isTerminal()

	if hasInput {
		// Create or update mockup
		return createMockup(client, ctx, id, label, *fileFlag, *descFlag, meta)
	}

	// View existing mockup
	return viewMockup(id, label, meta)
}

func listMockups(iss pt.Issue, meta pt.TaskMeta, id string) error {
	fmt.Printf("Mockups: %s\n", iss.Title)
	fmt.Println(strings.Repeat("─", 50))

	if len(meta.UXState.Mockups) == 0 {
		fmt.Println("\nNo mockups yet. Create with:")
		fmt.Printf("  pt ux-mockup %s A < mockup.txt\n", id)
		fmt.Printf("  pt ux-mockup %s A --file design.txt\n", id)
		return nil
	}

	fmt.Println()
	for _, m := range meta.UXState.Mockups {
		fidelityIcon := "📝"
		if m.Fidelity == "html" {
			fidelityIcon = "🌐"
		} else if m.Fidelity == "styled" {
			fidelityIcon = "🎨"
		}
		fmt.Printf("  [%s] %s %s (%s)\n", m.Label, fidelityIcon, m.Description, m.Fidelity)
	}

	fmt.Println("\nView: pt ux-mockup", id, "<label>")
	fmt.Println("Compare: pt ux-compare", id)
	return nil
}

func createMockup(client pt.Client, ctx context.Context, id, label, filePath, desc string, meta pt.TaskMeta) error {
	// Ensure directory exists
	if err := ensureMockupDir(id); err != nil {
		return fmt.Errorf("create mockup dir: %w", err)
	}

	// Read content
	var content []byte
	var err error
	if filePath != "" {
		content, err = os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	} else {
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	}

	if len(content) == 0 {
		return errors.New("empty mockup content")
	}

	// Write to file
	path := mockupPath(id, label, "ascii")
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write mockup: %w", err)
	}

	// Update metadata
	now := time.Now().Format(time.RFC3339)
	found := false
	for i, m := range meta.UXState.Mockups {
		if m.Label == label {
			meta.UXState.Mockups[i].UpdatedAt = now
			if desc != "" {
				meta.UXState.Mockups[i].Description = desc
			}
			found = true
			break
		}
	}
	if !found {
		if desc == "" {
			desc = fmt.Sprintf("Option %s", label)
		}
		meta.UXState.Mockups = append(meta.UXState.Mockups, pt.UXMockup{
			Label:       label,
			Description: desc,
			Fidelity:    "ascii",
			Path:        filepath.Base(path),
			CreatedAt:   now,
		})
	}

	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	fmt.Printf("Created mockup [%s]: %s\n", label, path)
	fmt.Printf("View: pt ux-mockup %s %s\n", id, label)
	return nil
}

func viewMockup(id, label string, meta pt.TaskMeta) error {
	// Find mockup in metadata
	var mockup *pt.UXMockup
	for _, m := range meta.UXState.Mockups {
		if m.Label == label {
			mockup = &m
			break
		}
	}

	if mockup == nil {
		return fmt.Errorf("no mockup [%s] found; create with: pt ux-mockup %s %s < file.txt", label, id, label)
	}

	path := mockupPath(id, label, mockup.Fidelity)
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read mockup: %w", err)
	}

	fmt.Printf("Mockup [%s]: %s (%s)\n", label, mockup.Description, mockup.Fidelity)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println(string(content))
	return nil
}

func cmdUXCompare(args []string) error {
	fs := flag.NewFlagSet("ux-compare", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-compare <id> [labels...]")
		fmt.Println("\nCompare mockups side-by-side.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-compare pt-42       # Compare all mockups")
		fmt.Println("  pt ux-compare pt-42 A B   # Compare A and B only")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	labels := fs.Args()[1:]

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UXState == nil || len(meta.UXState.Mockups) == 0 {
		return fmt.Errorf("no mockups to compare; create with: pt ux-mockup %s A", id)
	}

	// Filter mockups if labels specified
	mockups := meta.UXState.Mockups
	if len(labels) > 0 {
		filtered := []pt.UXMockup{}
		labelSet := make(map[string]bool)
		for _, l := range labels {
			labelSet[strings.ToUpper(l)] = true
		}
		for _, m := range mockups {
			if labelSet[m.Label] {
				filtered = append(filtered, m)
			}
		}
		mockups = filtered
	}

	if len(mockups) == 0 {
		return errors.New("no matching mockups found")
	}

	fmt.Printf("Comparing: %s\n", iss.Title)
	fmt.Println(strings.Repeat("═", 80))

	// Load all mockup contents
	type mockupContent struct {
		label  string
		desc   string
		lines  []string
		maxLen int
	}
	contents := make([]mockupContent, len(mockups))

	for i, m := range mockups {
		path := mockupPath(id, m.Label, m.Fidelity)
		data, err := os.ReadFile(path)
		if err != nil {
			contents[i] = mockupContent{
				label: m.Label,
				desc:  m.Description,
				lines: []string{"(file not found)"},
			}
			continue
		}
		lines := strings.Split(string(data), "\n")
		maxLen := 0
		for _, l := range lines {
			if len(l) > maxLen {
				maxLen = len(l)
			}
		}
		contents[i] = mockupContent{
			label:  m.Label,
			desc:   m.Description,
			lines:  lines,
			maxLen: maxLen,
		}
	}

	// Print headers
	colWidth := 26
	for _, c := range contents {
		header := fmt.Sprintf("Option %s", c.label)
		fmt.Printf("%-*s", colWidth, header)
	}
	fmt.Println()
	for range contents {
		fmt.Printf("%s", strings.Repeat("─", colWidth-2)+"  ")
	}
	fmt.Println()

	// Find max lines
	maxLines := 0
	for _, c := range contents {
		if len(c.lines) > maxLines {
			maxLines = len(c.lines)
		}
	}

	// Print side by side
	for lineNum := 0; lineNum < maxLines; lineNum++ {
		for _, c := range contents {
			line := ""
			if lineNum < len(c.lines) {
				line = c.lines[lineNum]
			}
			// Truncate long lines
			if len(line) > colWidth-2 {
				line = line[:colWidth-5] + "..."
			}
			fmt.Printf("%-*s", colWidth, line)
		}
		fmt.Println()
	}

	fmt.Println()
	fmt.Println("Select: pt ux-select", id, "\"<label>\"")
	return nil
}

func cmdUXUpgrade(args []string) error {
	fs := flag.NewFlagSet("ux-upgrade", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	toHTML := fs.Bool("html", false, "upgrade to basic HTML")
	toStyled := fs.Bool("styled", false, "upgrade to styled HTML")
	fileFlag := fs.String("file", "", "read upgraded content from file")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-upgrade <id> <label> --html|--styled [--file FILE]")
		fmt.Println("\nUpgrade mockup fidelity level.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-upgrade pt-42 A --html            # Upgrade A to HTML")
		fmt.Println("  pt ux-upgrade pt-42 A --html --file x   # With custom content")
		fmt.Println("  pt ux-upgrade pt-42 A --styled          # Upgrade to styled")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return errors.New("missing id and label arguments")
	}
	if !*toHTML && !*toStyled {
		return errors.New("specify --html or --styled")
	}

	id := fs.Arg(0)
	label := strings.ToUpper(fs.Arg(1))

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UXState == nil {
		return errors.New("no UX state; run ux-mockup first")
	}

	// Find mockup
	var mockupIdx int = -1
	for i, m := range meta.UXState.Mockups {
		if m.Label == label {
			mockupIdx = i
			break
		}
	}
	if mockupIdx == -1 {
		return fmt.Errorf("mockup [%s] not found", label)
	}

	targetFidelity := "html"
	if *toStyled {
		targetFidelity = "styled"
	}

	// Read content
	var content []byte
	if *fileFlag != "" {
		content, err = os.ReadFile(*fileFlag)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	} else if !isTerminal() {
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else {
		// Generate template
		content = generateHTMLTemplate(meta.UXState.Mockups[mockupIdx], id, label, targetFidelity)
	}

	// Write to appropriate path
	path := mockupPath(id, label, targetFidelity)
	if targetFidelity == "styled" {
		// Create directory for styled
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create styled dir: %w", err)
		}
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	// Update metadata
	meta.UXState.Mockups[mockupIdx].Fidelity = targetFidelity
	meta.UXState.Mockups[mockupIdx].UpdatedAt = time.Now().Format(time.RFC3339)

	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	fmt.Printf("Upgraded [%s] to %s: %s\n", label, targetFidelity, path)
	if *fileFlag == "" && isTerminal() {
		fmt.Println("\nGenerated template. Edit and save, or provide content via --file or stdin.")
	}
	return nil
}

func generateHTMLTemplate(m pt.UXMockup, id, label, fidelity string) []byte {
	if fidelity == "styled" {
		return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s - Option %s</title>
    <link rel="stylesheet" href="styles.css">
</head>
<body>
    <h1>%s</h1>
    <div class="mockup">
        <!-- TODO: Add styled content -->
        <p>Upgrade from ASCII mockup</p>
    </div>
</body>
</html>
`, m.Description, label, m.Description))
	}

	// Basic HTML
	return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s - Option %s</title>
    <style>
        body { font-family: system-ui, sans-serif; padding: 2rem; }
        .mockup { border: 1px solid #ccc; padding: 1rem; max-width: 600px; }
    </style>
</head>
<body>
    <h1>Option %s: %s</h1>
    <div class="mockup">
        <!-- TODO: Convert ASCII to interactive HTML -->
        <pre>%s</pre>
    </div>
</body>
</html>
`, m.Description, label, label, m.Description, "(paste ASCII content here)"))
}

func cmdUXDrill(args []string) error {
	fs := flag.NewFlagSet("ux-drill", flag.ContinueOnError)
	_ = fs.String("db", "", "override store path")
	_ = fs.String("prefix", "", "override issue prefix")
	focus := fs.String("focus", "", "aspect to focus on")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-drill <id> <label> --focus=\"aspect\"")
		fmt.Println("\nCreate a focused drill-down mockup for a specific aspect.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-drill pt-42 A --focus=\"error state\"")
		fmt.Println("  pt ux-drill pt-42 B --focus=\"loading sequence\"")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return errors.New("missing id and label arguments")
	}
	if *focus == "" {
		return errors.New("--focus is required")
	}

	id := fs.Arg(0)
	label := strings.ToUpper(fs.Arg(1))

	// Create drill-down filename
	focusSlug := strings.ToLower(strings.ReplaceAll(*focus, " ", "-"))
	drillPath := filepath.Join(uxMockupDir(id), fmt.Sprintf("option-%s.focus-%s.txt", strings.ToLower(label), focusSlug))

	// Check if content provided
	if !isTerminal() {
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		if err := ensureMockupDir(id); err != nil {
			return err
		}
		if err := os.WriteFile(drillPath, content, 0644); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		fmt.Printf("Created drill-down: %s\n", drillPath)
		return nil
	}

	// View existing or prompt for creation
	if content, err := os.ReadFile(drillPath); err == nil {
		fmt.Printf("Drill-down [%s] - %s\n", label, *focus)
		fmt.Println(strings.Repeat("─", 50))
		fmt.Println(string(content))
		return nil
	}

	fmt.Printf("Create drill-down for [%s] focusing on: %s\n", label, *focus)
	fmt.Printf("  echo 'content' | pt ux-drill %s %s --focus=\"%s\"\n", id, label, *focus)
	return nil
}

func cmdUXBreakout(args []string) error {
	fs := flag.NewFlagSet("ux-breakout", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-breakout <id> <label>")
		fmt.Println("\nCreate component breakout directory for implementation.")
		fmt.Println("\nCreates a directory structure with:")
		fmt.Println("  option-{label}/")
		fmt.Println("  ├── README.md          # Implementation notes")
		fmt.Println("  ├── components/        # Component specs")
		fmt.Println("  └── strings.json       # Copy/i18n strings")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return errors.New("missing id and label arguments")
	}

	id := fs.Arg(0)
	label := strings.ToLower(fs.Arg(1))

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	// Find mockup
	var mockup *pt.UXMockup
	for _, m := range meta.UXState.Mockups {
		if strings.ToLower(m.Label) == label {
			mockup = &m
			break
		}
	}
	if mockup == nil {
		return fmt.Errorf("mockup [%s] not found", strings.ToUpper(label))
	}

	// Create breakout directory
	breakoutDir := filepath.Join(uxMockupDir(id), fmt.Sprintf("option-%s", label))
	componentsDir := filepath.Join(breakoutDir, "components")

	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		return fmt.Errorf("create breakout dir: %w", err)
	}

	// Create README
	readme := fmt.Sprintf(`# Implementation: Option %s

## Task
%s

## Description
%s

## Components
- [ ] Component 1: TODO
- [ ] Component 2: TODO

## Copy/Strings
See strings.json for all user-facing text.

## Notes
- Created: %s
`, strings.ToUpper(label), iss.Title, mockup.Description, time.Now().Format("2006-01-02"))

	if err := os.WriteFile(filepath.Join(breakoutDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	// Create strings.json template
	stringsJSON := `{
  "title": "TODO",
  "messages": {
    "success": "TODO",
    "error": "TODO"
  },
  "buttons": {
    "primary": "TODO",
    "secondary": "TODO"
  }
}
`
	if err := os.WriteFile(filepath.Join(breakoutDir, "strings.json"), []byte(stringsJSON), 0644); err != nil {
		return fmt.Errorf("write strings.json: %w", err)
	}

	fmt.Printf("Created breakout for [%s]:\n", strings.ToUpper(label))
	fmt.Printf("  %s/\n", breakoutDir)
	fmt.Println("  ├── README.md")
	fmt.Println("  ├── components/")
	fmt.Println("  └── strings.json")
	fmt.Println("\nEdit these files with implementation details.")
	return nil
}

// isTerminal returns true if stdin is a terminal (not piped)
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// displayCoverageMatrix shows capability coverage for each mockup
func displayCoverageMatrix(state *pt.UXState) {
	if state == nil || len(state.UseCases) == 0 {
		return
	}

	// Get mockup labels
	var labels []string
	if len(state.Mockups) > 0 {
		for _, m := range state.Mockups {
			labels = append(labels, m.Label)
		}
	} else if len(state.Options) > 0 {
		for i := range state.Options {
			labels = append(labels, string(rune('A'+i)))
		}
	}

	if len(labels) == 0 {
		return
	}

	fmt.Println("\nCoverage Matrix:")
	fmt.Println(strings.Repeat("═", 60))

	// Header
	fmt.Printf("%-40s", "")
	for _, label := range labels {
		fmt.Printf("  %s  ", label)
	}
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))

	// Rows for each capability
	fullCount := make(map[string]int)
	partialCount := make(map[string]int)
	for _, uc := range state.UseCases {
		// Truncate capability description
		desc := uc.Goal
		if uc.Actor != "" {
			desc = uc.Actor + ": " + uc.Goal
		}
		if len(desc) > 35 {
			desc = desc[:32] + "..."
		}
		fmt.Printf("%-40s", fmt.Sprintf("[%s] %s", uc.ID, desc))

		for _, label := range labels {
			status := getCoverageStatus(state.Coverage, label, uc.ID)
			symbol := "·" // unmarked
			switch status {
			case "full":
				symbol = "✓"
				fullCount[label]++
			case "partial":
				symbol = "~"
				partialCount[label]++
			case "none":
				symbol = "✗"
			}
			fmt.Printf("  %s  ", symbol)
		}
		fmt.Println()
	}

	// Summary row
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("%-40s", "Coverage:")
	for _, label := range labels {
		full := fullCount[label]
		partial := partialCount[label]
		if partial > 0 {
			fmt.Printf("%d+%d/%d ", full, partial, len(state.UseCases))
		} else {
			fmt.Printf("%d/%d ", full, len(state.UseCases))
		}
	}
	fmt.Println()

	fmt.Println("\nLegend: ✓ full  ~ partial  ✗ none  · unmarked")
}

func getCoverageStatus(coverage pt.UXCoverage, mockupLabel, ucID string) string {
	if coverage == nil {
		return ""
	}
	if mockup, ok := coverage[mockupLabel]; ok {
		if status, ok := mockup[ucID]; ok {
			return status
		}
	}
	return ""
}

func cmdUXCover(args []string) error {
	fs := flag.NewFlagSet("ux-cover", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-cover <id> <mockup> <capability> <status>")
		fmt.Println("\nMark capability coverage for a mockup.")
		fmt.Println("\nStatus values:")
		fmt.Println("  full    (✓) Fully addresses the capability")
		fmt.Println("  partial (~) Partially addresses")
		fmt.Println("  none    (✗) Does not address")
		fmt.Println("\nExamples:")
		fmt.Println("  pt ux-cover pt-42 A UC1 full")
		fmt.Println("  pt ux-cover pt-42 B UC3 partial")
		fmt.Println("  pt ux-cover pt-42 A all full    # Mark all as full")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 4 {
		fs.Usage()
		return errors.New("missing arguments: id mockup capability status")
	}

	id := fs.Arg(0)
	mockupLabel := strings.ToUpper(fs.Arg(1))
	capabilityID := strings.ToUpper(fs.Arg(2))
	status := strings.ToLower(fs.Arg(3))

	// Validate status
	if status != "full" && status != "partial" && status != "none" {
		return fmt.Errorf("invalid status %q; use: full, partial, none", status)
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if meta.UXState == nil {
		return errors.New("no UX state; run ux-cases first")
	}

	// Initialize coverage map if needed
	if meta.UXState.Coverage == nil {
		meta.UXState.Coverage = make(pt.UXCoverage)
	}
	if meta.UXState.Coverage[mockupLabel] == nil {
		meta.UXState.Coverage[mockupLabel] = make(map[string]string)
	}

	// Handle "all" to mark all capabilities
	if capabilityID == "ALL" {
		for _, uc := range meta.UXState.UseCases {
			meta.UXState.Coverage[mockupLabel][uc.ID] = status
		}
		fmt.Printf("Marked all capabilities for [%s] as %s\n", mockupLabel, status)
	} else {
		// Validate capability exists
		found := false
		for _, uc := range meta.UXState.UseCases {
			if uc.ID == capabilityID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("capability %s not found", capabilityID)
		}

		meta.UXState.Coverage[mockupLabel][capabilityID] = status
		symbol := "✓"
		if status == "partial" {
			symbol = "~"
		} else if status == "none" {
			symbol = "✗"
		}
		fmt.Printf("Marked [%s] %s: %s %s\n", mockupLabel, capabilityID, symbol, status)
	}

	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	return nil
}
