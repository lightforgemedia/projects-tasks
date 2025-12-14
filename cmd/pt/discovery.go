package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	case "mockup":
		return cmdDiscoveryMockup(subArgs)
	case "component":
		return cmdDiscoveryComponent(subArgs)
	case "persona":
		return cmdDiscoveryPersona(subArgs)
	case "edge-case":
		return cmdDiscoveryEdgeCase(subArgs)
	case "usability":
		return cmdDiscoveryUsability(subArgs)
	case "friction":
		return cmdDiscoveryFriction(subArgs)
	case "help":
		return cmdDiscoveryHelp()
	default:
		return fmt.Errorf("unknown discovery subcommand: %s\nRun 'pt discovery help' for usage", subCmd)
	}
}

func cmdDiscoveryHelp() error {
	fmt.Println(`Discovery Workflow - Synthesis-Based UX Exploration

PHASES:
  1. INIT         → Create synthesis, define personas
  2. CAPABILITIES → Define what must be supported
  3. EXPLORE      → Generate 5+ options, 2+ approaches, cover edge cases
  4. SYNTHESIZE   → Narrow to top 3 with rationale
  5. USABILITY    → Run usability review (persona fit, friction, gaps)
  6. REVIEW       → User reviews comprehensive artifact
  7. APPROVE      → Final approval, generate handoff

CORE COMMANDS:
  pt discovery init <component> --type <cli|tui|web|api>
  pt discovery capabilities <component> --add "category: capability"
  pt discovery explore <component>
  pt discovery option <component> <label> --name "..." --desc "..."
  pt discovery synthesize <component>
  pt discovery review <component>
  pt discovery approve <component>

USABILITY COMMANDS:
  pt discovery persona <component> --add "Day Trader" --goals "fast execution"
  pt discovery edge-case <component> --cover <empty|loading|error|overflow> --in <option>
  pt discovery usability <component>              # Run full usability review
  pt discovery friction <component> --add "..." --severity high

MOCKUP COMMANDS:
  pt discovery mockup <component> <label>         # Create/view mockup
  pt discovery component <component> <label> <id> # Label components

EXAMPLES:
  # Initialize with persona
  pt discovery init order --type web
  pt discovery persona order --add "Day Trader" --goals "Execute trades in <3 clicks"
  pt discovery persona order --add "New User" --goals "Understand options before commit"

  # Explore with edge cases
  pt discovery capabilities order --add "action: Submit order"
  pt discovery explore order
  pt discovery option order A --name "Quick Trade" --desc "Minimal steps"
  pt discovery edge-case order --cover empty --in A
  pt discovery edge-case order --cover error --in A

  # Usability review before synthesis
  pt discovery usability order    # Shows persona fit, friction, gaps
  pt discovery synthesize order

  # User reviews and approves
  pt discovery review order
  pt discovery approve order`)
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
		Personas:  []pt.Persona{},
		EdgeCases: pt.RequiredEdgeCases(), // Initialize with required edge cases
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	fmt.Printf("✓ Discovery initialized for [%s] (type: %s)\n", componentID, *uxType)
	fmt.Printf("  Edge cases to cover: empty, loading, error, overflow, offline\n")
	fmt.Println("\nNext steps:")
	fmt.Printf("  pt discovery persona %s --add \"User Type\" --goals \"what they need\"\n", componentID)
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

	// Edge case coverage
	coveredEdgeCases := 0
	uncoveredEdgeCases := []string{}
	for _, ec := range syn.EdgeCases {
		if ec.Covered {
			coveredEdgeCases++
		} else {
			uncoveredEdgeCases = append(uncoveredEdgeCases, ec.ID)
		}
	}
	minEdgeCases := 3 // Require at least empty, loading, error
	edgeCasesOK := coveredEdgeCases >= minEdgeCases
	fmt.Printf("  %s Edge cases: %d/%d covered\n", checkMark(edgeCasesOK), coveredEdgeCases, len(syn.EdgeCases))
	if len(uncoveredEdgeCases) > 0 {
		fmt.Printf("    Missing: %s\n", strings.Join(uncoveredEdgeCases, ", "))
	}

	// Persona coverage
	personasOK := len(syn.Personas) >= 1
	fmt.Printf("  %s Personas:   %d defined\n", checkMark(personasOK), len(syn.Personas))

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
	fmt.Printf("  pt discovery edge-case %s --cover empty --in A  # Mark edge case covered\n", componentID)
	fmt.Printf("  pt discovery explore %s --approach \"tag\"  # Tag approach tried\n", componentID)

	allGatesOK := optionsOK && approachesOK && edgeCasesOK && personasOK
	if allGatesOK {
		fmt.Printf("\n✓ All gates passed! Ready to synthesize:\n")
		fmt.Printf("  pt discovery usability %s  # Run usability review first (recommended)\n", componentID)
		fmt.Printf("  pt discovery synthesize %s\n", componentID)
	} else {
		fmt.Println("\n⚠ Address missing gates before synthesis.")
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

// mockupDir returns the directory for discovery mockups
func discoveryMockupDir(componentID string) string {
	return filepath.Join(".pt", "discovery", componentID, "mockups")
}

// mockupFilePath returns the path to a mockup file
func mockupFilePath(componentID, label string) string {
	return filepath.Join(discoveryMockupDir(componentID), fmt.Sprintf("option-%s.txt", strings.ToLower(label)))
}

func cmdDiscoveryMockup(args []string) error {
	fs := flag.NewFlagSet("discovery mockup", flag.ExitOnError)
	fileFlag := fs.String("file", "", "Read mockup content from file")
	fs.Usage = func() {
		fmt.Println("Usage: pt discovery mockup <component-id> [label] [--file FILE]")
		fmt.Println("\nCreate, update, or view ASCII mockups with component IDs.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt discovery mockup order              # List all mockups")
		fmt.Println("  pt discovery mockup order A            # View mockup A")
		fmt.Println("  pt discovery mockup order A --file x   # Create from file")
		fmt.Println("  pt discovery mockup order A < mock.txt # Create from stdin")
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing component-id")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// No label = list mockups
	if fs.NArg() < 2 {
		return listDiscoveryMockups(componentID, syn)
	}

	label := strings.ToUpper(fs.Arg(1))

	// Check if creating or viewing
	hasInput := *fileFlag != "" || !isTerminal()

	if hasInput {
		return createDiscoveryMockup(componentID, label, *fileFlag, syn)
	}

	return viewDiscoveryMockup(componentID, label, syn)
}

func listDiscoveryMockups(componentID string, syn *pt.UXSynthesis) error {
	fmt.Printf("Mockups: %s\n", componentID)
	fmt.Println(strings.Repeat("─", 50))

	dir := discoveryMockupDir(componentID)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		fmt.Println("\nNo mockups yet. Create with:")
		fmt.Printf("  pt discovery mockup %s A < mockup.txt\n", componentID)
		fmt.Printf("  pt discovery mockup %s A --file design.txt\n", componentID)
		return nil
	}

	fmt.Println()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		label := strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(e.Name(), "option-"), ".txt"))
		fmt.Printf("  [%s] %s\n", label, e.Name())
	}

	fmt.Printf("\nView: pt discovery mockup %s <label>\n", componentID)
	return nil
}

