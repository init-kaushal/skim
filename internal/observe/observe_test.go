package observe_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/observe"
)

func withTempHome(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	old := os.Getenv("SKIM_HOME")
	os.Setenv("SKIM_HOME", dir)
	return func() { os.Setenv("SKIM_HOME", old) }
}

func TestRecord_And_Read(t *testing.T) {
	defer withTempHome(t)()

	e1 := observe.Entry{Tool: "Read", Target: "/tmp/foo.go", Strategy: cost.StrategyCheapWorker, Reason: "test"}
	e2 := observe.Entry{Tool: "Grep", Target: "pattern", Strategy: cost.StrategyDirect, Reason: "too cheap"}
	observe.Record(e1)
	observe.Record(e2)

	entries, err := observe.Read(observe.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Tool != "Read" {
		t.Errorf("expected first entry tool=Read, got %q", entries[0].Tool)
	}
	if entries[1].Strategy != cost.StrategyDirect {
		t.Errorf("expected second entry strategy=DIRECT, got %q", entries[1].Strategy)
	}
}

func TestRead_FilterByTool(t *testing.T) {
	defer withTempHome(t)()

	observe.Record(observe.Entry{Tool: "Read", Target: "a.go", Strategy: cost.StrategyCheapWorker})
	observe.Record(observe.Entry{Tool: "Grep", Target: "pattern", Strategy: cost.StrategyDirect})
	observe.Record(observe.Entry{Tool: "Read", Target: "b.go", Strategy: cost.StrategyPassthrough})

	entries, err := observe.Read(observe.Filter{Tool: "Read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 Read entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Tool != "Read" {
			t.Errorf("filter by tool=Read returned %q", e.Tool)
		}
	}
}

func TestRead_FilterByTarget(t *testing.T) {
	defer withTempHome(t)()

	observe.Record(observe.Entry{Tool: "Read", Target: "/repo/auth/service.go"})
	observe.Record(observe.Entry{Tool: "Read", Target: "/repo/main.go"})

	entries, err := observe.Read(observe.Filter{Target: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry matching 'auth', got %d", len(entries))
	}
}

func TestRead_Limit(t *testing.T) {
	defer withTempHome(t)()

	for i := 0; i < 30; i++ {
		observe.Record(observe.Entry{Tool: "Read", Target: "file.go"})
	}

	entries, err := observe.Read(observe.Filter{Last: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected 10 entries with Last=10, got %d", len(entries))
	}
}

func TestRead_NonExistent_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("SKIM_HOME", filepath.Join(dir, "nonexistent"))
	defer os.Unsetenv("SKIM_HOME")

	entries, err := observe.Read(observe.Filter{})
	if err != nil {
		t.Fatalf("non-existent log should return nil, not error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for non-existent log, got %v", entries)
	}
}

func TestFormat_Output(t *testing.T) {
	var buf bytes.Buffer
	e := observe.Entry{
		Tool:             "Read",
		Target:           "/repo/handler.go",
		Strategy:         cost.StrategyCheapWorker,
		EstOrigCostUSD:   0.0042,
		EstWorkerCostUSD: 0.0006,
		EstNetSavingsUSD: 0.0036,
		Reason:           "savings justify cost",
	}
	observe.Format(&buf, e)
	out := buf.String()
	if !containsAll(out, "Read", "handler.go", "CHEAP_WORKER", "$0.0042", "$0.0006", "savings justify cost") {
		t.Errorf("Format output missing expected fields:\n%s", out)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
