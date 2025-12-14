package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

// synthesisDir returns the directory for synthesis files
func synthesisDir() string {
	return filepath.Join(".pt", "synthesis")
}

// synthesisFile returns the path to a synthesis file for a component
func synthesisFile(componentID string) string {
	return filepath.Join(synthesisDir(), componentID+".json")
}

// loadSynthesis loads a synthesis from disk
func loadSynthesis(componentID string) (*pt.UXSynthesis, error) {
	path := synthesisFile(componentID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var syn pt.UXSynthesis
	if err := json.Unmarshal(data, &syn); err != nil {
		return nil, err
	}
	return &syn, nil
}

// saveSynthesis persists a synthesis to disk
func saveSynthesis(syn *pt.UXSynthesis) error {
	if err := os.MkdirAll(synthesisDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(syn, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(synthesisFile(syn.ComponentID), data, 0644)
}

// cmdDiscovery handles the discovery command
func cmdDiscovery(args []string) error {
	if len(args) == 0 {
		return cmdDiscoveryHelp()
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "init":
		return cmdDiscoveryInit(subArgs)
	case "status":
		return cmdDiscoveryStatus(subArgs)
	case "capabilities":
		return cmdDiscoveryCapabilities(subArgs)
	case "explore":
		return cmdDiscoveryExplore(subArgs)
	case "option":
		return cmdDiscoveryOption(subArgs)
	case "coverage":
		return cmdDiscoveryCoverage(subArgs)
	case "synthesize":
		return cmdDiscoverySynthesize(subArgs)
	case "review":
		return cmdDiscoveryReview(subArgs)
	case "feedback":
		return cmdDiscoveryFeedback(subArgs)
	case "iterate":
		return cmdDiscoveryIterate(subArgs)
	case "approve":
		return cmdDiscoveryApprove(subArgs)
	case "handoff":
		return cmdDiscoveryHandoff(subArgs)
	case "guidance":
		return cmdDiscoveryGuidance(subArgs)
	case "help":
		return cmdDiscoveryHelp()
	default:
		return fmt.Errorf("unknown discovery subcommand: %s\nRun 'pt discovery help' for usage", subCmd)
	}
}

func cmdDiscoveryHelp() error {
	fmt.Println(`Discovery Workflow - Synthesis-Based UX Exploration

PHASES:
  1. INIT         → Create synthesis for component
  2. CAPABILITIES → Define what must be supported
  3. EXPLORE      → Generate 5+ options, 2+ approaches
  4. SYNTHESIZE   → Narrow to top 3 with rationale
  5. REVIEW       → User reviews comprehensive artifact
  6. FEEDBACK     → User provides targeted feedback
  7. APPROVE      → Final approval, generate handoff

AGENT COMMANDS (automated):
  pt discovery init <component> --type <cli|tui|web|api>
  pt discovery capabilities <component> --add "category: capability"
  pt discovery explore <component>
  pt discovery option <component> <label> --name "..." --desc "..."
  pt discovery coverage <component> <label> <cap-id> <full|partial|none>
  pt discovery synthesize <component>
  pt discovery iterate <component>
  pt discovery handoff <component>

USER COMMANDS (gates):
  pt discovery review <component>       # View synthesis artifact
  pt discovery feedback <component>     # Add feedback
  pt discovery approve <component>      # Final approval

EXAMPLES:
  # Agent initializes and explores
  pt discovery init order --type cli
  pt discovery capabilities order --add "action: Submit order"
  pt discovery capabilities order --add "error: Handle rejection"
  pt discovery explore order
  pt discovery option order A --name "Interactive" --desc "Step-by-step prompts"
  pt discovery synthesize order

  # User reviews
  pt discovery review order             # Shows full synthesis
  pt discovery feedback order --component A3 "Needs confirmation step"
  pt discovery approve order

  # Agent generates implementation guidance
  pt discovery handoff order`)
	return nil
}

func cmdDiscoveryInit(args []string) error {
	fs := flag.NewFlagSet("discovery init", flag.ExitOnError)
	uxType := fs.String("type", "", "UX type: cli|tui|web|api (required)")
	taskID := fs.String("task", "", "Link to PT task ID")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery init <component-id> --type <cli|tui|web|api>")
	}

	componentID := fs.Arg(0)

	if *uxType == "" {
		return fmt.Errorf("--type is required (cli|tui|web|api)")
	}

	validTypes := map[string]bool{"cli": true, "tui": true, "web": true, "api": true}
	if !validTypes[*uxType] {
		return fmt.Errorf("invalid type %q; use: cli, tui, web, api", *uxType)
	}

	// Check if already exists
	existing, _ := loadSynthesis(componentID)
	if existing != nil {
		fmt.Printf("Synthesis already exists for %s (status: %s)\n", componentID, existing.Status)
		fmt.Printf("Use 'pt discovery status %s' to see current state.\n", componentID)
		return nil
	}

	syn := &pt.UXSynthesis{
		ComponentID:  componentID,
		TaskID:       *taskID,
		UXType:       *uxType,
		Status:       pt.StatusCapabilities,
		Capabilities: []pt.UseCase{},
		Options:      []pt.SynthesisOption{},
		Rejected:     []pt.RejectedOption{},
		Exploration: pt.ExplorationLog{
			Approaches: []string{},
		},
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	fmt.Printf("✓ Discovery initialized for [%s] (type: %s)\n", componentID, *uxType)
	fmt.Println("\nNext steps:")
	fmt.Printf("  pt discovery capabilities %s --add \"action: ...\"\n", componentID)
	fmt.Printf("  pt discovery guidance %s exploration  # See patterns for %s\n", componentID, *uxType)
	return nil
}

func cmdDiscoveryStatus(args []string) error {
	fs := flag.NewFlagSet("discovery status", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	if fs.NArg() < 1 {
		// List all syntheses
		return listAllSyntheses(*jsonOut)
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s; run 'pt discovery init %s --type <type>'", componentID, componentID)
	}

	if *jsonOut {
		return printJSON(syn)
	}

	fmt.Printf("Discovery: %s\n", componentID)
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("Type:   %s\n", syn.UXType)
	fmt.Printf("Status: %s\n", syn.Status)
	if syn.TaskID != "" {
		fmt.Printf("Task:   %s\n", syn.TaskID)
	}
	fmt.Printf("Iterations: %d\n", syn.Iterations)

	// Capabilities
	if len(syn.Capabilities) > 0 {
		fmt.Printf("\nCapabilities (%d):\n", len(syn.Capabilities))
		for _, c := range syn.Capabilities {
			if c.Actor != "" {
				fmt.Printf("  [%s] %s: %s\n", c.ID, c.Actor, c.Goal)
			} else {
				fmt.Printf("  [%s] %s\n", c.ID, c.Goal)
			}
		}
	}

	// Options
	if len(syn.Options) > 0 {
		fmt.Printf("\nOptions (%d):\n", len(syn.Options))
		for _, o := range syn.Options {
			fmt.Printf("  [%s] %s: %s (%d/%d coverage)\n", o.Label, o.Name, o.Description, o.Coverage, o.Total)
		}
	}

	// Exploration stats
	fmt.Printf("\nExploration:\n")
	fmt.Printf("  Total options explored: %d\n", syn.Exploration.TotalOptions)
	if len(syn.Exploration.Approaches) > 0 {
		fmt.Printf("  Approaches: %s\n", strings.Join(syn.Exploration.Approaches, ", "))
	}

	// Feedback
	unaddressed := 0
	for _, f := range syn.Feedback {
		if !f.Addressed {
			unaddressed++
		}
	}
	if len(syn.Feedback) > 0 {
		fmt.Printf("\nFeedback: %d total, %d unaddressed\n", len(syn.Feedback), unaddressed)
	}

	// Next step
	fmt.Println()
	printNextStep(syn)
	return nil
}

func listAllSyntheses(jsonOut bool) error {
	dir := synthesisDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No discoveries found.")
			fmt.Println("Create one with: pt discovery init <component> --type <cli|tui|web|api>")
			return nil
		}
		return err
	}

	syntheses := []map[string]interface{}{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		componentID := strings.TrimSuffix(e.Name(), ".json")
		syn, err := loadSynthesis(componentID)
		if err != nil || syn == nil {
			continue
		}
		syntheses = append(syntheses, map[string]interface{}{
			"component": syn.ComponentID,
			"type":      syn.UXType,
			"status":    syn.Status,
			"caps":      len(syn.Capabilities),
			"options":   len(syn.Options),
		})
	}

	if jsonOut {
		return printJSON(syntheses)
	}

	if len(syntheses) == 0 {
		fmt.Println("No discoveries found.")
		fmt.Println("Create one with: pt discovery init <component> --type <cli|tui|web|api>")
		return nil
	}

	fmt.Println("Active Discoveries:")
	fmt.Println(strings.Repeat("─", 60))
	for _, s := range syntheses {
		fmt.Printf("  [%s] %s: %s (%d caps, %d options)\n",
			s["component"], s["type"], s["status"], s["caps"], s["options"])
	}
	return nil
}

func printNextStep(syn *pt.UXSynthesis) {
	switch syn.Status {
	case pt.StatusCapabilities:
		if len(syn.Capabilities) == 0 {
			fmt.Printf("Next: pt discovery capabilities %s --add \"category: capability\"\n", syn.ComponentID)
		} else {
			fmt.Printf("Next: pt discovery explore %s  (or add more capabilities)\n", syn.ComponentID)
		}
	case pt.StatusExploring:
		gate := pt.DefaultExplorationGate()
		if syn.Exploration.TotalOptions < gate.MinOptions {
			fmt.Printf("Next: pt discovery option %s <label> --name \"...\"  (need %d+ options)\n", syn.ComponentID, gate.MinOptions)
		} else if len(syn.Exploration.Approaches) < gate.MinApproaches {
			fmt.Printf("Next: Try a different approach (have %d, need %d)\n", len(syn.Exploration.Approaches), gate.MinApproaches)
		} else {
			fmt.Printf("Next: pt discovery synthesize %s\n", syn.ComponentID)
		}
	case pt.StatusSynthesized:
		fmt.Printf("Next: pt discovery review %s  (USER: review synthesis)\n", syn.ComponentID)
	case pt.StatusReviewing:
		fmt.Printf("Next: pt discovery approve %s  (USER: or give feedback)\n", syn.ComponentID)
	case pt.StatusFeedback:
		fmt.Printf("Next: pt discovery iterate %s  (AGENT: address feedback)\n", syn.ComponentID)
	case pt.StatusApproved:
		fmt.Printf("Next: pt discovery handoff %s  (generate implementation guidance)\n", syn.ComponentID)
	}
}

func cmdDiscoveryCapabilities(args []string) error {
	fs := flag.NewFlagSet("discovery capabilities", flag.ExitOnError)
	addCap := fs.String("add", "", "Add capability: 'category: description'")
	confirm := fs.Bool("confirm", false, "Confirm capabilities and proceed to explore")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery capabilities <component-id> [--add 'cap'] [--confirm]")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Add capability
	if *addCap != "" {
		category := ""
		capability := *addCap
		if idx := strings.Index(*addCap, ":"); idx > 0 {
			category = strings.TrimSpace((*addCap)[:idx])
			capability = strings.TrimSpace((*addCap)[idx+1:])
		}

		ucID := fmt.Sprintf("UC%d", len(syn.Capabilities)+1)
		syn.Capabilities = append(syn.Capabilities, pt.UseCase{
			ID:    ucID,
			Actor: category,
			Goal:  capability,
		})

		if err := saveSynthesis(syn); err != nil {
			return err
		}

		if category != "" {
			fmt.Printf("Added [%s]: %s: %s\n", ucID, category, capability)
		} else {
			fmt.Printf("Added [%s]: %s\n", ucID, capability)
		}
		return nil
	}

	// Confirm and transition
	if *confirm {
		if len(syn.Capabilities) == 0 {
			return fmt.Errorf("no capabilities to confirm; add at least one first")
		}
		syn.Status = pt.StatusExploring
		if err := saveSynthesis(syn); err != nil {
			return err
		}
		fmt.Printf("✓ %d capabilities confirmed. Ready to explore.\n", len(syn.Capabilities))
		fmt.Printf("\nNext: pt discovery guidance %s exploration\n", componentID)
		fmt.Printf("      pt discovery option %s A --name \"Option Name\" --desc \"Description\"\n", componentID)
		return nil
	}

	// Show current capabilities
	fmt.Printf("Capabilities for %s:\n", componentID)
	fmt.Println(strings.Repeat("─", 50))

	if len(syn.Capabilities) == 0 {
		fmt.Println("\nNo capabilities yet. Add with:")
		fmt.Printf("  pt discovery capabilities %s --add \"action: Submit order\"\n", componentID)
		fmt.Printf("  pt discovery capabilities %s --add \"info: Show preview\"\n", componentID)
		fmt.Printf("  pt discovery capabilities %s --add \"error: Handle rejection\"\n", componentID)
		return nil
	}

	for _, c := range syn.Capabilities {
		if c.Actor != "" {
			fmt.Printf("  [%s] %s: %s\n", c.ID, c.Actor, c.Goal)
		} else {
			fmt.Printf("  [%s] %s\n", c.ID, c.Goal)
		}
	}

	fmt.Println("\nDiscovery prompts:")
	fmt.Println("  • What other ACTIONS are needed?")
	fmt.Println("  • What INFO must be displayed?")
	fmt.Println("  • What ERRORS must be handled?")
	fmt.Println("  • What's REPETITIVE that could be templated?")

	fmt.Printf("\nAdd more or confirm with: pt discovery capabilities %s --confirm\n", componentID)
	return nil
}

func cmdDiscoveryExplore(args []string) error {
	fs := flag.NewFlagSet("discovery explore", flag.ExitOnError)
	approach := fs.String("approach", "", "Add approach tag")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery explore <component-id> [--approach 'tag']")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Add approach tag
	if *approach != "" {
		found := false
		for _, a := range syn.Exploration.Approaches {
			if a == *approach {
				found = true
				break
			}
		}
		if !found {
			syn.Exploration.Approaches = append(syn.Exploration.Approaches, *approach)
			if err := saveSynthesis(syn); err != nil {
				return err
			}
			fmt.Printf("Added approach: %s\n", *approach)
		}
		return nil
	}

	// Ensure we're in capabilities or exploring phase
	if syn.Status != pt.StatusCapabilities && syn.Status != pt.StatusExploring {
		return fmt.Errorf("cannot explore from status %s", syn.Status)
	}

	if syn.Status == pt.StatusCapabilities {
		if len(syn.Capabilities) == 0 {
			return fmt.Errorf("add capabilities first with: pt discovery capabilities %s --add ...", componentID)
		}
		syn.Status = pt.StatusExploring
		if err := saveSynthesis(syn); err != nil {
			return err
		}
	}

	// Show exploration status
	gate := pt.DefaultExplorationGate()
	fmt.Printf("Exploration: %s (%s)\n", componentID, syn.UXType)
	fmt.Println(strings.Repeat("═", 60))

	// Gate requirements
	fmt.Println("\nGate Requirements:")
	optionsOK := syn.Exploration.TotalOptions >= gate.MinOptions
	approachesOK := len(syn.Exploration.Approaches) >= gate.MinApproaches

	checkMark := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}

	fmt.Printf("  %s Options:    %d/%d minimum\n", checkMark(optionsOK), syn.Exploration.TotalOptions, gate.MinOptions)
	fmt.Printf("  %s Approaches: %d/%d minimum\n", checkMark(approachesOK), len(syn.Exploration.Approaches), gate.MinApproaches)
	if len(syn.Exploration.Approaches) > 0 {
		fmt.Printf("    Tried: %s\n", strings.Join(syn.Exploration.Approaches, ", "))
	}

	// Show existing options
	if len(syn.Options) > 0 {
		fmt.Println("\nCurrent Options:")
		for _, o := range syn.Options {
			fmt.Printf("  [%s] %s\n", o.Label, o.Name)
			if o.Description != "" {
				fmt.Printf("      %s\n", o.Description)
			}
		}
	}

	// Show guidance hint
	fmt.Println("\nCommands:")
	fmt.Printf("  pt discovery guidance %s exploration   # View patterns for %s\n", componentID, syn.UXType)
	fmt.Printf("  pt discovery option %s <label> --name \"...\" --desc \"...\"  # Add option\n", componentID)
	fmt.Printf("  pt discovery explore %s --approach \"tag\"  # Tag approach tried\n", componentID)

	if optionsOK && approachesOK {
		fmt.Printf("\n✓ Gate passed! Ready to synthesize:\n")
		fmt.Printf("  pt discovery synthesize %s\n", componentID)
	}

	return nil
}

func cmdDiscoveryOption(args []string) error {
	fs := flag.NewFlagSet("discovery option", flag.ExitOnError)
	name := fs.String("name", "", "Option name (required)")
	desc := fs.String("desc", "", "Option description")
	mockup := fs.String("mockup", "", "Path to mockup file")
	approach := fs.String("approach", "", "Which approach this uses")
	fs.Parse(args)

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: pt discovery option <component-id> <label> --name \"...\" [--desc \"...\"]")
	}

	componentID := fs.Arg(0)
	label := strings.ToUpper(fs.Arg(1))

	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	if syn.Status != pt.StatusExploring && syn.Status != pt.StatusFeedback {
		return fmt.Errorf("cannot add options in status %s", syn.Status)
	}

	// Check if label already exists
	for i, o := range syn.Options {
		if o.Label == label {
			// Update existing
			syn.Options[i].Name = *name
			if *desc != "" {
				syn.Options[i].Description = *desc
			}
			if *mockup != "" {
				syn.Options[i].Mockup = *mockup
			}
			if err := saveSynthesis(syn); err != nil {
				return err
			}
			fmt.Printf("Updated option [%s]: %s\n", label, *name)
			return nil
		}
	}

	// Create new option
	opt := pt.SynthesisOption{
		Label:       label,
		Name:        *name,
		Description: *desc,
		Mockup:      *mockup,
		Components:  []pt.MockupComponent{},
		Total:       len(syn.Capabilities),
	}

	syn.Options = append(syn.Options, opt)
	syn.Exploration.TotalOptions++

	// Add approach if specified
	if *approach != "" {
		found := false
		for _, a := range syn.Exploration.Approaches {
			if a == *approach {
				found = true
				break
			}
		}
		if !found {
			syn.Exploration.Approaches = append(syn.Exploration.Approaches, *approach)
		}
	}

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	fmt.Printf("Added option [%s]: %s\n", label, *name)
	fmt.Printf("Options explored: %d\n", syn.Exploration.TotalOptions)

	// Suggest next steps
	fmt.Println("\nNext:")
	fmt.Printf("  pt discovery coverage %s %s <cap-id> full  # Mark coverage\n", componentID, label)
	return nil
}