func createDiscoveryMockup(componentID, label, filePath string, syn *pt.UXSynthesis) error {
	// Ensure directory
	dir := discoveryMockupDir(componentID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create mockup dir: %w", err)
	}

	// Read content
	var content []byte
	var err error
	if filePath != "" {
		content, err = os.ReadFile(filePath)
	} else {
		content, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return err
	}

	if len(content) == 0 {
		return fmt.Errorf("empty mockup content")
	}

	// Write file
	path := mockupFilePath(componentID, label)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write mockup: %w", err)
	}

	// Update option in synthesis
	for i, o := range syn.Options {
		if o.Label == label {
			syn.Options[i].Mockup = path
			if err := saveSynthesis(syn); err != nil {
				return err
			}
			break
		}
	}

	fmt.Printf("Created mockup [%s]: %s\n", label, path)
	fmt.Printf("View: pt discovery mockup %s %s\n", componentID, label)
	fmt.Printf("Add components: pt discovery component %s %s <id> --type <type> --content \"...\"\n", componentID, label)
	return nil
}

func viewDiscoveryMockup(componentID, label string, syn *pt.UXSynthesis) error {
	path := mockupFilePath(componentID, label)
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("no mockup [%s] found; create with: pt discovery mockup %s %s < file.txt", label, componentID, label)
	}

	// Find option for components
	var opt *pt.SynthesisOption
	for i, o := range syn.Options {
		if o.Label == label {
			opt = &syn.Options[i]
			break
		}
	}

	fmt.Printf("Mockup [%s]:\n", label)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println(string(content))

	if opt != nil && len(opt.Components) > 0 {
		fmt.Println()
		fmt.Println("Components:")
		for _, c := range opt.Components {
			impl := ""
			if c.Implementation != "" {
				impl = fmt.Sprintf(" → %s", c.Implementation)
			}
			fmt.Printf("  [%s] %s: %s%s\n", c.ID, c.Type, c.Content, impl)
		}
	}

	return nil
}

