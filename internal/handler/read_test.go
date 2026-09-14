package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

func baseDeps(t *testing.T, out *bytes.Buffer) Deps {
	t.Helper()
	return Deps{
		Cfg:      config.Default(),
		Env:      func(string) string { return "" },
		CacheKey: func(p, m string) (string, error) { return "k", nil },
		CacheGet: func(string) (digest.FileMap, bool) { return digest.FileMap{}, false },
		CachePut: func(string, digest.FileMap) error { return nil },
		Record:   func(metrics.Entry) {},
		Logf:     func(string, ...any) {},
		Now:      func() time.Time { return time.Unix(0, 0) },
		Stdout:   out,
		CountMatches: func(hookio.GrepInput) (int, error) {
			return 0, errors.New("no rg")
		},
		Sample: func(hookio.GrepInput) (string, error) {
			return "", errors.New("no rg")
		},
	}
}

// captureEntry swaps in a Record stub that records every metrics.Entry, so a
// test can assert on what skim actually logged — the stats ledger is a product
// surface, not a side effect.
func captureEntry(d *Deps, got *[]metrics.Entry) {
	d.Record = func(e metrics.Entry) { *got = append(*got, e) }
}

func readInput(t *testing.T, path string, offset, limit int) hookio.Input {
	t.Helper()
	js := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":` +
		`{"file_path":"` + path + `","offset":` + strconv.Itoa(offset) + `,"limit":` + strconv.Itoa(limit) + `}}`)
	in, err := hookio.Parse(bytes.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestReadHook_SmallFile_Allows(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "small.go")
	os.WriteFile(f, []byte("package main\n"), 0o644)

	var out bytes.Buffer
	d := baseDeps(t, &out)
	if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("small file should allow (no output), got %q", out.String())
	}
}

func TestReadHook_LargeFile_DeniesWithDigest(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)

	var out bytes.Buffer
	var recorded []metrics.Entry
	d := baseDeps(t, &out)
	captureEntry(&d, &recorded)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
		return []byte(`{"summary":"big file","map":[{"lines":"1-5000","kind":"x lines"}]}`), 777, nil
	}
	if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) {
		t.Fatalf("large file should deny, got %q", out.String())
	}
	if !strings.Contains(out.String(), "big file") {
		t.Fatal("deny reason should contain the digest summary")
	}

	if len(recorded) != 1 {
		t.Fatalf("expected exactly one metrics entry, got %d", len(recorded))
	}
	e := recorded[0]
	if e.Tool != "Read" {
		t.Errorf("Tool = %q, want Read", e.Tool)
	}
	if e.CacheHit {
		t.Error("CacheHit = true, want false (baseDeps CacheGet always misses)")
	}
	if e.WorkerTokens != 777 {
		t.Errorf("WorkerTokens = %d, want 777 (what the worker reported)", e.WorkerTokens)
	}
	if e.OrigTokensEst <= e.DigestTokensEst {
		t.Errorf("digest should be smaller than the original: orig=%d digest=%d",
			e.OrigTokensEst, e.DigestTokensEst)
	}
	if e.SavedEst != e.OrigTokensEst-e.DigestTokensEst {
		t.Errorf("SavedEst = %d, want orig-digest = %d", e.SavedEst, e.OrigTokensEst-e.DigestTokensEst)
	}
}

// TestReadHook_BinaryFile_Allows covers the two binary gates: a known binary
// extension, and an unknown extension whose bytes contain a NUL. Both must pass
// through even though they are far over the size threshold — a digest of an
// image is useless, and denying the Read would hide the file from the model
// entirely.
func TestReadHook_BinaryFile_Allows(t *testing.T) {
	dir := t.TempDir()
	blob := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03}, 40000)...)

	for _, name := range []string{"big.png", "big.bin"} {
		t.Run(name, func(t *testing.T) {
			f := filepath.Join(dir, name)
			if err := os.WriteFile(f, blob, 0o644); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			d := baseDeps(t, &out)
			d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
				t.Fatal("worker must not be called for a binary file")
				return nil, 0, nil
			}
			if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Fatalf("binary file should allow, got %q", out.String())
			}
		})
	}
}

func TestReadHook_WorkerFails_DegradesOpen(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)

	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
		return nil, 0, errors.New("boom")
	}
	if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("worker failure must degrade open (no output), got %q", out.String())
	}
}

func TestReadHook_OffsetPresent_Allows(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
		t.Fatal("summarize must not be called when offset/limit present")
		return nil, 0, nil
	}
	if err := ReadHook(context.Background(), readInput(t, f, 10, 50), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("targeted read should allow")
	}
}

func TestReadHook_RecursionGuard_Allows(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Env = func(k string) string {
		if k == "SKIM_ACTIVE" {
			return "1"
		}
		return ""
	}
	if err := ReadHook(context.Background(), readInput(t, "/whatever", 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("recursion guard should allow")
	}
}

// forceFullFileCommand extracts the "To force the full file:" command from a
// deny reason, similar to suggestedCommand for the Bash hook.
func forceFullFileCommand(t *testing.T, reason string) string {
	t.Helper()
	for _, line := range strings.Split(reason, "\n") {
		if strings.HasPrefix(line, "To force the full file: ") {
			return strings.TrimPrefix(line, "To force the full file: ")
		}
	}
	t.Fatalf("no 'force full file' command found in reason %q", reason)
	return ""
}

// readDenyReason runs ReadHook and returns the permissionDecisionReason it
// emitted, or "" if the read was allowed.
func readDenyReason(t *testing.T, path string) string {
	t.Helper()
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
		return []byte(`{"summary":"large file","map":[{"lines":"1-5000","kind":"code"}]}`), 777, nil
	}
	if err := ReadHook(context.Background(), readInput(t, path, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		return ""
	}
	var dec struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &dec); err != nil {
		t.Fatalf("deny output is not valid JSON: %v (%q)", err, out.String())
	}
	if dec.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("decision = %q, want deny", dec.HookSpecificOutput.PermissionDecision)
	}
	return dec.HookSpecificOutput.PermissionDecisionReason
}

// TestReadHook_SpacedSelfPath_QuotesFull is C1 for the Read hook: when the
// resolved skim binary's absolute path contains whitespace, the "force full
// file" escape-hatch suggestion must shell-quote it (quoteSelfIfNeeded), or
// when the model runs it as Bash and it gets fed back through the Bash hook,
// isSkimCommand won't recognize it and it will be denied again forever.
func TestReadHook_SpacedSelfPath_QuotesFull(t *testing.T) {
	orig := executablePath
	defer func() { executablePath = orig }()
	spaced := "/tmp/skim test dir/bin/skim"
	executablePath = func() (string, error) { return spaced, nil }

	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)

	reason := readDenyReason(t, f)
	if reason == "" {
		t.Fatal("large file should be denied")
	}

	cmd := forceFullFileCommand(t, reason)
	expectedQuoted := shellSingleQuote(spaced) + " cat " + f
	if cmd != expectedQuoted {
		t.Errorf("force full file command = %q, want %q", cmd, expectedQuoted)
	}

	// Verify the quoted form is actually in the suggestion
	if !strings.Contains(cmd, shellSingleQuote(spaced)) {
		t.Fatalf("suggestion should shell-quote the spaced self path, got %q", cmd)
	}
}