func cmdDiscoveryCoverage(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: pt discovery coverage <component-id> <label> <cap-id|all> <full|partial|none>")
	}

	componentID := args[0]
	label := strings.ToUpper(args[1])
	capID := strings.ToUpper(args[2])
	status := strings.ToLower(args[3])

	if status != "full" && status != "partial" && status != "none" {
		return fmt.Errorf("invalid status %q; use: full, partial, none", status)
	}

	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Find option
	var optIdx int = -1
	for i, o := range syn.Options {
		if o.Label == label {
			optIdx = i
			break
		}
	}
	if optIdx == -1 {
		return fmt.Errorf("option [%s] not found", label)
	}

	// Mark coverage
	if capID == "ALL" {
		count := 0
		for range syn.Capabilities {
			if status == "full" {
				count++
			}
		}
		syn.Options[optIdx].Coverage = count
		syn.Options[optIdx].Total = len(syn.Capabilities)
		fmt.Printf("Marked all capabilities for [%s] as %s\n", label, status)
	} else {
		// Find capability
		found := false
		for _, c := range syn.Capabilities {
			if c.ID == capID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("capability %s not found", capID)
		}

		// This is simplified - in real usage we'd track per-capability
		if status == "full" {
			syn.Options[optIdx].Coverage++
		}
		fmt.Printf("Marked [%s] %s: %s\n", label, capID, status)
	}

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	return nil
}