func cmdDiscoveryComponent(args []string) error {
	fs := flag.NewFlagSet("discovery component", flag.ExitOnError)
	compType := fs.String("type", "", "Component type: header|input|button|list|table|output|error")
	content := fs.String("content", "", "Component description")
	impl := fs.String("impl", "", "Implementation hint (e.g., '<Select> from shadcn/ui')")
	notes := fs.String("notes", "", "Design notes")
	fs.Usage = func() {
		fmt.Println("Usage: pt discovery component <component-id> <option-label> <comp-id> [flags]")
		fmt.Println("\nAdd or update component labels in a mockup.")
		fmt.Println("\nFlags:")
		fmt.Println("  --type      Component type (header|input|button|list|table|output|error)")
		fmt.Println("  --content   What this component shows/does")
		fmt.Println("  --impl      Implementation hint (library, pattern)")
		fmt.Println("  --notes     Design rationale")
		fmt.Println("\nExamples:")
		fmt.Println("  pt discovery component order A A1 --type header --content \"Order Entry\"")
		fmt.Println("  pt discovery component order A A2 --type input --content \"Symbol\" --impl \"<Input> + autocomplete\"")
		fmt.Println("  pt discovery component order A A3 --type button --content \"Submit Order\"")
	}
	fs.Parse(args)

	if fs.NArg() < 3 {
		fs.Usage()
		return fmt.Errorf("usage: pt discovery component <component-id> <option-label> <comp-id> --type <type> --content \"...\"")
	}

	componentID := fs.Arg(0)
	optionLabel := strings.ToUpper(fs.Arg(1))
	compID := fs.Arg(2)

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
		if o.Label == optionLabel {
			optIdx = i
			break
		}
	}
	if optIdx == -1 {
		return fmt.Errorf("option [%s] not found", optionLabel)
	}

	// Find or create component
	found := false
	for i, c := range syn.Options[optIdx].Components {
		if c.ID == compID {
			// Update existing
			if *compType != "" {
				syn.Options[optIdx].Components[i].Type = *compType
			}
			if *content != "" {
				syn.Options[optIdx].Components[i].Content = *content
			}
			if *impl != "" {
				syn.Options[optIdx].Components[i].Implementation = *impl
			}
			if *notes != "" {
				syn.Options[optIdx].Components[i].Notes = *notes
			}
			found = true
			fmt.Printf("Updated component [%s] in option [%s]\n", compID, optionLabel)
			break
		}
	}

	if !found {
		// Create new
		if *compType == "" {
			return fmt.Errorf("--type is required for new components")
		}
		if *content == "" {
			return fmt.Errorf("--content is required for new components")
		}

		comp := pt.MockupComponent{
			ID:             compID,
			Type:           *compType,
			Content:        *content,
			Implementation: *impl,
			Notes:          *notes,
		}
		syn.Options[optIdx].Components = append(syn.Options[optIdx].Components, comp)
		fmt.Printf("Added component [%s] to option [%s]\n", compID, optionLabel)
	}

	if err := saveSynthesis(syn); err != nil {
		return err
	}

	// Show all components
	fmt.Printf("\nComponents in [%s]:\n", optionLabel)
	for _, c := range syn.Options[optIdx].Components {
		implHint := ""
		if c.Implementation != "" {
			implHint = fmt.Sprintf(" → %s", c.Implementation)
		}
		fmt.Printf("  [%s] %s: %s%s\n", c.ID, c.Type, c.Content, implHint)
	}

	return nil
}

// ============================================================================
// USABILITY COMMANDS - Persona, Edge Cases, Friction, Usability Review
// ============================================================================

