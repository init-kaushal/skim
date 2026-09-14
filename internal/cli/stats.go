package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/kaushal/skim/internal/metrics"
)

// sessionPrices is the per-million-token INPUT price of the model that would
// have absorbed the content skim kept out of the context. It exists only to
// turn a token saving into a dollar saving for the report — skim's behaviour
// never branches on the session model, and nothing here is used at
// interception time.
//
// The session model cannot be detected: the PreToolUse hook payload does not
// carry it. So it is an explicit assumption, surfaced in the output and
// overridable with --session-model, rather than a silent guess.
//
// The worker side needs no table: its real cost comes from the CLI's own
// total_cost_usd, recorded per call.
var sessionPrices = map[string]float64{
	"opus-5":    5.00,
	"sonnet-5":  3.00,
	"haiku-4.5": 1.00,
}

const defaultSessionModel = "opus-5"

// cacheReadMultiplier is what a re-sent, cached prefix bills relative to fresh
// input. It is what makes the compounding real: content skim keeps out of the
// context would otherwise ride along on every later turn at this rate.
const cacheReadMultiplier = 0.10

// Stats prints the interception ledger. It answers one question — did this save
// money — and is careful about the fact that tokens and dollars are different
// units. An earlier version printed "net = tokens saved - worker tokens", which
// subtracted a figure billed at one model's rates from a figure billed at
// another's, and summed four differently-priced token classes to get there.
func Stats(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(w)
	model := fs.String("session-model", defaultSessionModel,
		"model that would have read the content, for pricing the saving "+
			"(opus-5, sonnet-5, haiku-4.5)")
	turns := fs.Int("turns", 10,
		"how many further turns the content would have ridden along for")
	if err := fs.Parse(args); err != nil {
		return err
	}
	price, ok := sessionPrices[*model]
	if !ok {
		names := make([]string, 0, len(sessionPrices))
		for k := range sessionPrices {
			names = append(names, k)
		}
		sort.Strings(names)
		return fmt.Errorf("unknown --session-model %q (known: %v)", *model, names)
	}

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

	fmt.Fprintln(w, "skim — interception ledger")
	fmt.Fprintln(w)

	// Sorted, not map order: an unsorted range made this output reshuffle
	// between invocations for no reason.
	tools := make([]string, 0, len(s.Interceptions))
	for t := range s.Interceptions {
		tools = append(tools, t)
	}
	sort.Strings(tools)

	fmt.Fprintf(w, "  %-6s %12s %14s %12s\n", "tool", "intercepts", "tokens saved", "worker $")
	for _, t := range tools {
		fmt.Fprintf(w, "  %-6s %12d %14d %12.4f\n",
			t, s.Interceptions[t], s.SavedEstByTool[t], s.CostUSDByTool[t])
	}
	fmt.Fprintln(w)

	// Bash redirects but never digests, so a Bash line showing zero saved is
	// correct rather than broken — the saving lands on the Run line.
	if s.Interceptions["Bash"] > 0 && s.SavedEstByTool["Bash"] == 0 {
		fmt.Fprintln(w, "  note    Bash only redirects; its savings and cost appear on the Run line.")
	}

	if s.CacheHits+s.CacheMisses > 0 {
		total := s.CacheHits + s.CacheMisses
		fmt.Fprintf(w, "  cache   %d hit / %d miss (%.0f%% hit rate, Read only)\n",
			s.CacheHits, s.CacheMisses, 100*float64(s.CacheHits)/float64(total))
	} else {
		fmt.Fprintln(w, "  cache   no cache-eligible interceptions yet")
	}
	fmt.Fprintln(w)

	// --- the actual question, in dollars ---
	savedFirst := float64(s.TotalSavedEst) * price / 1e6
	savedWithResend := savedFirst * (1 + float64(*turns)*cacheReadMultiplier)
	spent := s.TotalWorkerCostUSD

	fmt.Fprintf(w, "  assuming a %s session (%.2f $/Mtok in), content surviving %d more turns:\n",
		*model, price, *turns)
	fmt.Fprintf(w, "    saved    ~%d tokens  =  $%.4f on first read, $%.4f with re-sends\n",
		s.TotalSavedEst, savedFirst, savedWithResend)
	fmt.Fprintf(w, "    spent    $%.4f actually billed by the worker (%d of %d calls reported cost)\n",
		spent, s.CostedCalls, sum(s.Interceptions))
	net := savedWithResend - spent
	verdict := "ahead"
	if net < 0 {
		verdict = "BEHIND"
	}
	fmt.Fprintf(w, "    net      $%+.4f  — %s\n", net, verdict)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  worker tokens: %d (diagnostic only — input, output, cache-write and\n", s.TotalWorkerTokens)
	fmt.Fprintln(w, "  cache-read bill at 1x, 5x, 2x and 0.1x, so this sum is a size, not a cost)")

	if s.CostedCalls == 0 && sum(s.Interceptions) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  No call reported a cost. Entries recorded before cost tracking existed")
		fmt.Fprintln(w, "  carry none; run `skim config --clear-cache` and gather fresh data, or")
		fmt.Fprintln(w, "  use `make bench FILE=<path>` for a single measured comparison.")
	}
	return nil
}

func sum(m map[string]int) int {
	t := 0
	for _, v := range m {
		t += v
	}
	return t
}
