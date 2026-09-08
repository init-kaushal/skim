package digest

import (
	"flag"
	"os"
	"path/filepath"
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
	checkGolden(t, "filemap", RenderFileMap(fm, "/home/u/proj/internal/config/config.go"))
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