func cmdDiscoverySynthesize(args []string) error {
	fs := flag.NewFlagSet("discovery synthesize", flag.ExitOnError)
	force := fs.Bool("force", false, "Skip gate validation")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery synthesize <component-id> [--force]")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Validate gate
	gate := pt.DefaultExplorationGate()
	if !*force {
		if syn.Exploration.TotalOptions < gate.MinOptions {
			return fmt.Errorf("need %d+ options (have %d); use --force to override", gate.MinOptions, syn.Exploration.TotalOptions)
		}
		if len(syn.Exploration.Approaches) < gate.MinApproaches {
			return fmt.Errorf("need %d+ approaches (have %d); use --force to override", gate.MinApproaches, len(syn.Exploration.Approaches))
		}
	}

	// Select top 3 by coverage
	if len(syn.Options) > 3 {
		// Sort by coverage (simple bubble sort)
		for i := 0; i < len(syn.Options); i++ {
			for j := i + 1; j < len(syn.Options); j++ {
				if syn.Options[j].Coverage > syn.Options[i].Coverage {
					syn.Options[i], syn.Options[j] = syn.Options[j], syn.Options[i]
				}
			}
		}
		// Move extras to rejected
		for i := 3; i < len(syn.Options); i++ {
			syn.Rejected = append(syn.Rejected, pt.RejectedOption{
				Name:   syn.Options[i].Name,
				Reason: "Lower coverage than top 3 options",
			})
		}
		syn.Options = syn.Options[:3]
	}

	// Relabel as A, B, C
	for i := range syn.Options {
		syn.Options[i].Label = string(rune('A' + i))
	}

	// Set recommendation
	if len(syn.Options) > 0 {
		syn.Recommendation = syn.Options[0].Label
	}

	syn.Status = pt.StatusSynthesized
	syn.SynthesizedAt = time.Now().Format(time.RFC3339)

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	fmt.Println("✓ Synthesis complete!")
	fmt.Println()
	fmt.Printf("Top %d options:\n", len(syn.Options))
	for _, o := range syn.Options {
		rec := ""
		if o.Label == syn.Recommendation {
			rec = " (recommended)"
		}
		fmt.Printf("  [%s] %s%s\n", o.Label, o.Name, rec)
	}

	if len(syn.Rejected) > 0 {
		fmt.Printf("\n%d options rejected.\n", len(syn.Rejected))
	}

	fmt.Println()
	fmt.Printf("Ready for USER review:\n")
	fmt.Printf("  pt discovery review %s\n", componentID)
	return nil
}

