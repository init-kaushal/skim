// Package observe records per-operation routing decisions to a local log,
// enabling `skim explain` to answer "why was this optimized?" for every
// intercepted (or explicitly passed-through) operation.
package observe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/paths"
)

// Entry records one routing decision. It is written for every hook
// invocation — intercepted or not — so explain can show the full picture.
//
// Fields prefixed Est* are predictions made before the worker runs.
// Fields prefixed Actual* are filled in after the worker returns (or left zero
// for cache hits, dry-run, or passthrough). skim explain clearly labels which
// are predictions vs. realized figures.
type Entry struct {
	TS               string        `json:"ts"`
	Tool             string        `json:"tool"`   // "Read", "Grep", "Run"
	Target           string        `json:"target"` // file path, grep pattern, or command
	Strategy         cost.Strategy `json:"strategy"`
	DryRun           bool          `json:"dry_run,omitempty"`
	EstOrigCostUSD   float64       `json:"est_orig_cost_usd,omitempty"`
	EstWorkerCostUSD float64       `json:"est_worker_cost_usd,omitempty"`
	// EstNetSavingsUSD is the predicted net saving (gross savings − worker cost).
	// Positive means the interception is predicted to be profitable.
	EstNetSavingsUSD float64 `json:"est_net_savings_usd,omitempty"`
	// PredictedRatio is the output_tokens/input_tokens estimate used for the
	// cost calculation. Comes from calibration or DefaultOutputRatio.
	PredictedRatio  float64 `json:"predicted_ratio,omitempty"`
	CompressorKind  string  `json:"compressor_kind,omitempty"`
	CompressRatio   float64 `json:"compress_ratio,omitempty"`
	// ActualWorkerCostUSD is the actual billed cost of the worker call.
	// Zero for cache hits, dry-run, deterministic, or passthrough.
	ActualWorkerCostUSD float64 `json:"actual_worker_cost_usd,omitempty"`
	// ActualOutputRatio is the observed output_tokens/input_tokens from the
	// worker call. Recorded for calibration feedback. Zero when not applicable.
	ActualOutputRatio float64 `json:"actual_output_ratio,omitempty"`
	// ReuseCount is how many times this target was seen in the current session
	// (1 = first time, 2 = seen once before, etc.). Zero if not tracked.
	ReuseCount int    `json:"reuse_count,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Path returns the decisions log path.
func Path() string { return filepath.Join(paths.Home(), "decisions.jsonl") }

// Record appends e to the decisions log. Errors are silently discarded — the
// observe path must never block a tool call.
func Record(e Entry) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(Path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\n'))
}

// Filter controls which decisions are returned by Read.
type Filter struct {
	Tool   string // empty = all tools
	Target string // substring match; empty = all targets
	Last   int    // 0 = default (25)
}

// Read returns the last n decisions from the decisions log, newest last,
// applying f. It returns at most max(f.Last, 25) entries.
func Read(f Filter) ([]Entry, error) {
	limit := f.Last
	if limit <= 0 {
		limit = 25
	}

	file, err := os.Open(Path())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var all []Entry
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 32*1024), 512*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		if f.Tool != "" && !strings.EqualFold(e.Tool, f.Tool) {
			continue
		}
		if f.Target != "" && !strings.Contains(e.Target, f.Target) {
			continue
		}
		all = append(all, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	if len(all) <= limit {
		return all, nil
	}
	return all[len(all)-limit:], nil
}

// Format renders a single entry in human-readable form for `skim explain`.
// Estimates are clearly labeled as such; actual figures are labeled "realized".
func Format(w io.Writer, e Entry) {
	fmt.Fprintf(w, "Operation:  %s %s\n", e.Tool, e.Target)
	fmt.Fprintf(w, "Strategy:   %s\n", e.Strategy)
	if e.CompressorKind != "" {
		fmt.Fprintf(w, "Compressor: %s (ratio %.2f)\n", e.CompressorKind, e.CompressRatio)
	}
	if e.ReuseCount > 0 {
		fmt.Fprintf(w, "Reuse:      seen %d time(s) this session\n", e.ReuseCount)
	}
	if e.EstOrigCostUSD > 0 || e.EstWorkerCostUSD > 0 {
		fmt.Fprintf(w, "Est direct cost:  $%.4f  (estimated)\n", e.EstOrigCostUSD)
		if e.PredictedRatio > 0 {
			fmt.Fprintf(w, "Est compression:  %.0f%% output ratio  (estimated, from calibration)\n", e.PredictedRatio*100)
		}
		fmt.Fprintf(w, "Est worker cost:  $%.4f  (estimated)\n", e.EstWorkerCostUSD)
		fmt.Fprintf(w, "Est net savings:  $%.4f  (estimated)\n", e.EstNetSavingsUSD)
	}
	if e.ActualWorkerCostUSD > 0 {
		fmt.Fprintf(w, "Actual cost:      $%.4f  (realized)\n", e.ActualWorkerCostUSD)
		if e.ActualOutputRatio > 0 {
			fmt.Fprintf(w, "Actual compress:  %.0f%% output ratio  (realized)\n", e.ActualOutputRatio*100)
		}
		if e.EstWorkerCostUSD > 0 {
			delta := e.EstWorkerCostUSD - e.ActualWorkerCostUSD
			label := "over-estimated"
			if delta < 0 {
				label = "under-estimated"
				delta = -delta
			}
			fmt.Fprintf(w, "Prediction gap:   $%.4f %s\n", delta, label)
		}
	}
	if e.Reason != "" {
		fmt.Fprintf(w, "Reason:     %s\n", e.Reason)
	}
	if e.DryRun {
		fmt.Fprintf(w, "[dry-run — decision logged, tool call not intercepted]\n")
	}
	fmt.Fprintln(w)
}
