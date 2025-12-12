package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

func cmdHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt history <id> [--json]") }
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
	events, err := client.History(ctx, id)
	if err != nil {
		return fmt.Errorf("history failed: %w", err)
	}
	if *jsonOut {
		return printJSON(events)
	}
	for _, ev := range events {
		fmt.Printf("%s %s", ev.At.Format(time.RFC3339), ev.Action)
		if strings.TrimSpace(ev.Actor) != "" {
			fmt.Printf(" @%s", ev.Actor)
		}
		if strings.TrimSpace(ev.Note) != "" {
			fmt.Printf(" note:%s", ev.Note)
		}
		fmt.Println()
	}
	return nil
}
