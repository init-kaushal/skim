package decision_test

import (
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
