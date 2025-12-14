package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"projects-tasks/pkg/pt"
)

// journeysDir returns the path to the journeys directory
func journeysDir() string {
	return filepath.Join(".pt", "journeys")
}

// journeyFile returns the path to a specific journey file
func journeyFile(id string) string {
	return filepath.Join(journeysDir(), id+".json")
}

// loadJourneys loads all journeys from disk
func loadJourneys() ([]pt.UserJourney, error) {
	dir := journeysDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var journeys []pt.UserJourney
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var j pt.UserJourney
		if err := json.Unmarshal(data, &j); err != nil {
			continue
		}
		journeys = append(journeys, j)
	}
	return journeys, nil
}

// loadJourney loads a single journey
func loadJourney(id string) (*pt.UserJourney, error) {
	data, err := os.ReadFile(journeyFile(id))
	if err != nil {
		return nil, err
	}
	var j pt.UserJourney
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// saveJourney persists a journey to disk
func saveJourney(j *pt.UserJourney) error {
	if err := os.MkdirAll(journeysDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(journeyFile(j.ID), data, 0644)
}

// cmdJourney handles the journey command
func cmdJourney(args []string) error {
	if len(args) == 0 {
		return cmdJourneyList(nil)
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "list":
		return cmdJourneyList(subArgs)
	case "add":
		return cmdJourneyAdd(subArgs)
	case "step":
		return cmdJourneyStep(subArgs)
	case "trace":
		return cmdJourneyTrace(subArgs)
	case "coverage":
		return cmdJourneyCoverage(subArgs)
	default:
		return fmt.Errorf("unknown journey subcommand: %s\nUsage: pt journey [list|add|step|trace|coverage]", subCmd)
	}
}

// cmdJourneyList lists all journeys
func cmdJourneyList(args []string) error {
	journeys, err := loadJourneys()
	if err != nil {
		return err
	}

	if len(journeys) == 0 {
		fmt.Println("No journeys defined.")
		fmt.Println("Create one with: pt journey add <name> --goal 'What user wants to achieve'")
		return nil
	}

	fmt.Println("User Journeys:")
	fmt.Println(strings.Repeat("═", 60))
	for _, j := range journeys {
		priority := ""
		if j.Priority > 0 {
			priority = fmt.Sprintf(" [P%d]", j.Priority)
		}
		fmt.Printf("[%s]%s %s\n", j.ID, priority, j.Name)
		fmt.Printf("     Goal: %s\n", j.Goal)
		fmt.Printf("     Steps: %d\n", len(j.Steps))
	}
	return nil
}

// cmdJourneyAdd creates a new journey
func cmdJourneyAdd(args []string) error {
	fs := flag.NewFlagSet("journey add", flag.ExitOnError)
	goal := fs.String("goal", "", "What the user wants to achieve")
	persona := fs.String("persona", "", "Who takes this journey")
	trigger := fs.String("trigger", "", "What initiates this journey")
	priority := fs.Int("priority", 0, "Priority: 1=critical, 2=important, 3=nice-to-have")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt journey add <name> --goal 'description' [--persona '...'] [--priority 1]")
	}

	name := fs.Arg(0)

	// Generate ID
	journeys, _ := loadJourneys()
	id := fmt.Sprintf("J%d", len(journeys)+1)

	j := &pt.UserJourney{
		ID:       id,
		Name:     name,
		Goal:     *goal,
		Persona:  *persona,
		Trigger:  *trigger,
		Priority: *priority,
		Steps:    []pt.JourneyStep{},
	}

	if err := saveJourney(j); err != nil {
		return err
	}

	fmt.Printf("✓ Created journey [%s]: %s\n", id, name)
	fmt.Println("\nAdd steps with:")
	fmt.Printf("  pt journey step %s --action 'User does X' --component chain\n", id)
	return nil
}

// cmdJourneyStep adds a step to a journey
func cmdJourneyStep(args []string) error {
	fs := flag.NewFlagSet("journey step", flag.ExitOnError)
	action := fs.String("action", "", "What the user does")
	component := fs.String("component", "", "Component ID from system map")
	expectation := fs.String("expect", "", "What user expects to see")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt journey step <journey-id> --action 'User does X' --component <id>")
	}

	journeyID := fs.Arg(0)

	j, err := loadJourney(journeyID)
	if err != nil {
		return fmt.Errorf("journey %q not found", journeyID)
	}

	// Validate component exists in system map
	sm, _ := loadSystemMap()
	if sm != nil && *component != "" {
		found := false
		for _, c := range sm.Components {
			if c.ID == *component {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("⚠ Warning: component %q not found in system map\n", *component)
		}
	}

	step := pt.JourneyStep{
		Order:       len(j.Steps) + 1,
		Action:      *action,
		Component:   *component,
		Expectation: *expectation,
	}

	j.Steps = append(j.Steps, step)

	if err := saveJourney(j); err != nil {
		return err
	}

	fmt.Printf("✓ Added step %d to journey [%s]\n", step.Order, journeyID)
	return nil
}

// cmdJourneyTrace walks through a journey step-by-step
func cmdJourneyTrace(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: pt journey trace <journey-id>")
	}

	journeyID := args[0]

	j, err := loadJourney(journeyID)
	if err != nil {
		return fmt.Errorf("journey %q not found", journeyID)
	}

	fmt.Printf("Journey: %s\n", j.Name)
	fmt.Printf("Goal: %s\n", j.Goal)
	if j.Persona != "" {
		fmt.Printf("Persona: %s\n", j.Persona)
	}
	fmt.Println(strings.Repeat("─", 50))

	// Load system map for component descriptions
	sm, _ := loadSystemMap()
	compDesc := make(map[string]string)
	if sm != nil {
		for _, c := range sm.Components {
			compDesc[c.ID] = c.Description
		}
	}

	for _, step := range j.Steps {
		compInfo := step.Component
		if desc, ok := compDesc[step.Component]; ok && desc != "" {
			compInfo = fmt.Sprintf("%s (%s)", step.Component, desc)
		}

		fmt.Printf("\n[Step %d] %s\n", step.Order, step.Action)
		fmt.Printf("  Component: %s\n", compInfo)
		if step.Expectation != "" {
			fmt.Printf("  Expects: %s\n", step.Expectation)
		}
	}

	if j.Outcome != "" {
		fmt.Printf("\n✓ Outcome: %s\n", j.Outcome)
	}
	return nil
}

