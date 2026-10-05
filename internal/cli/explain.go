package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/kaushal/skim/internal/observe"
)

// Explain prints recent routing decisions from the decisions log.
// It answers "why was this operation optimized?" for every intercepted
// (or explicitly passed) operation.
func Explain(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(w)
	last := fs.Int("last", 10, "number of recent decisions to show")
	tool := fs.String("tool", "", "filter by tool name (Read, Grep, Run)")
	target := fs.String("file", "", "filter by target path substring")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, err := observe.Read(observe.Filter{
		Tool:   *tool,
		Target: *target,
		Last:   *last,
	})
	if err != nil {
		return fmt.Errorf("explain: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(w, "No decisions recorded yet.")
		fmt.Fprintln(w, "Run some Claude Code operations, then try again.")
		return nil
	}

	fmt.Fprintf(w, "Last %d decisions:\n\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(w, "  %s\n", e.TS)
		observe.Format(w, e)
	}
	return nil
}
