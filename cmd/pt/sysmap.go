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

// sysmapFile returns the path to the system map file
func sysmapFile() string {
	return filepath.Join(".pt", "sysmap.json")
}

// loadSystemMap loads the system map from disk
func loadSystemMap() (*pt.SystemMap, error) {
	path := sysmapFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No system map yet
		}
		return nil, err
	}
	var sm pt.SystemMap
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, err
	}
	return &sm, nil
}

// saveSystemMap persists the system map to disk
func saveSystemMap(sm *pt.SystemMap) error {
	if err := os.MkdirAll(".pt", 0755); err != nil {
		return err
	}
	sm.UpdatedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sysmapFile(), data, 0644)
}

// cmdSysmap handles the sysmap command
func cmdSysmap(args []string) error {
	if len(args) == 0 {
		return cmdSysmapShow(nil)
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "init":
		return cmdSysmapInit(subArgs)
	case "add":
		return cmdSysmapAdd(subArgs)
	case "link":
		return cmdSysmapLink(subArgs)
	case "show":
		return cmdSysmapShow(subArgs)
	case "verify":
		return cmdSysmapVerify(subArgs)
	default:
		return fmt.Errorf("unknown sysmap subcommand: %s\nUsage: pt sysmap [init|add|link|show|verify]", subCmd)
	}
}

// cmdSysmapInit initializes a new system map
func cmdSysmapInit(args []string) error {
	fs := flag.NewFlagSet("sysmap init", flag.ExitOnError)
	desc := fs.String("desc", "", "System description")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: pt sysmap init <name> [--desc 'description']")
	}

	name := fs.Arg(0)

	// Check if already exists
	existing, _ := loadSystemMap()
	if existing != nil {
		fmt.Printf("System map already exists: %s (v%s)\n", existing.Name, existing.Version)
		fmt.Printf("Use 'pt sysmap add' to add components.\n")
		return nil
	}

	sm := &pt.SystemMap{
		ID:          fmt.Sprintf("sysmap-%d", time.Now().Unix()),
		Name:        name,
		Description: *desc,
		Version:     "1.0",
		Components:  []pt.Component{},
		Edges:       []pt.Edge{},
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	if err := saveSystemMap(sm); err != nil {
		return err
	}

	fmt.Printf("✓ System map initialized: %s\n", name)
	fmt.Println("\nNext steps:")
	fmt.Println("  pt sysmap add <id> <type>  # Add components (types: command|screen|service|store)")
	fmt.Println("  pt sysmap link <from> <to> # Connect components")
	fmt.Println("  pt sysmap show             # Visualize the map")
	return nil
}

// cmdSysmapAdd adds a component to the system map
func cmdSysmapAdd(args []string) error {
	fs := flag.NewFlagSet("sysmap add", flag.ExitOnError)
	name := fs.String("name", "", "Human-readable name")
	desc := fs.String("desc", "", "Component description")
	category := fs.String("cat", "", "Category: view|action|data|util")
	nouns := fs.String("nouns", "", "Domain nouns (comma-separated)")
	verbs := fs.String("verbs", "", "Actions performed (comma-separated)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: pt sysmap add <id> <type> [--name 'Name'] [--desc 'Description']\n  types: command|screen|service|store|api")
	}

	id := fs.Arg(0)
	compType := fs.Arg(1)

	// Validate type
	validTypes := map[string]bool{"command": true, "screen": true, "service": true, "store": true, "api": true}
	if !validTypes[compType] {
		return fmt.Errorf("invalid type %q; must be one of: command, screen, service, store, api", compType)
	}

	sm, err := loadSystemMap()
	if err != nil {
		return err
	}
	if sm == nil {
		return fmt.Errorf("no system map found; run 'pt sysmap init <name>' first")
	}

	// Check for duplicate
	for _, c := range sm.Components {
		if c.ID == id {
			return fmt.Errorf("component %q already exists", id)
		}
	}

	displayName := *name
	if displayName == "" {
		displayName = strings.Title(strings.ReplaceAll(id, "-", " "))
	}

	comp := pt.Component{
		ID:          id,
		Name:        displayName,
		Type:        compType,
		Category:    *category,
		Description: *desc,
	}

	if *nouns != "" {
		comp.Nouns = strings.Split(*nouns, ",")
	}
	if *verbs != "" {
		comp.Verbs = strings.Split(*verbs, ",")
	}

	sm.Components = append(sm.Components, comp)

	if err := saveSystemMap(sm); err != nil {
		return err
	}

	fmt.Printf("✓ Added component [%s] (%s)\n", id, compType)
	return nil
}

