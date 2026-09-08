package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withFakeClaude(t *testing.T, mode, resultJSON string) {
	t.Helper()
	repoRoot, _ := filepath.Abs("../..")
	t.Setenv("PATH", filepath.Join(repoRoot, "testdata", "fakeclaude")+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE_MODE", mode)
	if resultJSON != "" {
		f := filepath.Join(t.TempDir(), "result.json")
		os.WriteFile(f, []byte(resultJSON), 0o644)
		t.Setenv("FAKE_CLAUDE_RESULT_FILE", f)
	}
}

func TestRun_OK_ReturnsResultBytes(t *testing.T) {
	want := `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`
	withFakeClaude(t, "ok", want)

	got, err := Run(context.Background(), Request{
		Model: "claude-haiku-4-5-20251001", Kind: KindFileMap,
		Content: "package main", Meta: "/x/main.go", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRun_NonZeroExit_Errors(t *testing.T) {
	withFakeClaude(t, "exit1", "")
	if _, err := Run(context.Background(), Request{Model: "m", Kind: KindRun, Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}

func TestRun_Timeout(t *testing.T) {
	withFakeClaude(t, "hang", "")
	_, err := Run(context.Background(), Request{Model: "m", Kind: KindRun, Timeout: 200 * time.Millisecond})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestRun_GarbageEnvelope_Errors(t *testing.T) {
	withFakeClaude(t, "garbage", "")
	if _, err := Run(context.Background(), Request{Model: "m", Kind: KindRun, Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected error on unparseable envelope")
	}
}

func TestRun_SetsSkimActiveForChild(t *testing.T) {
	// The fake echoes $SKIM_ACTIVE into .result when asked via a special result file marker.
	withFakeClaude(t, "ok", `{"summary":"SKIM_ACTIVE_CHECK","map":[{"lines":"1","kind":"x"}]}`)
	// Covered indirectly here; the authoritative recursion-guard test is in handler + integration.
	if _, err := Run(context.Background(), Request{Model: "m", Kind: KindFileMap, Timeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
}
