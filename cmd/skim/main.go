package main

import (
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

const usage = `usage: skim <command> [args]

hook handlers (invoked by Claude Code, read JSON on stdin):
  read-hook     intercept oversized Read calls
  grep-hook     intercept wide Grep calls
  bash-hook     intercept noisy Bash calls

commands:
  run -- <cmd>  execute a command, capture full output, print a digest
  cat <path>    print a whole file with no interception
  doctor        environment and config diagnostics
  stats         cumulative interception savings
  config        show or change configuration
  version       print version
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "read-hook", "grep-hook", "bash-hook":
		// Real wiring lands in Task 14. For now: allow (no-op), exit 0.
		return 0
	case "run", "cat", "doctor", "stats", "config":
		fmt.Fprintf(stderr, "skim %s: not implemented yet\n", args[0])
		return 1
	case "version":
		fmt.Fprintln(stdout, "skim (dev)")
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}
