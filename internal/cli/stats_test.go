package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/metrics"
)

func TestStats_NoFile(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	var out bytes.Buffer
	if err := Stats(&out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no interceptions") {
		t.Fatalf("got %q", out.String())
	}
}

func TestStats_Summarises(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	metrics.Record(metrics.Entry{Tool: "Read", SavedEst: 5000, CacheHit: metrics.Miss()})
	metrics.Record(metrics.Entry{Tool: "Read", SavedEst: 5000, CacheHit: metrics.Hit()})
	var out bytes.Buffer
	if err := Stats(&out, nil); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Read") || !strings.Contains(s, "10000") {
		t.Fatalf("stats missing expected numbers: %q", s)
	}
}

// TestStats_ReportsDollarsNotTokenArithmetic is the regression guard for the
// headline number being meaningless. The old output printed
// "net = tokens saved - worker tokens", which subtracted tokens billed at the
// worker model's four different rates from tokens billed at the session
// model's rate. On one real 92KB file that read as "net ~-64,349 tokens" and
// looked like a disaster, when the same call was actually a net win in dollars.
func TestStats_ReportsDollarsNotTokenArithmetic(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	// 20,000 tokens kept out of context for 1.6 cents of worker spend.
	if err := metrics.Record(metrics.Entry{
		Tool: "Read", SavedEst: 20000, WorkerTokens: 30000,
		WorkerCostUSD: 0.0160, CacheHit: metrics.Miss(),
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Stats(&out, []string{"--session-model", "opus-5", "--turns", "0"}); err != nil {
		t.Fatal(err)
	}
	s := out.String()

	// 20,000 tokens x $5/Mtok = $0.10 saved against $0.016 spent: clearly ahead.
	if !strings.Contains(s, "$0.1000") {
		t.Errorf("expected the saving priced at the session model's rate:\n%s", s)
	}
	if !strings.Contains(s, "ahead") {
		t.Errorf("expected an 'ahead' verdict on a 6x win:\n%s", s)
	}
	// The assumption must be visible, since the session model cannot be detected
	// from the hook payload.
	if !strings.Contains(s, "assuming a opus-5 session") {
		t.Errorf("the session-model assumption must be stated:\n%s", s)
	}
	// The token sum may appear, but only labelled as a diagnostic.
	if !strings.Contains(s, "diagnostic only") {
		t.Errorf("worker token sum must be marked as not-a-cost:\n%s", s)
	}
}

// TestStats_VerdictFlipsWithSessionModel pins the fact the whole design turns
// on: the same interception is a win on Opus and a loss on a Haiku session,
// because the saving is priced at the session model's rate while the cost is
// fixed at the worker's.
func TestStats_VerdictFlipsWithSessionModel(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	if err := metrics.Record(metrics.Entry{
		Tool: "Read", SavedEst: 20000, WorkerCostUSD: 0.0500, CacheHit: metrics.Miss(),
	}); err != nil {
		t.Fatal(err)
	}

	// Opus: 20,000 x $5/Mtok = $0.10 > $0.05 spent.
	var opus bytes.Buffer
	if err := Stats(&opus, []string{"--session-model", "opus-5", "--turns", "0"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opus.String(), "ahead") {
		t.Errorf("opus-5 should be ahead:\n%s", opus.String())
	}

	// Haiku: 20,000 x $1/Mtok = $0.02 < $0.05 spent.
	var haiku bytes.Buffer
	if err := Stats(&haiku, []string{"--session-model", "haiku-4.5", "--turns", "0"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(haiku.String(), "BEHIND") {
		t.Errorf("haiku-4.5 should be behind:\n%s", haiku.String())
	}
}

// TestStats_CachelessToolsExcludedFromHitRate is the reporting half of the
// cache-rate fix: Bash and Run entries must not appear in the denominator.
func TestStats_CachelessToolsExcludedFromHitRate(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	for _, e := range []metrics.Entry{
		{Tool: "Read", CacheHit: metrics.Hit()},
		{Tool: "Bash"},
		{Tool: "Bash"},
		{Tool: "Run", SavedEst: 900, WorkerCostUSD: 0.003},
	} {
		if err := metrics.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := Stats(&out, nil); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "1 hit / 0 miss (100% hit rate") {
		t.Errorf("cacheless tools must not dilute the hit rate:\n%s", s)
	}
	// Bash redirects without saving anything; the output should explain that
	// rather than look like a bug.
	if !strings.Contains(s, "Bash only redirects") {
		t.Errorf("expected the Bash/Run split to be explained:\n%s", s)
	}
}

func TestStats_UnknownSessionModel(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	var out bytes.Buffer
	err := Stats(&out, []string{"--session-model", "gpt-9"})
	if err == nil {
		t.Fatal("expected an error for an unknown session model")
	}
	if !strings.Contains(err.Error(), "haiku-4.5") {
		t.Errorf("error should list the known models, got: %v", err)
	}
}