func cmdDiscoveryReview(args []string) error {
	fs := flag.NewFlagSet("discovery review", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery review <component-id>")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	if syn.Status != pt.StatusSynthesized && syn.Status != pt.StatusReviewing {
		return fmt.Errorf("synthesis not ready for review (status: %s)", syn.Status)
	}

	// Transition to reviewing
	if syn.Status == pt.StatusSynthesized {
		syn.Status = pt.StatusReviewing
		if err := saveSynthesis(syn); err != nil {
			return err
		}
	}

	// Display comprehensive synthesis artifact
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  SYNTHESIS: %-46s ║\n", componentID)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	fmt.Printf("\nType: %s | Iterations: %d | Explored: %d options\n",
		syn.UXType, syn.Iterations, syn.Exploration.TotalOptions)

	// Capabilities
	fmt.Println("\n┌─ CAPABILITIES ─────────────────────────────────────────────┐")
	for _, c := range syn.Capabilities {
		if c.Actor != "" {
			fmt.Printf("│ [%s] %-7s %s\n", c.ID, c.Actor+":", c.Goal)
		} else {
			fmt.Printf("│ [%s] %s\n", c.ID, c.Goal)
		}
	}
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// Options with component IDs
	fmt.Println("\n┌─ TOP OPTIONS ──────────────────────────────────────────────┐")
	for _, o := range syn.Options {
		rec := ""
		if o.Label == syn.Recommendation {
			rec = " ★ RECOMMENDED"
		}
		fmt.Printf("│\n│ [%s] %s%s\n", o.Label, o.Name, rec)
		fmt.Printf("│     %s\n", o.Description)
		fmt.Printf("│     Coverage: %d/%d | ", o.Coverage, o.Total)
		if len(o.Gaps) > 0 {
			fmt.Printf("Gaps: %s\n", strings.Join(o.Gaps, ", "))
		} else {
			fmt.Println("Full coverage")
		}

		if len(o.Components) > 0 {
			fmt.Println("│")
			fmt.Println("│     Components:")
			for _, comp := range o.Components {
				fmt.Printf("│       [%s] %s: %s\n", comp.ID, comp.Type, comp.Content)
			}
		}

		if o.Rationale != "" {
			fmt.Printf("│     Rationale: %s\n", o.Rationale)
		}
	}
	fmt.Println("│")
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// Rejected
	if len(syn.Rejected) > 0 {
		fmt.Println("\n┌─ REJECTED ─────────────────────────────────────────────────┐")
		for _, r := range syn.Rejected {
			fmt.Printf("│ ✗ %s: %s\n", r.Name, r.Reason)
		}
		fmt.Println("└────────────────────────────────────────────────────────────┘")
	}

	// Feedback
	if len(syn.Feedback) > 0 {
		fmt.Println("\n┌─ FEEDBACK ─────────────────────────────────────────────────┐")
		for _, f := range syn.Feedback {
			status := "○"
			if f.Addressed {
				status = "●"
			}
			target := ""
			if f.ComponentID != "" {
				target = fmt.Sprintf("[%s] ", f.ComponentID)
			} else if f.OptionLabel != "" {
				target = fmt.Sprintf("[%s] ", f.OptionLabel)
			}
			fmt.Printf("│ %s %s%s\n", status, target, f.Feedback)
		}
		fmt.Println("└────────────────────────────────────────────────────────────┘")
	}

	// Actions
	fmt.Println("\n┌─ ACTIONS ──────────────────────────────────────────────────┐")
	fmt.Printf("│ pt discovery feedback %s --component A3 \"Your feedback\"\n", componentID)
	fmt.Printf("│ pt discovery feedback %s \"General feedback\"\n", componentID)
	fmt.Printf("│ pt discovery approve %s\n", componentID)
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	return nil
}

