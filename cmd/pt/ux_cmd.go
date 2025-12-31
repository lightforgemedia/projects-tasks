package main

import (
	"errors"
	"fmt"
)

func cmdUX(args []string) error {
	if len(args) == 0 {
		return uxUsage()
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "preflight":
		return cmdUXPreflight(subArgs)
	case "cases":
		return cmdUXCases(subArgs)
	case "explore":
		return cmdUXExplore(subArgs)
	case "select":
		return cmdUXSelect(subArgs)
	case "status":
		return cmdUXStatus(subArgs)
	case "mockup":
		return cmdUXMockup(subArgs)
	case "compare":
		return cmdUXCompare(subArgs)
	case "upgrade":
		return cmdUXUpgrade(subArgs)
	case "drill":
		return cmdUXDrill(subArgs)
	case "breakout":
		return cmdUXBreakout(subArgs)
	case "cover":
		return cmdUXCover(subArgs)
	case "-h", "--help", "help":
		return uxUsage()
	default:
		_ = uxUsage()
		return fmt.Errorf("unknown ux subcommand %q", subcmd)
	}
}

func uxUsage() error {
	fmt.Println("Usage: pt ux <subcmd> [args]")
	fmt.Println("")
	fmt.Println("Subcommands:")
	fmt.Println("  preflight <id>   Write UX preflight packet in .pt/reviews/ (canonical + archived)")
	fmt.Println("  cases <id>       Define use cases (alias for pt ux-cases)")
	fmt.Println("  explore <id>     Generate design options (alias for pt ux-explore)")
	fmt.Println("  select <id> ...  Choose approach (alias for pt ux-select)")
	fmt.Println("")
	return errors.New("")
}
