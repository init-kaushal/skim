package metrics

import (
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
	must(Record(Entry{Tool: "Read", OrigTokensEst: 7000, DigestTokensEst: 500, SavedEst: 6500, CacheHit: false}))
	must(Record(Entry{Tool: "Read", SavedEst: 6500, CacheHit: true}))
	must(Record(Entry{Tool: "Bash", SavedEst: 1200, CacheHit: false}))

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