func cmdDiscoveryFeedback(args []string) error {
	fs := flag.NewFlagSet("discovery feedback", flag.ExitOnError)
	componentID_flag := fs.String("component", "", "Target component ID (e.g., A3)")
	optionLabel := fs.String("option", "", "Target option label (e.g., A)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: pt discovery feedback <component-id> \"feedback text\" [--component A3] [--option A]")
	}

	componentID := fs.Arg(0)
	feedbackText := strings.Join(fs.Args()[1:], " ")

	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	if syn.Status != pt.StatusReviewing && syn.Status != pt.StatusFeedback {
		return fmt.Errorf("cannot add feedback in status %s", syn.Status)
	}

	// Create feedback item
	fID := fmt.Sprintf("F%d", len(syn.Feedback)+1)
	feedback := pt.FeedbackItem{
		ID:          fID,
		ComponentID: *componentID_flag,
		OptionLabel: *optionLabel,
		Feedback:    feedbackText,
		Addressed:   false,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	syn.Feedback = append(syn.Feedback, feedback)
	syn.Status = pt.StatusFeedback

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	target := ""
	if *componentID_flag != "" {
		target = fmt.Sprintf(" on [%s]", *componentID_flag)
	} else if *optionLabel != "" {
		target = fmt.Sprintf(" on option [%s]", *optionLabel)
	}

	fmt.Printf("✓ Feedback [%s] added%s: %s\n", fID, target, feedbackText)
	fmt.Printf("\nAgent should iterate with: pt discovery iterate %s\n", componentID)
	return nil
}

