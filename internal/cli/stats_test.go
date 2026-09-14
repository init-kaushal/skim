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
	if err := Stats(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no interceptions") {
		t.Fatalf("got %q", out.String())
	}
}

func TestStats_Summarises(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	metrics.Record(metrics.Entry{Tool: "Read", SavedEst: 5000, CacheHit: false})
	metrics.Record(metrics.Entry{Tool: "Read", SavedEst: 5000, CacheHit: true})
	var out bytes.Buffer
	if err := Stats(&out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Read") || !strings.Contains(s, "10000") {
		t.Fatalf("stats missing expected numbers: %q", s)
	}
}
