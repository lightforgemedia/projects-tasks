package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agenttools/internal/agenttools"

	// Blank imports self-register tools.
	_ "agenttools/tools/echo"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		usage()
		return errors.New("missing command")
	}
	cmd := args[1]
	switch cmd {
	case "list":
		return cmdList()
	case "call":
		return cmdCall(args[2:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Print(`agent-tools CLI

Commands:
  list                           List registered tools
  call <tool> [json]             Call a tool with JSON input (default: {})
`)
}

func cmdList() error {
	for _, t := range agenttools.Default.List() {
		fmt.Printf("%s - %s\n", t.Name(), strings.TrimSpace(t.Description()))
	}
	return nil
}

func cmdCall(args []string) error {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	fs.Usage = func() { fmt.Println("Usage: agent-tools call <tool> [json]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("missing tool name")
	}
	name := fs.Arg(0)
	input := []byte(`{}`)
	if fs.NArg() >= 2 {
		input = []byte(fs.Arg(1))
	}
	ctx := context.Background()
	out, err := agenttools.Default.Call(ctx, name, input)
	if err != nil {
		return err
	}
	var pretty any
	_ = json.Unmarshal(out, &pretty)
	buf, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(buf))
	return nil
}
