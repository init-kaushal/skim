package cli

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/detect"
)

// TestDemo_WritesAFileThatActuallyTripsInterception is the point of the demo.
// A sample that falls under the thresholds would produce no digest, and the one
// command whose job is to prove skim works would silently prove nothing.
func TestDemo_WritesAFileThatActuallyTripsInterception(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := Demo(&out, dir); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, DemoFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("demo file not written: %v", err)
	}

	// Measured the same way the Read hook measures it.
	sz, err := detect.Measure(path, 2000)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	if !detect.ExceedsThreshold(sz.Lines, sz.Bytes, cfg) {
		t.Errorf("demo file is %d lines / %d bytes — under the %d-line / %d-byte "+
			"thresholds, so it would pass through and demonstrate nothing",
			sz.Lines, sz.Bytes, cfg.ReadMaxLines, cfg.ReadMaxBytes)
	}

	// It must not itself be a passthrough path, or the same silent failure.
	if detect.MatchPassthrough(path, cfg.PassthroughGlobs) {
		t.Errorf("demo file at %s matches a passthrough glob", path)
	}

	// The whole file must fit inside the worker's line cap, or the demo shows a
	// PARTIAL banner and muddies the point on a first run.
	if sz.Lines > 2000 {
		t.Errorf("demo file is %d lines; over the 2000-line worker cap it would "+
			"render as a PARTIAL digest", sz.Lines)
	}

	if len(body) == 0 {
		t.Fatal("demo file is empty")
	}
}

// TestDemo_GeneratesValidGoFormattedSource keeps the sample credible. It is the
// first thing a user will look at, in a Go project whose own CI is gofmt-clean.
func TestDemo_GeneratesValidGoFormattedSource(t *testing.T) {
	src := []byte(demoFile())
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("demo file is not valid Go: %v", err)
	}
	if !bytes.Equal(src, formatted) {
		t.Error("demo file is not gofmt-clean; run the generator output through " +
			"gofmt -d to see the difference")
	}
}

// TestDemo_HasDistinctSections guards the reason the sample is not just one
// construct repeated: a digest of a uniform file shows nothing interesting, and
// the demo is meant to show the line-range map describing real structure.
func TestDemo_HasDistinctSections(t *testing.T) {
	src := demoFile()
	for _, section := range []string{"Domain types", "Validation", "In-memory storage", "HTTP handlers", "Helpers"} {
		if !strings.Contains(src, section) {
			t.Errorf("demo file is missing the %q section", section)
		}
	}
}

// TestDemo_RefusesDotClaudeDirectory covers the trap that motivated writing the
// sample into the user's own directory. An installed plugin lives under
// ~/.claude/plugins/, which matches the default `.claude/**` passthrough glob —
// a sample written there is never intercepted, so the demo has to refuse rather
// than appear to work.
func TestDemo_RefusesDotClaudeDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude", "plugins", "skim")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := Demo(&out, dir)
	if err == nil {
		t.Fatal("expected Demo to refuse a path under .claude/")
	}
	if !strings.Contains(err.Error(), "passthrough") {
		t.Errorf("error should explain why, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, DemoFileName)); statErr == nil {
		t.Error("Demo must not write a file it has refused to write")
	}
}

// TestDemo_TellsTheUserWhatToDoNext matters because the file alone demonstrates
// nothing — the effect only appears when a Read happens.
func TestDemo_TellsTheUserWhatToDoNext(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := Demo(&out, dir); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		DemoFileName,  // the path it wrote
		"Read ",       // the action that triggers the effect
		"/skim stats", // how to see the numbers
		"/skim off",   // how to see the contrast
		"rm ",         // how to clean up
	} {
		if !strings.Contains(s, want) {
			t.Errorf("demo output should mention %q:\n%s", want, s)
		}
	}
}

// TestDemo_DefaultsToWorkingDirectory covers the no-argument form, which is how
// the slash command invokes it.
func TestDemo_DefaultsToWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Demo(&out, ""); err != nil {
		t.Fatal(err)
	}
	// macOS temp dirs are symlinked (/var -> /private/var), so compare by
	// existence in the directory rather than by string equality of paths.
	if _, err := os.Stat(filepath.Join(dir, DemoFileName)); err != nil {
		t.Errorf("no demo file in the working directory: %v", err)
	}
}
