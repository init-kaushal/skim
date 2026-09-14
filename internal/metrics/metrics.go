package metrics

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kaushal/skim/internal/paths"
)

// Entry is one line of the interception log. Every intercepting path writes one
// — including `skim run`, which for a long time wrote none at all and so made
// the whole Bash side of the ledger invisible.
type Entry struct {
	TS              string `json:"ts"`
	Tool            string `json:"tool"`
	OrigTokensEst   int    `json:"orig_tokens_est"`
	DigestTokensEst int    `json:"digest_tokens_est"`
	SavedEst        int    `json:"saved_est"`

	// WorkerTokens is the flat sum of every billed token variant. Kept as a
	// diagnostic for sizing a call; it is NOT a cost and must not be subtracted
	// from SavedEst — the variants below bill at different rates.
	WorkerTokens int `json:"worker_tokens"`

	WorkerInputTokens      int `json:"worker_input_tokens,omitempty"`
	WorkerOutputTokens     int `json:"worker_output_tokens,omitempty"`
	WorkerCacheWriteTokens int `json:"worker_cache_write_tokens,omitempty"`
	WorkerCacheReadTokens  int `json:"worker_cache_read_tokens,omitempty"`

	// WorkerCostUSD is what the nested call actually billed, as reported by the
	// CLI itself. This is the only figure that can be weighed against a saving.
	WorkerCostUSD float64 `json:"worker_cost_usd,omitempty"`

	// CacheHit is three-state on purpose. nil means this path has no digest
	// cache at all, which is true of Grep and Bash — counting them as misses
	// made the reported hit rate meaningless (one Read plus three Bash
	// interceptions reported "0 hit / 4 miss (0% hit rate)").
	CacheHit *bool `json:"cache_hit,omitempty"`
}

// Hit and Miss build the tri-state CacheHit for a path that does consult the
// cache. Leaving it nil is the correct choice for a path that does not.
func Hit() *bool  { t := true; return &t }
func Miss() *bool { f := false; return &f }

func Path() string { return paths.Metrics() }

// EstimateTokens approximates a token count from a byte count at 4 bytes per
// token. It is a rough scale indicator, not an accounting figure — real
// tokenization runs closer to 3 bytes/token on prose and markdown.
func EstimateTokens(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func Record(e Entry) error {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(Path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

type Stats struct {
	Interceptions map[string]int
	// CacheHits/CacheMisses count only entries from a path that consults the
	// digest cache, so the rate describes what it claims to.
	CacheHits   int
	CacheMisses int

	TotalSavedEst int
	// TotalWorkerTokens is a diagnostic sum, not a cost. TotalWorkerCostUSD is
	// the figure to reason about.
	TotalWorkerTokens  int
	TotalWorkerCostUSD float64
	// CostedCalls is how many entries carried a real cost figure. Entries
	// written before cost was recorded, or by a CLI that did not report it,
	// have none — so the cost total is a floor over this many calls, not over
	// every interception.
	CostedCalls int
	// SavedEstByTool lets `skim stats` show which path is actually earning its
	// keep rather than only a grand total.
	SavedEstByTool map[string]int
	CostUSDByTool  map[string]float64
}

func Aggregate(r io.Reader) (Stats, error) {
	s := Stats{
		Interceptions:  map[string]int{},
		SavedEstByTool: map[string]int{},
		CostUSDByTool:  map[string]float64{},
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		if e.Tool != "" {
			s.Interceptions[e.Tool]++
			s.SavedEstByTool[e.Tool] += e.SavedEst
			s.CostUSDByTool[e.Tool] += e.WorkerCostUSD
		}
		// nil means "this path has no cache", which is not a miss.
		if e.CacheHit != nil {
			if *e.CacheHit {
				s.CacheHits++
			} else {
				s.CacheMisses++
			}
		}
		s.TotalSavedEst += e.SavedEst
		s.TotalWorkerTokens += e.WorkerTokens
		s.TotalWorkerCostUSD += e.WorkerCostUSD
		if e.WorkerCostUSD > 0 {
			s.CostedCalls++
		}
	}
	return s, sc.Err()
}