// cmdSysmapLink creates an edge between components
func cmdSysmapLink(args []string) error {
	fs := flag.NewFlagSet("sysmap link", flag.ExitOnError)
	relation := fs.String("rel", "calls", "Relationship: calls|uses|triggers|provides|requires")
	label := fs.String("label", "", "Edge label (e.g., data passed)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: pt sysmap link <from> <to> [--rel calls|uses|triggers|provides|requires]")
	}

	from := fs.Arg(0)
	to := fs.Arg(1)

	sm, err := loadSystemMap()
	if err != nil {
		return err
	}
	if sm == nil {
		return fmt.Errorf("no system map found; run 'pt sysmap init <name>' first")
	}

	// Validate components exist
	var fromExists, toExists bool
	for _, c := range sm.Components {
		if c.ID == from {
			fromExists = true
		}
		if c.ID == to {
			toExists = true
		}
	}
	if !fromExists {
		return fmt.Errorf("component %q not found", from)
	}
	if !toExists {
		return fmt.Errorf("component %q not found", to)
	}

	// Check for duplicate edge
	for _, e := range sm.Edges {
		if e.From == from && e.To == to {
			return fmt.Errorf("edge %s -> %s already exists", from, to)
		}
	}

	edge := pt.Edge{
		From:     from,
		To:       to,
		Relation: *relation,
		Label:    *label,
	}

	sm.Edges = append(sm.Edges, edge)

	if err := saveSystemMap(sm); err != nil {
		return err
	}

	fmt.Printf("✓ Linked: %s --%s--> %s\n", from, *relation, to)
	return nil
}

// cmdSysmapShow displays the system map
func cmdSysmapShow(args []string) error {
	sm, err := loadSystemMap()
	if err != nil {
		return err
	}
	if sm == nil {
		fmt.Println("No system map found.")
		fmt.Println("Create one with: pt sysmap init <name>")
		return nil
	}

	fmt.Printf("System: %s (v%s)\n", sm.Name, sm.Version)
	if sm.Description != "" {
		fmt.Printf("%s\n", sm.Description)
	}
	fmt.Println(strings.Repeat("═", 60))

	// Group by type
	byType := make(map[string][]pt.Component)
	for _, c := range sm.Components {
		byType[c.Type] = append(byType[c.Type], c)
	}

	// Display order
	typeOrder := []string{"command", "screen", "service", "api", "store"}
	for _, t := range typeOrder {
		comps := byType[t]
		if len(comps) == 0 {
			continue
		}
		fmt.Printf("\n%s:\n", strings.ToUpper(t)+"S")
		for _, c := range comps {
			desc := c.Description
			if desc == "" {
				desc = c.Name
			}
			if len(desc) > 45 {
				desc = desc[:42] + "..."
			}
			fmt.Printf("  [%s] %s\n", c.ID, desc)
		}
	}

	// Display edges
	if len(sm.Edges) > 0 {
		fmt.Printf("\nDAG:\n")
		for _, e := range sm.Edges {
			label := ""
			if e.Label != "" {
				label = fmt.Sprintf(" (%s)", e.Label)
			}
			fmt.Printf("  %s --%s--> %s%s\n", e.From, e.Relation, e.To, label)
		}
	}

	fmt.Println()
	return nil
}

// cmdSysmapVerify checks the system map for issues
func cmdSysmapVerify(args []string) error {
	sm, err := loadSystemMap()
	if err != nil {
		return err
	}
	if sm == nil {
		return fmt.Errorf("no system map found")
	}

	issues := []string{}

	// Build component set
	compSet := make(map[string]bool)
	for _, c := range sm.Components {
		compSet[c.ID] = true
	}

	// Check edges reference valid components
	for _, e := range sm.Edges {
		if !compSet[e.From] {
			issues = append(issues, fmt.Sprintf("Edge references unknown component: %s", e.From))
		}
		if !compSet[e.To] {
			issues = append(issues, fmt.Sprintf("Edge references unknown component: %s", e.To))
		}
	}

	// Find orphan components (no edges in or out)
	hasEdge := make(map[string]bool)
	for _, e := range sm.Edges {
		hasEdge[e.From] = true
		hasEdge[e.To] = true
	}
	for _, c := range sm.Components {
		if !hasEdge[c.ID] {
			issues = append(issues, fmt.Sprintf("Orphan component (no edges): %s", c.ID))
		}
	}

	// Check for cycles (simple DFS)
	// Build adjacency list
	adj := make(map[string][]string)
	for _, e := range sm.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		for _, neighbor := range adj[node] {
			if !visited[neighbor] {
				if hasCycle(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				return true
			}
		}
		recStack[node] = false
		return false
	}

	for _, c := range sm.Components {
		if !visited[c.ID] {
			if hasCycle(c.ID) {
				issues = append(issues, "Cycle detected in component DAG")
				break
			}
		}
	}

	// Report
	if len(issues) == 0 {
		fmt.Printf("✓ System map valid: %d components, %d edges\n", len(sm.Components), len(sm.Edges))
		return nil
	}

	fmt.Printf("Found %d issues:\n", len(issues))
	for _, issue := range issues {
		fmt.Printf("  ⚠ %s\n", issue)
	}
	return nil
}

// promptLine reads a line from stdin
func promptLine(prompt string) string {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}