func cmdDiscoveryPersona(args []string) error {
	fs := flag.NewFlagSet("discovery persona", flag.ExitOnError)
	add := fs.String("add", "", "Add persona by name")
	goals := fs.String("goals", "", "Persona goals (comma-separated)")
	constraints := fs.String("constraints", "", "Constraints (e.g., 'time pressure, low expertise')")
	frequency := fs.String("frequency", "daily", "Usage frequency: daily|weekly|occasional")
	fs.Usage = func() {
		fmt.Println("Usage: pt discovery persona <component-id> [--add 'name'] [--goals '...']")
		fmt.Println("\nDefine who uses this component and their needs.")
		fmt.Println("\nExamples:")
		fmt.Println("  pt discovery persona order --add \"Day Trader\" --goals \"fast execution\" --constraints \"time pressure\"")
		fmt.Println("  pt discovery persona order --add \"New User\" --goals \"understand before commit\" --frequency occasional")
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing component-id")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Add new persona
	if *add != "" {
		id := fmt.Sprintf("P%d", len(syn.Personas)+1)
		persona := pt.Persona{
			ID:        id,
			Name:      *add,
			Frequency: *frequency,
		}
		if *goals != "" {
			persona.Goals = strings.Split(*goals, ",")
			for i := range persona.Goals {
				persona.Goals[i] = strings.TrimSpace(persona.Goals[i])
			}
		}
		if *constraints != "" {
			persona.Constraints = strings.Split(*constraints, ",")
			for i := range persona.Constraints {
				persona.Constraints[i] = strings.TrimSpace(persona.Constraints[i])
			}
		}
		syn.Personas = append(syn.Personas, persona)
		if err := saveSynthesis(syn); err != nil {
			return err
		}
		fmt.Printf("Added persona [%s]: %s\n", id, *add)
		if len(persona.Goals) > 0 {
			fmt.Printf("  Goals: %s\n", strings.Join(persona.Goals, ", "))
		}
		if len(persona.Constraints) > 0 {
			fmt.Printf("  Constraints: %s\n", strings.Join(persona.Constraints, ", "))
		}
		return nil
	}

	// Show personas
	fmt.Printf("Personas for %s:\n", componentID)
	fmt.Println(strings.Repeat("─", 50))

	if len(syn.Personas) == 0 {
		fmt.Println("\nNo personas defined yet.")
		fmt.Println("\nPersonas help evaluate if the UX fits real user needs.")
		fmt.Println("Add one with:")
		fmt.Printf("  pt discovery persona %s --add \"User Type\" --goals \"what they need\"\n", componentID)
		return nil
	}

	for _, p := range syn.Personas {
		fmt.Printf("\n[%s] %s (%s)\n", p.ID, p.Name, p.Frequency)
		if len(p.Goals) > 0 {
			fmt.Printf("  Goals: %s\n", strings.Join(p.Goals, ", "))
		}
		if len(p.Constraints) > 0 {
			fmt.Printf("  Constraints: %s\n", strings.Join(p.Constraints, ", "))
		}
	}

	return nil
}

func cmdDiscoveryEdgeCase(args []string) error {
	fs := flag.NewFlagSet("discovery edge-case", flag.ExitOnError)
	cover := fs.String("cover", "", "Mark edge case as covered: empty|loading|error|overflow|offline")
	inOption := fs.String("in", "", "Which option covers this (e.g., A)")
	fs.Usage = func() {
		fmt.Println("Usage: pt discovery edge-case <component-id> [--cover <case> --in <option>]")
		fmt.Println("\nTrack edge case coverage in mockups.")
		fmt.Println("\nRequired edge cases:")
		fmt.Println("  empty    - No data to display")
		fmt.Println("  loading  - Fetching data")
		fmt.Println("  error    - Something went wrong")
		fmt.Println("  overflow - Too many items (100+)")
		fmt.Println("  offline  - Network unavailable")
		fmt.Println("\nExamples:")
		fmt.Println("  pt discovery edge-case order --cover empty --in A")
		fmt.Println("  pt discovery edge-case order --cover error --in A")
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing component-id")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Mark edge case as covered
	if *cover != "" {
		if *inOption == "" {
			return fmt.Errorf("--in is required (which option covers this)")
		}
		found := false
		for i, ec := range syn.EdgeCases {
			if ec.ID == *cover {
				syn.EdgeCases[i].Covered = true
				syn.EdgeCases[i].CoveredIn = strings.ToUpper(*inOption)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown edge case %q; use: empty, loading, error, overflow, offline", *cover)
		}
		if err := saveSynthesis(syn); err != nil {
			return err
		}
		fmt.Printf("✓ Edge case [%s] marked as covered in option [%s]\n", *cover, strings.ToUpper(*inOption))
		return nil
	}

	// Show edge cases
	fmt.Printf("Edge Cases for %s:\n", componentID)
	fmt.Println(strings.Repeat("─", 50))

	covered := 0
	for _, ec := range syn.EdgeCases {
		status := "○"
		coveredIn := ""
		if ec.Covered {
			status = "●"
			covered++
			coveredIn = fmt.Sprintf(" (in %s)", ec.CoveredIn)
		}
		fmt.Printf("  %s %-10s %s%s\n", status, ec.ID, ec.Description, coveredIn)
	}

	fmt.Printf("\nCoverage: %d/%d\n", covered, len(syn.EdgeCases))
	if covered < 3 {
		fmt.Println("\n⚠ Cover at least: empty, loading, error before synthesis")
	}

	return nil
}

func cmdDiscoveryUsability(args []string) error {
	fs := flag.NewFlagSet("discovery usability", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt discovery usability <component-id>")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Run usability review
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  USABILITY REVIEW: %-39s ║\n", componentID)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	score := 0
	maxScore := 0
	gaps := []string{}

	// 1. PERSONA FIT
	fmt.Println("\n┌─ PERSONA FIT ──────────────────────────────────────────────┐")
	if len(syn.Personas) == 0 {
		fmt.Println("│ ⚠ No personas defined - cannot evaluate user fit")
		gaps = append(gaps, "Define at least one persona")
	} else {
		for _, p := range syn.Personas {
			maxScore += 10
			fmt.Printf("│\n│ [%s] %s\n", p.ID, p.Name)
			if len(p.Goals) > 0 {
				fmt.Printf("│   Goals: %s\n", strings.Join(p.Goals, ", "))
			}
			if len(p.Constraints) > 0 {
				fmt.Printf("│   Constraints: %s\n", strings.Join(p.Constraints, ", "))
			}

			// Generate persona-specific questions
			fmt.Println("│")
			fmt.Println("│   Questions to verify:")
			questions := getPersonaQuestions(p, syn.UXType)
			for _, q := range questions {
				fmt.Printf("│     ○ %s\n", q)
			}
			score += 5 // Partial credit for having persona
		}
	}
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// 2. FUNCTIONAL COMPLETENESS (Capabilities)
	fmt.Println("\n┌─ FUNCTIONAL COMPLETENESS ──────────────────────────────────┐")
	maxScore += len(syn.Capabilities) * 5
	for _, cap := range syn.Capabilities {
		covered := false
		for _, opt := range syn.Options {
			if opt.Coverage > 0 {
				covered = true
				break
			}
		}
		status := "○"
		if covered {
			status = "●"
			score += 5
		} else {
			gaps = append(gaps, fmt.Sprintf("Capability %s may not be covered", cap.ID))
		}
		if cap.Actor != "" {
			fmt.Printf("│ %s [%s] %s: %s\n", status, cap.ID, cap.Actor, cap.Goal)
		} else {
			fmt.Printf("│ %s [%s] %s\n", status, cap.ID, cap.Goal)
		}
	}
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// 3. EDGE CASE COVERAGE
	fmt.Println("\n┌─ EDGE CASE COVERAGE ──────────────────────────────────────┐")
	for _, ec := range syn.EdgeCases {
		maxScore += 5
		status := "○"
		coveredIn := ""
		if ec.Covered {
			status = "●"
			score += 5
			coveredIn = fmt.Sprintf(" → %s", ec.CoveredIn)
		} else {
			gaps = append(gaps, fmt.Sprintf("Edge case '%s' not covered", ec.ID))
		}
		fmt.Printf("│ %s %-12s %s%s\n", status, ec.ID, ec.Description, coveredIn)
	}
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// 4. FRICTION POINTS
	fmt.Println("\n┌─ FRICTION ANALYSIS ────────────────────────────────────────┐")
	if syn.Usability != nil && len(syn.Usability.FrictionPoints) > 0 {
		for _, fp := range syn.Usability.FrictionPoints {
			status := "○"
			if fp.Addressed {
				status = "●"
			}
			severityIcon := ""
			switch fp.Severity {
			case "high":
				severityIcon = "🔴"
			case "medium":
				severityIcon = "🟡"
			default:
				severityIcon = "🟢"
			}
			fmt.Printf("│ %s %s [%s] %s\n", status, severityIcon, fp.ID, fp.Description)
			if fp.Suggestion != "" {
				fmt.Printf("│     → %s\n", fp.Suggestion)
			}
		}
	} else {
		fmt.Println("│ No friction points identified yet.")
		fmt.Println("│")
		fmt.Println("│ Consider:")
		fmt.Println("│   • How many clicks/steps to complete primary action?")
		fmt.Println("│   • Any unnecessary confirmations?")
		fmt.Println("│   • Can user recover from mistakes easily?")
		fmt.Println("│   • Is important info visible without scrolling?")
	}
	fmt.Println("│")
	fmt.Printf("│ Add friction: pt discovery friction %s --add \"...\" --severity high\n", componentID)
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// 5. ENTRY POINTS (Cross-journey)
	fmt.Println("\n┌─ ENTRY POINTS ─────────────────────────────────────────────┐")
	fmt.Println("│ Where can users reach this component from?")
	fmt.Println("│")
	if syn.Usability != nil && len(syn.Usability.EntryPoints) > 0 {
		for _, ep := range syn.Usability.EntryPoints {
			status := "○"
			if ep.Supported {
				status = "●"
				maxScore += 5
				score += 5
			} else {
				maxScore += 5
				gaps = append(gaps, fmt.Sprintf("Entry from '%s' not designed", ep.From))
			}
			fmt.Printf("│ %s From: %s\n", status, ep.From)
		}
	} else {
		fmt.Println("│ ⚠ No entry points defined")
		fmt.Println("│ Consider: Where else might users want to access this?")
		gaps = append(gaps, "Define entry points")
	}
	fmt.Println("└────────────────────────────────────────────────────────────┘")

	// SCORE
	fmt.Println()
	percentage := 0
	if maxScore > 0 {
		percentage = (score * 100) / maxScore
	}
	fmt.Printf("Usability Score: %d/%d (%d%%)\n", score, maxScore, percentage)

	// GAPS
	if len(gaps) > 0 {
		fmt.Println("\n┌─ GAPS TO ADDRESS ──────────────────────────────────────────┐")
		for _, g := range gaps {
			fmt.Printf("│ ⚠ %s\n", g)
		}
		fmt.Println("└────────────────────────────────────────────────────────────┘")
	}

	// Update synthesis with review
	if syn.Usability == nil {
		syn.Usability = &pt.UsabilityReview{ComponentID: componentID}
	}
	syn.Usability.Score = percentage
	syn.Usability.Gaps = gaps
	syn.Usability.ReviewedAt = time.Now().Format(time.RFC3339)
	if err := saveSynthesis(syn); err != nil {
		return err
	}

	// Next steps
	fmt.Println()
	if percentage >= 70 {
		fmt.Println("✓ Usability review complete. Ready to synthesize.")
		fmt.Printf("  pt discovery synthesize %s\n", componentID)
	} else {
		fmt.Println("⚠ Address gaps above before synthesizing.")
	}

	return nil
}

func getPersonaQuestions(p pt.Persona, uxType string) []string {
	questions := []string{}

	// Goal-based questions
	for _, goal := range p.Goals {
		questions = append(questions, fmt.Sprintf("Can %s achieve: %s?", p.Name, goal))
	}

	// Constraint-based questions
	for _, c := range p.Constraints {
		switch strings.ToLower(c) {
		case "time pressure":
			questions = append(questions, "Can complete primary action in <3 steps?")
		case "low expertise", "new user":
			questions = append(questions, "Is terminology clear without jargon?")
			questions = append(questions, "Is there help/guidance available?")
		case "mobile", "on the go":
			questions = append(questions, "Works well on small screens?")
		}
	}

	// UX type specific
	switch uxType {
	case "web":
		questions = append(questions, "Critical info visible without scrolling?")
	case "cli":
		questions = append(questions, "Has --help with examples?")
	case "tui":
		questions = append(questions, "Keyboard shortcuts for power users?")
	}

	if len(questions) == 0 {
		questions = append(questions, "Does the UX fit this user's needs?")
	}

	return questions
}

func cmdDiscoveryFriction(args []string) error {
	fs := flag.NewFlagSet("discovery friction", flag.ExitOnError)
	add := fs.String("add", "", "Add friction point description")
	severity := fs.String("severity", "medium", "Severity: high|medium|low")
	location := fs.String("at", "", "Where in the flow")
	suggestion := fs.String("fix", "", "Suggested fix")
	address := fs.String("address", "", "Mark friction point as addressed")
	fs.Usage = func() {
		fmt.Println("Usage: pt discovery friction <component-id> [flags]")
		fmt.Println("\nIdentify and track UX friction points.")
		fmt.Println("\nFlags:")
		fmt.Println("  --add         Friction description")
		fmt.Println("  --severity    high|medium|low")
		fmt.Println("  --at          Location in the flow")
		fmt.Println("  --fix         Suggested fix")
		fmt.Println("  --address     Mark friction point ID as addressed")
		fmt.Println("\nExamples:")
		fmt.Println("  pt discovery friction order --add \"3 clicks to submit\" --severity high --fix \"Single-click submit\"")
		fmt.Println("  pt discovery friction order --address FP1")
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing component-id")
	}

	componentID := fs.Arg(0)
	syn, err := loadSynthesis(componentID)
	if err != nil {
		return err
	}
	if syn == nil {
		return fmt.Errorf("no discovery found for %s", componentID)
	}

	// Initialize usability if needed
	if syn.Usability == nil {
		syn.Usability = &pt.UsabilityReview{ComponentID: componentID}
	}

	// Mark as addressed
	if *address != "" {
		for i, fp := range syn.Usability.FrictionPoints {
			if fp.ID == *address {
				syn.Usability.FrictionPoints[i].Addressed = true
				if err := saveSynthesis(syn); err != nil {
					return err
				}
				fmt.Printf("✓ Friction point [%s] marked as addressed\n", *address)
				return nil
			}
		}
		return fmt.Errorf("friction point %s not found", *address)
	}

	// Add new friction point
	if *add != "" {
		id := fmt.Sprintf("FP%d", len(syn.Usability.FrictionPoints)+1)
		fp := pt.FrictionPoint{
			ID:          id,
			Description: *add,
			Severity:    *severity,
			Location:    *location,
			Suggestion:  *suggestion,
			Addressed:   false,
		}
		syn.Usability.FrictionPoints = append(syn.Usability.FrictionPoints, fp)
		if err := saveSynthesis(syn); err != nil {
			return err
		}

		severityIcon := "🟡"
		switch *severity {
		case "high":
			severityIcon = "🔴"
		case "low":
			severityIcon = "🟢"
		}
		fmt.Printf("Added friction [%s] %s: %s\n", id, severityIcon, *add)
		if *suggestion != "" {
			fmt.Printf("  Fix: %s\n", *suggestion)
		}
		return nil
	}

	// Show friction points
	fmt.Printf("Friction Points for %s:\n", componentID)
	fmt.Println(strings.Repeat("─", 50))

	if len(syn.Usability.FrictionPoints) == 0 {
		fmt.Println("\nNo friction points identified.")
		fmt.Println("\nConsider common friction sources:")
		fmt.Println("  • Too many steps to complete action")
		fmt.Println("  • Unclear error messages")
		fmt.Println("  • No way to undo/cancel")
		fmt.Println("  • Required info not visible")
		fmt.Println("  • Confusing terminology")
		return nil
	}

	unaddressed := 0
	for _, fp := range syn.Usability.FrictionPoints {
		status := "○"
		if fp.Addressed {
			status = "●"
		} else {
			unaddressed++
		}
		severityIcon := "🟡"
		switch fp.Severity {
		case "high":
			severityIcon = "🔴"
		case "low":
			severityIcon = "🟢"
		}
		fmt.Printf("\n%s %s [%s] %s\n", status, severityIcon, fp.ID, fp.Description)
		if fp.Location != "" {
			fmt.Printf("    At: %s\n", fp.Location)
		}
		if fp.Suggestion != "" {
			fmt.Printf("    Fix: %s\n", fp.Suggestion)
		}
	}

	fmt.Printf("\n%d friction points, %d unaddressed\n", len(syn.Usability.FrictionPoints), unaddressed)

	return nil
}