// cmdJourneyCoverage shows which components are covered by journeys
func cmdJourneyCoverage(args []string) error {
	sm, _ := loadSystemMap()
	if sm == nil {
		return fmt.Errorf("no system map found; create with 'pt sysmap init'")
	}

	journeys, _ := loadJourneys()

	// Build coverage map: component -> []journeys
	coverage := make(map[string][]string)
	stepCount := make(map[string]int)

	for _, j := range journeys {
		for _, step := range j.Steps {
			if step.Component != "" {
				coverage[step.Component] = append(coverage[step.Component], j.ID)
				stepCount[step.Component]++
			}
		}
	}

	fmt.Println("Component Coverage:")
	fmt.Println(strings.Repeat("═", 70))
	fmt.Printf("%-20s │ %-15s │ %-5s │ Coverage\n", "Component", "Journeys", "Steps")
	fmt.Println(strings.Repeat("─", 70))

	uncovered := []string{}
	for _, c := range sm.Components {
		jList := coverage[c.ID]
		steps := stepCount[c.ID]

		journeyStr := "-"
		if len(jList) > 0 {
			journeyStr = strings.Join(jList, ",")
			if len(journeyStr) > 15 {
				journeyStr = journeyStr[:12] + "..."
			}
		} else {
			uncovered = append(uncovered, c.ID)
		}

		fmt.Printf("%-20s │ %-15s │ %5d │", c.ID, journeyStr, steps)

		// Visual bar
		bar := ""
		if steps > 0 {
			barLen := steps
			if barLen > 10 {
				barLen = 10
			}
			bar = strings.Repeat("█", barLen)
		}
		fmt.Printf(" %s\n", bar)
	}

	fmt.Println(strings.Repeat("─", 70))

	if len(uncovered) > 0 {
		fmt.Printf("\n⚠ Components with NO journeys: %s\n", strings.Join(uncovered, ", "))
		fmt.Println("  Consider: Are these necessary? Add journeys or remove components.")
	} else {
		fmt.Println("\n✓ All components have journey coverage")
	}

	return nil
}

// nextJourneyID generates the next journey ID
func nextJourneyID() string {
	journeys, _ := loadJourneys()
	return fmt.Sprintf("J%d", len(journeys)+1)
}
