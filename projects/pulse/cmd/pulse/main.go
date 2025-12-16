package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	fs := flag.NewFlagSet("pulse", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print selected flows without running a browser")
	diffPath := fs.String("diff", "", "path to a git diff file (optional)")
	flowsDir := fs.String("flows", "flows", "directory containing micro-flows")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	if *dryRun {
		fmt.Printf("pulse dry-run: flows=%s diff=%s\n", *flowsDir, *diffPath)
		return
	}

	fmt.Fprintln(os.Stderr, "not implemented: use --dry-run (runner skeleton is a later task)")
	os.Exit(1)
}