func cmdDiscoveryIterate(args []string) error {
	fs := flag.NewFlagSet("discovery iterate", flag.ExitOnError)
	address := fs.String("address", "", "Mark feedback ID as addressed")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery iterate <component-id> [--address F1]")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Mark feedback as addressed
	if *address != "" {
		for i, f := range syn.Feedback {
			if f.ID == *address {
				syn.Feedback[i].Addressed = true
				syn.Feedback[i].AddressedIn = fmt.Sprintf("iteration-%d", syn.Iterations+1)
				if err := saveSynthesis(syn); err != nil {
					return err
				}
				fmt.Printf("✓ Feedback [%s] marked as addressed\n", *address)
				return nil
			}
		}
		return fmt.Errorf("feedback %s not found", *address)
	}

	// Show unaddressed feedback
	fmt.Printf("Iteration: %s (round %d)\n", componentID, syn.Iterations+1)
	fmt.Println(strings.Repeat("─", 50))

	unaddressed := []pt.FeedbackItem{}
	for _, f := range syn.Feedback {
		if !f.Addressed {
			unaddressed = append(unaddressed, f)
		}
	}

	if len(unaddressed) == 0 {
		fmt.Println("\n✓ All feedback addressed!")
		fmt.Printf("Ready to re-synthesize: pt discovery synthesize %s\n", componentID)
		syn.Status = pt.StatusExploring
		syn.Iterations++
		return saveSynthesis(syn)
	}

	fmt.Printf("\nUnaddressed feedback (%d):\n", len(unaddressed))
	for _, f := range unaddressed {
		target := ""
		if f.ComponentID != "" {
			target = fmt.Sprintf("[%s] ", f.ComponentID)
		} else if f.OptionLabel != "" {
			target = fmt.Sprintf("[%s] ", f.OptionLabel)
		}
		fmt.Printf("  [%s] %s%s\n", f.ID, target, f.Feedback)
	}

	fmt.Println("\nMark as addressed:")
	for _, f := range unaddressed {
		fmt.Printf("  pt discovery iterate %s --address %s\n", componentID, f.ID)
	}

	return nil
}

func cmdDiscoveryApprove(args []string) error {
	fs := flag.NewFlagSet("discovery approve", flag.ExitOnError)
	selection := fs.String("select", "", "Override recommendation (e.g., 'B' or 'A+C')")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery approve <component-id> [--select B]")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	if syn.Status != pt.StatusReviewing {
		return fmt.Errorf("cannot approve from status %s; must be reviewing", syn.Status)
	}

	// Check for unaddressed feedback
	unaddressed := 0
	for _, f := range syn.Feedback {
		if !f.Addressed {
			unaddressed++
		}
	}
	if unaddressed > 0 {
		fmt.Printf("⚠ %d unaddressed feedback items:\n", unaddressed)
		for _, f := range syn.Feedback {
			if !f.Addressed {
				fmt.Printf("  [%s] %s\n", f.ID, f.Feedback)
			}
		}
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nApprove anyway? (y/N): ")
		input, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			return fmt.Errorf("approval cancelled; address feedback first")
		}
	}

	// Update selection if provided
	if *selection != "" {
		syn.Recommendation = strings.ToUpper(*selection)
	}

	syn.Status = pt.StatusApproved
	syn.ApprovedAt = time.Now().Format(time.RFC3339)

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  ✓ APPROVED: %-45s ║\n", componentID)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nSelected: %s\n", syn.Recommendation)
	fmt.Printf("\nGenerate implementation guidance:\n")
	fmt.Printf("  pt discovery handoff %s\n", componentID)
	return nil
}

func cmdDiscoveryHandoff(args []string) error {
	fs := flag.NewFlagSet("discovery handoff", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery handoff <component-id>")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	if syn.Status != pt.StatusApproved {
		return fmt.Errorf("cannot generate handoff until approved (status: %s)", syn.Status)
	}

	// Find selected option
	var selected *pt.SynthesisOption
	for i, o := range syn.Options {
		if o.Label == syn.Recommendation {
			selected = &syn.Options[i]
			break
		}
	}

	if selected == nil {
		return fmt.Errorf("selected option %s not found", syn.Recommendation)
	}

	// Generate handoff document
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  IMPLEMENTATION HANDOFF: %-33s ║\n", componentID)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	fmt.Printf("\nComponent: %s\n", componentID)
	fmt.Printf("UX Type: %s\n", syn.UXType)
	fmt.Printf("Selected: [%s] %s\n", selected.Label, selected.Name)

	// Type-specific guidance
	fmt.Println("\n┌─ IMPLEMENTATION GUIDANCE ──────────────────────────────────┐")
	printImplementationGuidance(syn.UXType, selected)
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// Capabilities as acceptance criteria
	fmt.Println("\n┌─ ACCEPTANCE CRITERIA ──────────────────────────────────────┐")
	for _, c := range syn.Capabilities {
		fmt.Printf("│ ☐ [%s] %s\n", c.ID, c.Goal)
	}
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// Component mappings
	if len(selected.Components) > 0 {
		fmt.Println("\n┌─ COMPONENT MAPPINGS ───────────────────────────────────────┐")
		for _, comp := range selected.Components {
			impl := comp.Implementation
			if impl == "" {
				impl = "(to be implemented)"
			}
			fmt.Printf("│ [%s] %s → %s\n", comp.ID, comp.Type, impl)
		}
		fmt.Println("└────────────────────────────────────────────────────────────┘")
	}

	return nil
}

