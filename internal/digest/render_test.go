package digest

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create)", err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestRenderFileMap(t *testing.T) {
	fm := FileMap{
		Summary: "Config loader with validation and precedence rules.",
		Map: []MapEntry{
			{Lines: "1-20", Kind: "imports + package doc"},
			{Lines: "21-90", Kind: "type Config + Default()"},
			{Lines: "91-160", Kind: "Load() and Save()"},
		},
		Symbols: []string{"Config", "Default", "Load", "Save"},
		Notes:   "line numbers approximate +/- 3",
	}
	// Whole file seen: the digest describes everything, so no PARTIAL banner.
	checkGolden(t, "filemap", RenderFileMap(fm,
		"/home/u/proj/internal/config/config.go",
		"/home/u/.claude/plugins/skim/bin/skim",
		Coverage{SeenBytes: 4096, TotalBytes: 4096}))
}

// TestRenderFileMap_PartialIsDisclosed is the regression guard for a digest
// that silently passed off a prefix as the whole file. A 908 KB file was capped
// to 400 KB before the worker saw it, yet the rendered digest said only "is
// large — digest instead of full contents": the model got a structure map
// covering 46% of the file with nothing marking the other 54% as undescribed.
func TestRenderFileMap_PartialIsDisclosed(t *testing.T) {
	fm := FileMap{
		Summary: "Generated accessors.",
		Map:     []MapEntry{{Lines: "1-13938", Kind: "generated funcs"}},
	}
	got := RenderFileMap(fm, "/x/huge.go", "/bin/skim",
		Coverage{SeenBytes: 409600, TotalBytes: 907779, SeenLines: 2000, TotalLines: 29999})

	if !strings.Contains(got, "PARTIAL") {
		t.Errorf("partial digest not disclosed:\n%s", got)
	}
	if !strings.Contains(got, "lines 1-2000 of 29999") {
		t.Errorf("disclosure lacks the actual extent:\n%s", got)
	}
	// The banner has to be actionable: recovering the remainder means Read with
	// an offset, which needs a line number, not a byte count.
	if !strings.Contains(got, "Read with offset 2001") {
		t.Errorf("disclosure gives the model no way to recover the rest:\n%s", got)
	}
	// The banner has to precede the map it qualifies, or it reads as a footnote
	// to ranges the model has already trusted.
	if strings.Index(got, "PARTIAL") > strings.Index(got, "Structure:") {
		t.Error("PARTIAL banner must come before the Structure: section")
	}
}

// TestRenderFileMap_SymbolsCapped guards the case where the digest became most
// of what it was meant to save: a 2,002-line file of 400 near-identical types
// rendered ~1,300 tokens, ~1,200 of which was every Widget0…Widget399 name.
// TestRenderFileMap_PartialByBytes covers the pathological shape that hits the
// byte backstop instead of the line cap — a minified bundle on one enormous
// line. There is no line number to hand back, so the disclosure falls back to
// bytes rather than printing a nonsensical "Read with offset 1".
func TestRenderFileMap_PartialByBytes(t *testing.T) {
	fm := FileMap{Summary: "minified bundle.", Map: []MapEntry{{Lines: "1", Kind: "everything"}}}
	got := RenderFileMap(fm, "/x/bundle.min.js", "/bin/skim",
		Coverage{SeenBytes: 131072, TotalBytes: 900000, SeenLines: 1, TotalLines: 1})

	if !strings.Contains(got, "PARTIAL") {
		t.Errorf("partial digest not disclosed:\n%s", got)
	}
	if !strings.Contains(got, "131072 of 900000 bytes") {
		t.Errorf("expected the byte-based fallback:\n%s", got)
	}
	if strings.Contains(got, "Read with offset") {
		t.Errorf("must not suggest a line offset when there are no lines to skip to:\n%s", got)
	}
}

func TestRenderFileMap_SymbolsCapped(t *testing.T) {
	syms := make([]string, 400)
	for i := range syms {
		syms[i] = fmt.Sprintf("Widget%d", i)
	}
	fm := FileMap{
		Summary: "400 widget types.",
		Map:     []MapEntry{{Lines: "1-1201", Kind: "widget types"}},
		Symbols: syms,
	}
	got := RenderFileMap(fm, "/x/w.go", "/bin/skim", Coverage{SeenBytes: 71976, TotalBytes: 71976})

	if !strings.Contains(got, "(+360 more)") {
		t.Errorf("expected a count of the elided symbols, got:\n%s", got)
	}
	if strings.Contains(got, "Widget40,") || strings.Contains(got, "Widget399") {
		t.Error("symbols past the cap were still rendered")
	}
	if !strings.Contains(got, "Widget39") {
		t.Error("symbols up to the cap should still be rendered")
	}
	// The whole point is size. A capped digest must be a small fraction of the
	// uncapped one.
	uncapped := len(strings.Join(syms, ", "))
	if len(got) > uncapped/2 {
		t.Errorf("digest still %d bytes against a %d-byte symbol list — cap ineffective",
			len(got), uncapped)
	}
}

func TestCoverage_Partial(t *testing.T) {
	for _, tc := range []struct {
		name string
		cov  Coverage
		want bool
	}{
		{"whole file", Coverage{SeenBytes: 100, TotalBytes: 100}, false},
		{"prefix", Coverage{SeenBytes: 40, TotalBytes: 100}, true},
		{"empty file", Coverage{}, false},
		// A zero total means "size unknown" — never claim partial on a guess.
		{"unknown total", Coverage{SeenBytes: 40}, false},
		// Defensive: seen > total shouldn't read as partial.
		{"over-read", Coverage{SeenBytes: 140, TotalBytes: 100}, false},
	} {
		if got := tc.cov.Partial(); got != tc.want {
			t.Errorf("%s: Partial() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRenderClusters(t *testing.T) {
	c := Clusters{
		Summary:  "Handler wiring for each intercepted tool.",
		Clusters: []Cluster{{Where: "internal/handler/", Matches: 23, Gist: "per-tool orchestration"}, {Where: "internal/detect/", Matches: 11, Gist: "threshold checks"}},
		Total:    34, RepresentativeFiles: []string{"internal/handler/read.go", "internal/detect/size.go"},
	}
	checkGolden(t, "clusters", RenderClusters(c))
}

func TestRenderRun(t *testing.T) {
	r := Run{Summary: "2 tests failed, 44 passed.", KeyLines: []string{"FAIL TestApply/oversize", "2 failed, 44 passed"}, ExitCode: 1, LogPath: "/home/u/.claude/skim/runs/abc123.log"}
	checkGolden(t, "run", RenderRun(r))
}
