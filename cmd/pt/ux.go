package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	pt "projects-tasks/pkg/pt"
)

func cmdUXCases(args []string) error {
	fs := flag.NewFlagSet("ux-cases", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	addCase := fs.String("add", "", "non-interactive: add use case 'actor:goal'")
	confirm := fs.Bool("confirm", false, "non-interactive: confirm use cases and proceed")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux-cases <id>")
		fmt.Println("\nShow and confirm use cases for a UX-enabled task.")
		fmt.Println("Use cases ground the UX exploration in real user needs.")
		fmt.Println("\nFlags:")
		fmt.Println("  --add 'actor:goal'  Add a use case non-interactively")
		fmt.Println("  --confirm           Confirm use cases and proceed to explore")
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

	// Non-interactive add
	if *addCase != "" {
		parts := strings.SplitN(*addCase, ":", 2)
		if len(parts) != 2 {
			return errors.New("use case format: 'actor:goal'")
		}
		if meta.UXState == nil {
			meta.UXState = &pt.UXState{Status: "pending"}
		}
		ucID := fmt.Sprintf("UC%d", len(meta.UXState.UseCases)+1)
		meta.UXState.UseCases = append(meta.UXState.UseCases, pt.UseCase{
			ID:    ucID,
			Actor: strings.TrimSpace(parts[0]),
			Goal:  strings.TrimSpace(parts[1]),
		})
		if err := updateTaskMeta(client, ctx, id, meta); err != nil {
			return err
		}
		fmt.Printf("Added use case [%s]: As a %s, I want to %s\n", ucID, parts[0], parts[1])
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

	if meta.UX == nil {
		return fmt.Errorf("task %s does not have UX exploration enabled (no [tasks.ux] field)", id)
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
		fmt.Println("\nConfirmed Use Cases:")
		for _, uc := range meta.UXState.UseCases {
			fmt.Printf("  [%s] As a %s, I want to %s\n", uc.ID, uc.Actor, uc.Goal)
			if uc.Context != "" {
				fmt.Printf("       Context: %s\n", uc.Context)
			}
		}
		fmt.Println("\nOptions: [A]dd more, [E]dit, [C]ontinue to explore, [Q]uit")
	} else {
		fmt.Println("\nNo use cases defined yet.")
		fmt.Println("Use cases describe WHO wants WHAT and WHY.")
		fmt.Println("\nSuggested format: As a <actor>, I want to <goal>")
		fmt.Println("Example: As a trader, I want to roll my option position to a later expiry")
		fmt.Println("\nOptions: [A]dd use case, [S]uggest from context, [Q]uit")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nChoice: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(strings.ToLower(choice))

	switch choice {
	case "a", "add":
		return addUseCase(client, ctx, id, meta)
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
	case "s", "suggest":
		fmt.Println("\nBased on task context, suggested use cases:")
		fmt.Println("  [UC1] As a developer, I want to " + strings.ToLower(iss.Title))
		fmt.Println("\nPress Enter to add this, or type your own:")
		return nil
	case "q", "quit":
		return nil
	default:
		return fmt.Errorf("unknown choice: %s", choice)
	}
}

func addUseCase(client pt.Client, ctx context.Context, id string, meta pt.TaskMeta) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Actor (e.g., 'developer', 'trader', 'user'): ")
	actor, _ := reader.ReadString('\n')
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return errors.New("actor is required")
	}

	fmt.Print("Goal (what they want to do): ")
	goal, _ := reader.ReadString('\n')
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return errors.New("goal is required")
	}

	fmt.Print("Context (optional, when/where): ")
	context, _ := reader.ReadString('\n')
	context = strings.TrimSpace(context)

	ucID := fmt.Sprintf("UC%d", len(meta.UXState.UseCases)+1)
	uc := pt.UseCase{
		ID:      ucID,
		Actor:   actor,
		Goal:    goal,
		Context: context,
	}
	meta.UXState.UseCases = append(meta.UXState.UseCases, uc)
	meta.UXState.Status = "cases"

	// Update task metadata
	if err := updateTaskMeta(client, ctx, id, meta); err != nil {
		return err
	}

	fmt.Printf("\nAdded: [%s] As a %s, I want to %s\n", ucID, actor, goal)
	fmt.Println("Run 'pt ux-cases", id, "' again to add more or continue.")
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
		return fmt.Errorf("task %s does not have UX exploration enabled", id)
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

	fmt.Println("\nUse Cases:")
	for _, uc := range meta.UXState.UseCases {
		fmt.Printf("  [%s] %s wants to %s\n", uc.ID, uc.Actor, uc.Goal)
	}

	// Show existing options or generate placeholders
	fmt.Println("\nOptions (label for easy reference):")
	if len(meta.UXState.Options) > 0 {
		for i, opt := range meta.UXState.Options {
			label := string(rune('A' + i))
			fmt.Printf("  [%s] %s\n", label, opt)
		}
	} else {
		// Generate placeholder options based on UX type
		placeholders := generateOptionPlaceholders(meta.UX.Type, meta.UXState.UseCases)
		for i, opt := range placeholders {
			label := string(rune('A' + i))
			fmt.Printf("  [%s] %s\n", label, opt)
		}
		meta.UXState.Options = placeholders
	}

	fmt.Println("\nUse Case Coverage:")
	for _, uc := range meta.UXState.UseCases {
		fmt.Printf("  %s: [A]✓ [B]✓ [C]~\n", uc.ID)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  pt ux-select", id, "\"A\"      # Select option A")
	fmt.Println("  pt ux-select", id, "\"A+C\"    # Combine A and C")
	fmt.Println("  pt ux-select", id, "--iterate # Request refinement")

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
		return fmt.Errorf("task %s does not have UX exploration enabled", id)
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
		return fmt.Errorf("task %s does not have UX exploration enabled", id)
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
		fmt.Println("\nUse Cases:")
		for _, uc := range meta.UXState.UseCases {
			fmt.Printf("  [%s] %s wants to %s\n", uc.ID, uc.Actor, uc.Goal)
		}
	}

	if len(meta.UXState.Options) > 0 {
		fmt.Println("\nOptions:")
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