func printImplementationGuidance(uxType string, opt *pt.SynthesisOption) {
	switch uxType {
	case "cli":
		fmt.Println("│ CLI Pattern: Command with flags + optional interactive mode")
		fmt.Println("│")
		fmt.Println("│ Libraries:")
		fmt.Println("│   - github.com/spf13/cobra (command structure)")
		fmt.Println("│   - github.com/spf13/pflag (flag parsing)")
		fmt.Println("│   - github.com/charmbracelet/lipgloss (styling)")
		fmt.Println("│")
		fmt.Println("│ Patterns:")
		fmt.Println("│   - Flags: --symbol, --quantity, --preview")
		fmt.Println("│   - Interactive prompts for missing required values")
		fmt.Println("│   - JSON/table output modes (--json, --format table)")
		fmt.Println("│   - Dry-run mode (--dry-run)")
	case "tui":
		fmt.Println("│ TUI Pattern: Full-screen terminal UI with keyboard nav")
		fmt.Println("│")
		fmt.Println("│ Libraries:")
		fmt.Println("│   - github.com/charmbracelet/bubbletea (framework)")
		fmt.Println("│   - github.com/charmbracelet/bubbles (components)")
		fmt.Println("│   - github.com/charmbracelet/lipgloss (styling)")
		fmt.Println("│")
		fmt.Println("│ Patterns:")
		fmt.Println("│   - Model-Update-View architecture")
		fmt.Println("│   - Key bindings: j/k (nav), enter (select), q (quit)")
		fmt.Println("│   - Status bar with context")
		fmt.Println("│   - Help overlay (? key)")
	case "web":
		fmt.Println("│ Web Pattern: React SPA with component library")
		fmt.Println("│")
		fmt.Println("│ Libraries:")
		fmt.Println("│   - React 18+ with TypeScript")
		fmt.Println("│   - Tailwind CSS (styling)")
		fmt.Println("│   - shadcn/ui (components)")
		fmt.Println("│   - react-query (data fetching)")
		fmt.Println("│")
		fmt.Println("│ Patterns:")
		fmt.Println("│   - Form components with zod validation")
		fmt.Println("│   - Loading states with skeletons")
		fmt.Println("│   - Error boundaries")
		fmt.Println("│   - Responsive breakpoints")
	case "api":
		fmt.Println("│ API Pattern: REST endpoints with OpenAPI")
		fmt.Println("│")
		fmt.Println("│ Libraries:")
		fmt.Println("│   - chi or echo (routing)")
		fmt.Println("│   - oapi-codegen (OpenAPI)")
		fmt.Println("│   - slog (logging)")
		fmt.Println("│")
		fmt.Println("│ Patterns:")
		fmt.Println("│   - RESTful resource naming")
		fmt.Println("│   - Request validation middleware")
		fmt.Println("│   - Error response format")
		fmt.Println("│   - Rate limiting")
	}
}

func cmdDiscoveryGuidance(args []string) error {
	fs := flag.NewFlagSet("discovery guidance", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: pt discovery guidance <component-id> <exploration|implementation>")
	}

	componentID := fs.Arg(0)
	phase := fs.Arg(1)

	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	switch phase {
	case "exploration", "explore":
		return printExplorationGuidance(syn.UXType)
	case "implementation", "impl":
		return printImplGuidance(syn.UXType)
	default:
		return fmt.Errorf("unknown phase %q; use: exploration, implementation", phase)
	}
}

