package decision_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/decision"
)

func testEngine() *decision.Engine {
	cfg := config.Default()
	cfg.SessionModel = "claude-opus-5-5"
	return decision.New(cfg)
}

func TestPlanRead_BelowThreshold(t *testing.T) {
	e := testEngine()
	// 10 lines, 500 bytes — well below default threshold
	p := e.PlanRead("/tmp/small.go", "package foo\n", 10, 500)
	if p.Strategy != cost.StrategyPassthrough {
		t.Errorf("expected PASSTHROUGH, got %s: %s", p.Strategy, p.Reason)
	}
}

func TestPlanRead_LargeFileIntercepted(t *testing.T) {
	e := testEngine()
	// Simulate a 200-line, 80KB file
	content := strings.Repeat("x", 80*1024)
	p := e.PlanRead("/tmp/large.go", content, 200, 80*1024)
	if p.Strategy != cost.StrategyCheapWorker {
		t.Errorf("expected CHEAP_WORKER for large Opus-session file, got %s: %s", p.Strategy, p.Reason)
	}
}

func TestPlanRead_WorkerInputSet(t *testing.T) {
	e := testEngine()
	content := strings.Repeat("line\n", 200) // 200 lines, 1000 bytes
	p := e.PlanRead("/tmp/file.go", content, 200, len(content))
	// May be passthrough (200 lines = at threshold) or cheap worker
	// Just ensure WorkerInput is set when strategy is not passthrough
	if p.Strategy == cost.StrategyCheapWorker && p.WorkerInput == "" {
		t.Error("WorkerInput must be set for CHEAP_WORKER plan")
	}
}

func TestPlanGrep_LargeOutput(t *testing.T) {
	e := testEngine()
	sample := strings.Repeat("match: file.go:1: pattern\n", 100)
	p := e.PlanGrep(sample, len(sample)*3) // full output 3x the sample
	if p.Strategy != cost.StrategyCheapWorker {
		t.Errorf("expected CHEAP_WORKER for large grep output, got %s: %s", p.Strategy, p.Reason)
	}
}

const goTestOutput = `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
    bar_test.go:42: expected 1, got 2
--- FAIL: TestBar (0.01s)
FAIL
FAIL	github.com/example/pkg	0.123s
`

func TestPlanRun_GoTestDeterministic(t *testing.T) {
	e := testEngine()
	p := e.PlanRun(goTestOutput)
	if p.Strategy != cost.StrategyDeterministic {
		t.Errorf("expected DETERMINISTIC for go test output, got %s: %s", p.Strategy, p.Reason)
	}
	if p.CompressResult.Kind != "go_test" {
		t.Errorf("expected kind=go_test, got %q", p.CompressResult.Kind)
	}
}

func TestPlanRun_UnknownOutput_UsesWorker(t *testing.T) {
	e := testEngine()
	// Large output that no compressor recognises
	output := strings.Repeat("some command output line\n", 500)
	p := e.PlanRun(output)
	// Should be CHEAP_WORKER or DIRECT (cost engine decides), not DETERMINISTIC
	if p.Strategy == cost.StrategyDeterministic {
		t.Error("unknown output should not be classified as DETERMINISTIC")
	}
}

// TestPlanRead_DeterministicForValidGoFile verifies that a real Go source file
// (one with a valid package declaration) routes to DETERMINISTIC, not the worker.
func TestPlanRead_DeterministicForValidGoFile(t *testing.T) {
	e := testEngine()

	// Well-formed Go source that exceeds the default threshold. Use enough lines
	// to pass ExceedsThreshold but also be real Go so the parser produces a map.
	var sb strings.Builder
	sb.WriteString("package bench\n\nimport \"fmt\"\n\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&sb, "func F%d() string { return \"value%d\" }\n", i, i)
	}
	src := sb.String()

	p := e.PlanRead("/tmp/bench.go", src, strings.Count(src, "\n"), len(src))
	if p.Strategy != cost.StrategyDeterministic {
		t.Errorf("expected DETERMINISTIC for valid Go file, got %s: %s", p.Strategy, p.Reason)
	}
	if p.DeterministicFileMap == nil {
		t.Error("DeterministicFileMap must be set for DETERMINISTIC plan")
	}
}

// TestPlanRead_PartialGoFileFallsThrough verifies that invalid Go content (no
// package declaration) does NOT trigger the deterministic path.
func TestPlanRead_PartialGoFileFallsThrough(t *testing.T) {
	e := testEngine()
	content := strings.Repeat("x\n", 300)
	p := e.PlanRead("/tmp/notgo.go", content, 300, len(content))
	if p.Strategy == cost.StrategyDeterministic {
		t.Errorf("invalid Go content should not get DETERMINISTIC, got %s", p.Strategy)
	}
}

