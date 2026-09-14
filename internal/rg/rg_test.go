package rg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/hookio"
)

// These tests drive the real ripgrep binary rather than asserting on argument
// slices. The bugs they guard against were both semantic — the wrong rg
// counting mode, and flags dropped on the floor — and an argument-shape
// assertion would have happily passed while skim still counted the wrong thing.

// requireRg skips only when ripgrep is reachable by neither route. It goes
// through rgPath rather than LookPath("rg") on purpose: these tests originally
// skipped on a machine that had no standalone rg binary but did have ripgrep
// inside the Claude Code executable — the exact configuration in which skim's
// Grep interception was silently dead, and therefore the one configuration the
// tests most need to cover.
func requireRg(t *testing.T) {
	t.Helper()
	if _, _, err := Path(); err != nil {
		t.Skip("ripgrep unavailable by any route:", err)
	}
}

// TestPath_FallsBackToClaude documents the resolution order and proves the
// resolved binary really is ripgrep, whichever route it came from.
func TestPath_FallsBackToClaude(t *testing.T) {
	path, argv0, err := Path()
	if err != nil {
		t.Skip("neither rg nor claude on PATH")
	}
	if argv0 != path && argv0 != "rg" {
		t.Errorf("argv0 = %q, want either the resolved path or \"rg\"", argv0)
	}

	// Whatever was resolved must answer to --version as ripgrep, or the
	// fallback is not a real substitute.
	cmd, err := Command("--version")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolved rg could not run: %v", err)
	}
	if !strings.Contains(string(out), "ripgrep") {
		t.Errorf("resolved binary does not identify as ripgrep: %q", out)
	}
}

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestCount_CountsLinesNotOccurrences pins the counting-mode fix. Claude
// Code's Grep in content mode returns one line per match and grep_max_matches
// is compared against that, but skim used `--count-matches`, which counts
// occurrences. Any line with more than one hit inflated the number and pushed
// calls toward interception they had not earned.
func TestCount_CountsLinesNotOccurrences(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{
		"a.txt": "foo foo foo\nfoo\nbar\n", // 4 occurrences across 2 lines
	})

	got, err := Count(hookio.GrepInput{Pattern: "foo", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("rgCount = %d, want 2 matching lines (--count-matches would say 4)", got)
	}
}

