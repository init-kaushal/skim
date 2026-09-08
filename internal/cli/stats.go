package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/kaushal/skim/internal/metrics"
)

// Stats prints cumulative interception statistics from the metrics JSONL log:
// per-tool interception counts, cache hit/miss with hit rate, and an estimate
// of the tokens kept out of the main context.
func Stats(w io.Writer) error {
	f, err := os.Open(metrics.Path())
	if os.IsNotExist(err) {
		fmt.Fprintln(w, "skim: no interceptions recorded yet")
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	s, err := metrics.Aggregate(f)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "skim — cumulative interception stats")
	for tool, n := range s.Interceptions {
		fmt.Fprintf(w, "  %-6s %d interceptions\n", tool, n)
	}
	total := s.CacheHits + s.CacheMisses
	rate := 0.0
	if total > 0 {
		rate = 100 * float64(s.CacheHits) / float64(total)
	}
	fmt.Fprintf(w, "  cache   %d hit / %d miss (%.0f%% hit rate)\n", s.CacheHits, s.CacheMisses, rate)
	fmt.Fprintf(w, "  saved   ~%d tokens kept out of the main context (estimate)\n", s.TotalSavedEst)
	return nil
}