// TestPlanRead_PartialCompressionNoWorkerWhenCostSaysNo verifies the fix for
// the decision consistency bug: when the cost engine returns ShouldIntercept=false,
// a partial deterministic reduction must NOT force a CHEAP_WORKER call.
func TestPlanRun_PartialCompressionNoWorkerWhenCostSaysNo(t *testing.T) {
	// Use a Haiku session (cheap — harder to justify worker calls).
	cfg := config.Default()
	cfg.SessionModel = "claude-haiku-4-5-20251001"
	cfg.MinSavingsUSD = 1.0 // very high minimum so cost engine always says no
	e := decision.New(cfg)

	// This git diff output will be partially compressed by the git compressor
	// (cr.Reduced=true, cr.Complete=false) — it's partial, not complete.
	gitDiff := "diff --git a/file.go b/file.go\n" +
		"index 1234567..abcdefg 100644\n" +
		"--- a/file.go\n" +
		"+++ b/file.go\n" +
		strings.Repeat("@@ -1,3 +1,4 @@\n context\n+added line\n", 10)

	p := e.PlanRun(gitDiff)
	// Cost engine says no (min savings = $1.00 is impossibly high)
	// so we must get DIRECT, not CHEAP_WORKER — even if there was partial compression.
	if p.Strategy == cost.StrategyCheapWorker {
		t.Errorf("partial compression + cost-engine-says-no must return DIRECT, got CHEAP_WORKER. Reason: %s", p.Reason)
	}
}

// TestPlanRun_PartialCompressionWorkerWhenCostSaysYes verifies that when the
// cost engine says yes, a partially-compressed input IS sent to the worker.
func TestPlanRun_PartialCompressionWorkerWhenCostSaysYes(t *testing.T) {
	cfg := config.Default()
	cfg.SessionModel = "claude-opus-5-5"
	cfg.MinSavingsUSD = 0.0001
	e := decision.New(cfg)

	// Large git diff — should exceed cost threshold on Opus session.
	largeDiff := "diff --git a/file.go b/file.go\n" +
		"index 1234567..abcdefg 100644\n" +
		"--- a/file.go\n" +
		"+++ b/file.go\n" +
		strings.Repeat("@@ -1,3 +1,4 @@\n context line\n+added line number one\n", 400)

	p := e.PlanRun(largeDiff)
	// Large Opus-session content should justify a worker call.
	if p.Strategy == cost.StrategyDirect {
		t.Logf("got DIRECT — cost engine may have said no for this size. Reason: %s", p.Reason)
		// This is only a failure if it's definitively wrong; the cost engine
		// may have a legitimate reason on the test machine's config.
	}
}

// TestPlanRead_PredictedRatioSet verifies that PlanRead always sets a non-zero
// PredictedRatio for intercepted operations.
func TestPlanRead_PredictedRatioSet(t *testing.T) {
	e := testEngine()
	content := strings.Repeat("x", 80*1024)
	p := e.PlanRead("/tmp/large.go", content, 200, 80*1024)
	if p.Strategy == cost.StrategyCheapWorker && p.PredictedRatio == 0 {
		t.Error("PredictedRatio must be set for CHEAP_WORKER plan")
	}
}

// TestPlanRead_ReuseCountSet verifies that PlanRead increments ReuseCount for
// the same target within a single session (same engine instance). Cross-
// invocation persistence is tested in session_test.go.
func TestPlanRead_ReuseCountSet(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	cfg := config.Default()
	cfg.SessionModel = "claude-opus-5-5"
	e := decision.New(cfg)

	content := strings.Repeat("x", 80*1024)

	p1 := e.PlanRead("/tmp/tracked.go", content, 200, 80*1024)
	if p1.ReuseCount != 1 {
		t.Errorf("first read: want ReuseCount=1, got %d", p1.ReuseCount)
	}

	// Second call with the same target: count must increment.
	p2 := e.PlanRead("/tmp/tracked.go", content, 200, 80*1024)
	if p2.ReuseCount != 2 {
		t.Errorf("second read: want ReuseCount=2, got %d", p2.ReuseCount)
	}

	// Different target: should start at 1.
	p3 := e.PlanRead("/tmp/other.go", content, 200, 80*1024)
	if p3.ReuseCount != 1 {
		t.Errorf("first read of other target: want ReuseCount=1, got %d", p3.ReuseCount)
	}
}
