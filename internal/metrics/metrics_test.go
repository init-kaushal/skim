package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	for in, want := range map[int]int{0: 0, 1: 1, 4: 1, 5: 2, 4000: 1000} {
		if got := EstimateTokens(in); got != want {
			t.Errorf("EstimateTokens(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestRecordThenAggregate(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(Record(Entry{Tool: "Read", OrigTokensEst: 7000, DigestTokensEst: 500, SavedEst: 6500, CacheHit: Miss()}))
	must(Record(Entry{Tool: "Read", SavedEst: 6500, CacheHit: Hit()}))
	must(Record(Entry{Tool: "Bash", SavedEst: 1200, CacheHit: Miss()}))

	f, _ := os.Open(Path())
	defer f.Close()
	s, err := Aggregate(f)
	if err != nil {
		t.Fatal(err)
	}
	if s.Interceptions["Read"] != 2 || s.Interceptions["Bash"] != 1 {
		t.Fatalf("interceptions = %+v", s.Interceptions)
	}
	if s.CacheHits != 1 || s.CacheMisses != 2 {
		t.Fatalf("hits=%d misses=%d", s.CacheHits, s.CacheMisses)
	}
	if s.TotalSavedEst != 14200 {
		t.Fatalf("saved = %d, want 14200", s.TotalSavedEst)
	}
}

func TestAggregate_SkipsGarbageLines(t *testing.T) {
	s, err := Aggregate(strings.NewReader("not json\n{\"tool\":\"Read\",\"saved_est\":10}\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Interceptions["Read"] != 1 || s.TotalSavedEst != 10 {
		t.Fatalf("got %+v", s)
	}
}

// TestAggregate_CacheRateIgnoresCachelessPaths is the regression guard for a
// hit rate that described nothing. Aggregate counted every entry as hit-or-miss,
// but Grep and `skim run` have no digest cache and Bash makes no worker call at
// all — so one Read interception plus three Bash redirects reported
// "0 hit / 4 miss (0% hit rate)". Only entries from a cache-consulting path
// belong in the denominator.
func TestAggregate_CacheRateIgnoresCachelessPaths(t *testing.T) {
	lines := []Entry{
		{Tool: "Read", CacheHit: Hit()},
		{Tool: "Read", CacheHit: Miss()},
		{Tool: "Grep"}, // nil: no cache on this path
		{Tool: "Bash"}, // nil: no worker call at all
		{Tool: "Run"},  // nil: output is never cached
	}
	var buf bytes.Buffer
	for _, e := range lines {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(append(b, '\n'))
	}

	s, err := Aggregate(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if s.CacheHits != 1 || s.CacheMisses != 1 {
		t.Errorf("cache = %d hit / %d miss, want 1/1 (the two Read entries only)",
			s.CacheHits, s.CacheMisses)
	}
	if got := s.CacheHits + s.CacheMisses; got != 2 {
		t.Errorf("cache denominator = %d, want 2 — cacheless paths must not dilute it", got)
	}
	if len(s.Interceptions) != 4 {
		t.Errorf("Interceptions covers %d tools, want 4", len(s.Interceptions))
	}
}

// TestAggregate_CostIsSeparateFromTokens pins the distinction the old ledger
// collapsed: the token sum is a size, the dollar figure is the cost, and they
// are not interchangeable.
func TestAggregate_CostIsSeparateFromTokens(t *testing.T) {
	entries := []Entry{
		{Tool: "Read", SavedEst: 1000, WorkerTokens: 30000, WorkerCostUSD: 0.0160},
		{Tool: "Run", SavedEst: 500, WorkerTokens: 5000, WorkerCostUSD: 0.0031},
		// An entry from before cost was recorded: contributes tokens, no cost.
		{Tool: "Read", SavedEst: 250, WorkerTokens: 1000},
	}
	var buf bytes.Buffer
	for _, e := range entries {
		b, _ := json.Marshal(e)
		buf.Write(append(b, '\n'))
	}
	s, err := Aggregate(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if s.TotalSavedEst != 1750 {
		t.Errorf("TotalSavedEst = %d, want 1750", s.TotalSavedEst)
	}
	if s.TotalWorkerTokens != 36000 {
		t.Errorf("TotalWorkerTokens = %d, want 36000", s.TotalWorkerTokens)
	}
	if got := s.TotalWorkerCostUSD; got < 0.0190 || got > 0.0192 {
		t.Errorf("TotalWorkerCostUSD = %v, want ~0.0191", got)
	}
	// Only two of the three entries carried a cost, and stats says so rather
	// than presenting a partial total as complete.
	if s.CostedCalls != 2 {
		t.Errorf("CostedCalls = %d, want 2", s.CostedCalls)
	}
	if s.SavedEstByTool["Read"] != 1250 || s.SavedEstByTool["Run"] != 500 {
		t.Errorf("per-tool savings = %v, want Read:1250 Run:500", s.SavedEstByTool)
	}
}

// TestEntry_CacheHitOmittedWhenNil keeps the wire format clean: a cacheless
// path writes no cache_hit key at all, so it reads back as nil rather than as
// the false that caused the original miscount.
func TestEntry_CacheHitOmittedWhenNil(t *testing.T) {
	b, err := json.Marshal(Entry{Tool: "Grep"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("cache_hit")) {
		t.Errorf("nil CacheHit should be omitted, got %s", b)
	}
	var back Entry
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.CacheHit != nil {
		t.Error("absent cache_hit must decode to nil, not false")
	}
}