func printExplorationGuidance(uxType string) error {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  EXPLORATION GUIDANCE: %-35s ║\n", strings.ToUpper(uxType))
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	switch uxType {
	case "cli":
		fmt.Println(`
APPROACHES TO TRY:
  1. Flags-only (fully scriptable, no prompts)
  2. Interactive wizard (step-by-step prompts)
  3. Hybrid (flags set defaults, prompts fill gaps)
  4. Subcommand structure (git-style)
  5. REPL mode (persistent session)

PATTERNS TO CONSIDER:
  • Flag sets: --symbol, --quantity, --type
  • Short flags: -s, -q, -t for common options
  • Boolean flags: --preview, --dry-run, --force
  • Output formats: --json, --format table|json|csv
  • Config file: --config for persistent settings
  • Piping: stdin/stdout for composability

COMPONENT IDS TO USE:
  [A1] Command name/help
  [A2] Required flags
  [A3] Optional flags
  [A4] Interactive prompts
  [A5] Output display
  [A6] Error messages
  [A7] Confirmation step

MINIMUM REQUIREMENTS:
  ✓ Show error handling approach
  ✓ Show help output
  ✓ Consider scriptability
  ✓ Define exit codes`)

	case "tui":
		fmt.Println(`
APPROACHES TO TRY:
  1. Single-screen form
  2. Multi-panel dashboard
  3. Wizard with progress
  4. Menu-driven navigation
  5. Split view (list + detail)

PATTERNS TO CONSIDER:
  • Navigation: j/k or arrows, tab between sections
  • Actions: enter (select), space (toggle), q (quit)
  • Help: ? for overlay, F1 for context help
  • Status bar: mode, errors, hints
  • Modal dialogs: confirmation, input
  • Live updates: auto-refresh, loading states

COMPONENT IDS TO USE:
  [A1] Header/title bar
  [A2] Main content area
  [A3] Input fields
  [A4] List/table
  [A5] Status bar
  [A6] Key hints
  [A7] Modal/overlay

MINIMUM REQUIREMENTS:
  ✓ Define keyboard shortcuts
  ✓ Show focus indicators
  ✓ Handle resize
  ✓ Error state display`)

	case "web":
		fmt.Println(`
APPROACHES TO TRY:
  1. Single-page form with sections
  2. Multi-step wizard with progress
  3. Dashboard with drill-down
  4. Modal-based workflow
  5. Inline editing (table/list)

PATTERNS TO CONSIDER:
  • Forms: zod validation, inline errors
  • Loading: skeleton screens, spinners
  • Navigation: breadcrumbs, back buttons
  • Feedback: toasts, inline messages
  • Mobile: responsive, touch targets
  • Accessibility: labels, focus management

COMPONENT IDS TO USE:
  [A1] Page header
  [A2] Form container
  [A3] Input groups
  [A4] Action buttons
  [A5] Results display
  [A6] Error messages
  [A7] Loading states

MINIMUM REQUIREMENTS:
  ✓ Form validation states
  ✓ Loading indicators
  ✓ Error handling
  ✓ Mobile responsive`)

	case "api":
		fmt.Println(`
APPROACHES TO TRY:
  1. REST CRUD endpoints
  2. GraphQL with mutations
  3. RPC-style actions
  4. Event-driven (webhooks)
  5. Batch operations

PATTERNS TO CONSIDER:
  • Resources: /orders, /positions
  • Actions: POST /orders/{id}/cancel
  • Queries: ?status=open&limit=10
  • Errors: structured error response
  • Auth: Bearer token, API key
  • Versioning: /v1/, Accept header

COMPONENT IDS TO USE:
  [A1] Endpoint path
  [A2] Request body
  [A3] Response body
  [A4] Error responses
  [A5] Query params
  [A6] Headers
  [A7] Auth flow

MINIMUM REQUIREMENTS:
  ✓ Error response format
  ✓ Validation messages
  ✓ Rate limiting
  ✓ Auth handling`)
	}

	fmt.Println("\n" + strings.Repeat("─", 60))
	fmt.Println("Use component IDs in mockups for precise feedback.")
	fmt.Println("Try at least 2 different approaches before synthesizing.")
	return nil
}

func printImplGuidance(uxType string) error {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  IMPLEMENTATION GUIDANCE: %-32s ║\n", strings.ToUpper(uxType))
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	switch uxType {
	case "cli":
		fmt.Println(`
STRUCTURE:
  cmd/
    myapp/
      main.go         # Entry point
      cmd_order.go    # Order command
      flags.go        # Shared flags
      output.go       # Output formatting

LIBRARIES:
  go get github.com/spf13/cobra
  go get github.com/spf13/pflag
  go get github.com/charmbracelet/lipgloss

EXAMPLE CODE:
  var orderCmd = &cobra.Command{
      Use:   "order",
      Short: "Manage orders",
      RunE:  runOrder,
  }

  func init() {
      orderCmd.Flags().StringP("symbol", "s", "", "Symbol")
      orderCmd.Flags().IntP("quantity", "q", 0, "Quantity")
      orderCmd.Flags().Bool("preview", false, "Preview only")
  }`)

	case "tui":
		fmt.Println(`
STRUCTURE:
  internal/
    tui/
      model.go        # Main model
      view.go         # Render functions
      update.go       # Event handling
      components/     # Reusable components

LIBRARIES:
  go get github.com/charmbracelet/bubbletea
  go get github.com/charmbracelet/bubbles
  go get github.com/charmbracelet/lipgloss

EXAMPLE CODE:
  type model struct {
      state    State
      form     *huh.Form
      list     list.Model
      err      error
  }

  func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
      switch msg := msg.(type) {
      case tea.KeyMsg:
          switch msg.String() {
          case "q", "ctrl+c":
              return m, tea.Quit
          }
      }
      return m, nil
  }`)

	case "web":
		fmt.Println(`
STRUCTURE:
  src/
    components/
      order/
        OrderForm.tsx
        OrderPreview.tsx
        OrderConfirm.tsx
    hooks/
      useOrder.ts
    lib/
      api.ts

LIBRARIES:
  npx shadcn@latest init
  npm install @tanstack/react-query zod react-hook-form

EXAMPLE CODE:
  export function OrderForm() {
    const form = useForm<OrderInput>({
      resolver: zodResolver(orderSchema),
    });

    return (
      <Form {...form}>
        <FormField
          control={form.control}
          name="symbol"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Symbol</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </Form>
    );
  }`)

	case "api":
		fmt.Println(`
STRUCTURE:
  internal/
    api/
      handler.go      # HTTP handlers
      routes.go       # Route setup
      middleware.go   # Auth, logging
    openapi/
      spec.yaml       # OpenAPI spec
      generated/      # Generated code

LIBRARIES:
  go get github.com/go-chi/chi/v5
  go get github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen

EXAMPLE CODE:
  func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
      var req CreateOrderRequest
      if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
          h.Error(w, err, http.StatusBadRequest)
          return
      }

      if err := h.validate.Struct(req); err != nil {
          h.ValidationError(w, err)
          return
      }

      order, err := h.service.CreateOrder(r.Context(), req)
      if err != nil {
          h.Error(w, err, http.StatusInternalServerError)
          return
      }

      h.JSON(w, order, http.StatusCreated)
  }`)
	}

	return nil
}