// TestCount_HonoursCaseInsensitive is the flag-fidelity fix. `-i` arrives as
// a literal "-i" key in the tool_input and was not parsed at all, so skim
// counted a case-sensitive version of a case-insensitive search — 1 instead of
// 3 here, which is the difference between passing a call through and
// intercepting it.
func TestCount_HonoursCaseInsensitive(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{"b.txt": "FOO\nFoo\nfoo\n"})

	sensitive, err := Count(hookio.GrepInput{Pattern: "foo", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if sensitive != 1 {
		t.Errorf("case-sensitive count = %d, want 1", sensitive)
	}

	insensitive, err := Count(hookio.GrepInput{Pattern: "foo", Path: dir, CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if insensitive != 3 {
		t.Errorf("case-insensitive count = %d, want 3 — the -i flag was dropped", insensitive)
	}
}

// TestCount_HonoursScope keeps the glob/type restriction working through the
// refactor that split matching args from output args.
func TestCount_HonoursScope(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{
		"keep.go":   "needle\n",
		"skip.md":   "needle\n",
		"sub/x.go":  "needle\n",
		"sub/y.txt": "needle\n",
	})

	all, err := Count(hookio.GrepInput{Pattern: "needle", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if all != 4 {
		t.Fatalf("unscoped count = %d, want 4", all)
	}

	goOnly, err := Count(hookio.GrepInput{Pattern: "needle", Path: dir, Glob: "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if goOnly != 2 {
		t.Errorf("glob-scoped count = %d, want 2", goOnly)
	}
}

// TestCount_HonoursMultiline covers the third matching flag, and the failure
// mode here turned out to be sharper than a miscount. ripgrep REFUSES a pattern
// containing a literal "\n" unless multiline mode is on — it exits 2 with
// `the literal "\n" is not allowed in a regex` rather than reporting zero
// matches. Since rgCount treats only exit 1 as "no matches", dropping the flag
// made every multiline Grep an error, which degraded open: skim never
// intercepted a multiline search at all, silently.
func TestCount_HonoursMultiline(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{"c.txt": "alpha\nbeta\n"})
	gi := hookio.GrepInput{Pattern: `alpha\nbeta`, Path: dir}

	// Without the flag: rg rejects the pattern outright. Surfacing this as an
	// error is correct — the hook degrades open rather than guessing.
	if _, err := Count(gi); err == nil {
		t.Error("expected an error for a \\n pattern without --multiline")
	}

	// With it: the pattern matches across the newline and is counted.
	gi.Multiline = true
	with, err := Count(gi)
	if err != nil {
		t.Fatalf("multiline count failed: %v", err)
	}
	if with != 1 {
		t.Errorf("count with --multiline = %d, want 1", with)
	}
}

// TestCount_NoMatchesIsNotAnError keeps rg's exit-1-on-no-matches from being
// treated as a failure, which would degrade open on a perfectly ordinary Grep.
func TestCount_NoMatchesIsNotAnError(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{"d.txt": "nothing here\n"})
	got, err := Count(hookio.GrepInput{Pattern: "zzzzz", Path: dir})
	if err != nil {
		t.Fatalf("no matches must not be an error: %v", err)
	}
	if got != 0 {
		t.Errorf("count = %d, want 0", got)
	}
}

// TestSample_ReflectsRequestedContext covers the saving baseline. The sample
// used to force `-n -C1` regardless of what the call asked for, so fullBytes —
// now the figure the saving is measured against — described output the Grep
// would never have produced.
func TestSample_ReflectsRequestedContext(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{
		"e.txt": "pad1\npad2\nneedle\npad3\npad4\n",
	})

	bare, bareBytes, err := Sample(hookio.GrepInput{Pattern: "needle", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(bare, "\n"); n != 1 {
		t.Errorf("no-context sample has %d lines, want 1", n)
	}

	ctx, ctxBytes, err := Sample(hookio.GrepInput{Pattern: "needle", Path: dir, Context: 2})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(ctx, "\n"); n != 5 {
		t.Errorf("-C2 sample has %d lines, want 5 (match plus 2 either side)", n)
	}
	if ctxBytes <= bareBytes {
		t.Errorf("context request should enlarge the baseline: %d vs %d", ctxBytes, bareBytes)
	}
}

// TestSample_ReportsUncappedSize pins the other half of the baseline fix:
// the returned byte count must be the size BEFORE the 32KB prompt cap, since
// that cap is skim's own constraint and not something the Grep would have
// applied.
func TestSample_ReportsUncappedSize(t *testing.T) {
	requireRg(t)
	// ~60KB of matching lines, comfortably past sampleCap.
	var sb strings.Builder
	for i := 0; i < 3000; i++ {
		sb.WriteString("needle 0123456789\n")
	}
	dir := fixture(t, map[string]string{"big.txt": sb.String()})

	sample, full, err := Sample(hookio.GrepInput{Pattern: "needle", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(sample) > sampleCap {
		t.Errorf("sample is %d bytes, must be capped at %d", len(sample), sampleCap)
	}
	if full <= sampleCap {
		t.Fatalf("fullBytes = %d, expected the uncapped size to exceed the cap", full)
	}
	if full <= len(sample) {
		t.Errorf("fullBytes (%d) must exceed the capped sample (%d)", full, len(sample))
	}
}

// TestArgs_PatternAfterDoubleDash keeps a pattern that begins with a dash
// from being parsed as an rg flag.
func TestArgs_PatternAfterDoubleDash(t *testing.T) {
	args := Args(hookio.GrepInput{Pattern: "-i-am-a-pattern", Path: "/tmp"}, "--count")
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep < 0 {
		t.Fatalf("no -- separator in %v", args)
	}
	if args[sep+1] != "-i-am-a-pattern" {
		t.Errorf("pattern must follow --, got %v", args)
	}
}

// TestArgs_MatchFlagsPrecedeSeparator makes sure the split between matching
// and output args did not accidentally place flags after `--`, where rg would
// read them as paths.
func TestArgs_MatchFlagsPrecedeSeparator(t *testing.T) {
	args := Args(hookio.GrepInput{
		Pattern: "x", Path: "/tmp", CaseInsensitive: true, Multiline: true, Glob: "*.go",
	}, "--count")
	joined := strings.Join(args, " ")
	sep := strings.Index(joined, " -- ")
	if sep < 0 {
		t.Fatalf("no -- separator in %q", joined)
	}
	for _, flag := range []string{"-i", "--multiline", "--glob"} {
		if at := strings.Index(joined, flag); at < 0 || at > sep {
			t.Errorf("%s must appear before the -- separator: %q", flag, joined)
		}
	}
}

// TestOutputArgs_ContextOverridesAfterBefore documents the precedence: rg's
// -C sets both sides, so passing it alongside -A/-B would be redundant and
// could conflict.
func TestOutputArgs_ContextOverridesAfterBefore(t *testing.T) {
	got := strings.Join(outputArgs(hookio.GrepInput{Context: 3, After: 9, Before: 9}), " ")
	if got != "-C 3" {
		t.Errorf("rgOutputArgs = %q, want %q", got, "-C 3")
	}

	got = strings.Join(outputArgs(hookio.GrepInput{After: 2, Before: 1, LineNumbers: true}), " ")
	if !strings.Contains(got, "-A 2") || !strings.Contains(got, "-B 1") || !strings.Contains(got, "-n") {
		t.Errorf("rgOutputArgs = %q, want -A 2, -B 1 and -n", got)
	}
}

// TestOutputArgs_ExcludedFromCount guards the split itself: context lines
// must never reach the counting pass, where they would inflate a match count.
func TestOutputArgs_ExcludedFromCount(t *testing.T) {
	requireRg(t)
	dir := fixture(t, map[string]string{
		"f.txt": "pad\npad\nneedle\npad\npad\n",
	})
	gi := hookio.GrepInput{Pattern: "needle", Path: dir, Context: 2}

	got, err := Count(gi)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("rgCount = %d, want 1 — context lines must not be counted as matches", got)
	}
}

// TestVerify_AcceptsRealRipgrep is the positive case: whichever route Path
// resolved, the binary must identify itself as ripgrep.
func TestVerify_AcceptsRealRipgrep(t *testing.T) {
	requireRg(t)
	ver, err := Verify()
	if err != nil {
		t.Fatalf("Verify failed on a working install: %v", err)
	}
	if !strings.Contains(strings.ToLower(ver), "ripgrep") {
		t.Errorf("version line = %q, want it to mention ripgrep", ver)
	}
}

// TestVerify_RejectsAnImpostor is the case that motivated Verify. The fallback
// assumes a binary named `claude` is Claude Code, and therefore ripgrep. When
// that assumption is wrong, Count parses the impostor's output as ripgrep
// counts and returns a quietly incorrect number — a 240-match search counted
// low enough to pass through untouched during testing. Verify has to name that.
func TestVerify_RejectsAnImpostor(t *testing.T) {
	dir := t.TempDir()
	// No `rg`; a `claude` that is emphatically not ripgrep.
	impostor := filepath.Join(dir, "claude")
	if err := os.WriteFile(impostor,
		[]byte("#!/bin/sh\necho '{\"type\":\"result\",\"result\":\"hello\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if _, _, err := Path(); err != nil {
		t.Fatalf("Path should still resolve the fallback: %v", err)
	}
	ver, err := Verify()
	if err == nil {
		t.Errorf("Verify accepted a non-ripgrep binary (reported %q)", ver)
	}
}

// TestPath_NoRipgrepAnywhere covers the honest failure: with neither binary
// present, callers get an error and the Grep hook degrades open.
func TestPath_NoRipgrepAnywhere(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, _, err := Path(); err == nil {
		t.Error("expected an error when neither rg nor claude is on PATH")
	}
	if _, err := Count(hookio.GrepInput{Pattern: "x", Path: "."}); err == nil {
		t.Error("Count must surface the resolution failure so the hook degrades open")
	}
}
