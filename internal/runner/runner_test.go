package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

func TestRun_CapturesOutputAndPrintsDigest(t *testing.T) {
	runs := t.TempDir()
	var out bytes.Buffer
	d := Deps{
		Model: "m", TimeoutSec: 5, RunsDir: runs,
		Now:    func() time.Time { return time.Unix(0, 0) },
		Stdout: &out,
		Summarize: func(_ context.Context, req worker.Request) ([]byte, worker.Usage, error) {
			if !strings.Contains(req.Content, "hello-from-cmd") {
				t.Fatalf("worker should receive captured output, got %q", req.Content)
			}
			return []byte(`{"summary":"printed a greeting","key_lines":["hello-from-cmd"],"exit_code":0}`),
				worker.Usage{InputTokens: 40, OutputTokens: 2, CostUSD: 0.001}, nil
		},
	}
	err := Run(context.Background(), []string{"sh", "-c", "echo hello-from-cmd"}, d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "printed a greeting") {
		t.Fatalf("digest not printed: %q", out.String())
	}
	entries, _ := os.ReadDir(runs)
	if len(entries) != 1 {
		t.Fatalf("expected one run log, got %d", len(entries))
	}
	b, _ := os.ReadFile(filepath.Join(runs, entries[0].Name()))
	if !strings.Contains(string(b), "hello-from-cmd") {
		t.Fatalf("log file missing output: %q", b)
	}
}

func TestRun_WorkerFails_FallbackDigest(t *testing.T) {
	var out bytes.Buffer
	d := Deps{
		Model: "m", TimeoutSec: 5, RunsDir: t.TempDir(),
		Now: func() time.Time { return time.Unix(0, 0) }, Stdout: &out,
		Summarize: func(_ context.Context, _ worker.Request) ([]byte, worker.Usage, error) {
			return nil, worker.Usage{}, context.DeadlineExceeded
		},
	}
	if err := Run(context.Background(), []string{"sh", "-c", "echo x; exit 3"}, d); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Full output:") || !strings.Contains(s, "exit 3") {
		t.Fatalf("fallback digest should still give log path and exit code, got %q", s)
	}
}

// TestRun_RecordsMetrics is the regression guard for the Bash path being
// invisible. `skim run` is where that path's tokens are actually saved and its
// worker cost actually incurred, but it recorded nothing at all — verified by
// watching metrics.jsonl stay at the same line count across a run — so
// `skim stats` showed a Bash interception count with zero savings and zero
// cost, and no way to tell whether the redirect was worth it.
func TestRun_RecordsMetrics(t *testing.T) {
	var out bytes.Buffer
	var recorded []metrics.Entry
	d := Deps{
		Model: "m", TimeoutSec: 5, RunsDir: t.TempDir(),
		Now: func() time.Time { return time.Unix(0, 0) }, Stdout: &out,
		Record: func(e metrics.Entry) { recorded = append(recorded, e) },
		Summarize: func(_ context.Context, _ worker.Request) ([]byte, worker.Usage, error) {
			return []byte(`{"summary":"ran","key_lines":["x"],"exit_code":0}`),
				worker.Usage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.0031}, nil
		},
	}

	// Emit a known number of bytes so the baseline is checkable.
	const lines = 400
	script := "for i in $(seq 1 400); do echo 0123456789012345678; done"
	if err := Run(context.Background(), []string{"sh", "-c", script}, d); err != nil {
		t.Fatal(err)
	}

	if len(recorded) != 1 {
		t.Fatalf("recorded %d entries, want exactly 1", len(recorded))
	}
	e := recorded[0]
	if e.Tool != "Run" {
		t.Errorf("Tool = %q, want Run (distinct from the Bash redirect entry)", e.Tool)
	}
	// The baseline is everything the command emitted, not the retained tail.
	wantOrig := metrics.EstimateTokens(lines * 20)
	if e.OrigTokensEst != wantOrig {
		t.Errorf("OrigTokensEst = %d, want %d (the whole output stream)", e.OrigTokensEst, wantOrig)
	}
	if e.SavedEst != e.OrigTokensEst-e.DigestTokensEst {
		t.Errorf("SavedEst = %d, want orig-digest", e.SavedEst)
	}
	if e.SavedEst <= 0 {
		t.Errorf("SavedEst = %d, expected a positive saving on 8KB of output", e.SavedEst)
	}
	// The cost side must be carried through, or the ledger is one-sided again.
	if e.WorkerCostUSD != 0.0031 {
		t.Errorf("WorkerCostUSD = %v, want 0.0031", e.WorkerCostUSD)
	}
	if e.WorkerInputTokens != 100 || e.WorkerOutputTokens != 20 {
		t.Errorf("per-tier tokens = %d/%d, want 100/20", e.WorkerInputTokens, e.WorkerOutputTokens)
	}
	// nil: the same command run twice can produce different output, so there is
	// no digest cache on this path to hit or miss.
	if e.CacheHit != nil {
		t.Errorf("CacheHit = %v, want nil (skim run has no cache)", *e.CacheHit)
	}
}

// TestRun_NilRecord_DoesNotPanic keeps Record optional: a caller that only
// wants the digest should not have to wire the ledger.
func TestRun_NilRecord_DoesNotPanic(t *testing.T) {
	var out bytes.Buffer
	d := Deps{
		Model: "m", TimeoutSec: 5, RunsDir: t.TempDir(),
		Now: func() time.Time { return time.Unix(0, 0) }, Stdout: &out,
		Summarize: func(_ context.Context, _ worker.Request) ([]byte, worker.Usage, error) {
			return []byte(`{"summary":"ok","exit_code":0}`), worker.Usage{}, nil
		},
	}
	if err := Run(context.Background(), []string{"sh", "-c", "echo hi"}, d); err != nil {
		t.Fatal(err)
	}
}

// TestRun_TailOnly_IsDisclosed covers output larger than the retained tail.
// The runner keeps only the last tailCap bytes, so the digest describes the end
// of the stream — `cat`ing a large file through `skim run` used to be summarised
// as if it were the whole thing, with nothing saying otherwise.
func TestRun_TailOnly_IsDisclosed(t *testing.T) {
	var out bytes.Buffer
	var gotReq worker.Request
	d := Deps{
		Model: "m", TimeoutSec: 30, RunsDir: t.TempDir(),
		Now: func() time.Time { return time.Unix(0, 0) }, Stdout: &out,
		Summarize: func(_ context.Context, req worker.Request) ([]byte, worker.Usage, error) {
			gotReq = req
			return []byte(`{"summary":"lots of output","exit_code":0}`), worker.Usage{}, nil
		},
	}
	// Comfortably more than tailCap (16KB): 2000 lines x 20 bytes = 40KB.
	script := "for i in $(seq 1 2000); do echo 0123456789012345678; done"
	if err := Run(context.Background(), []string{"sh", "-c", script}, d); err != nil {
		t.Fatal(err)
	}

	if len(gotReq.Content) > tailCap {
		t.Errorf("worker got %d bytes, must not exceed tailCap %d", len(gotReq.Content), tailCap)
	}
	if !gotReq.Partial {
		t.Error("worker.Request.Partial should be set when output exceeded the tail buffer")
	}
	if !strings.Contains(out.String(), "PARTIAL") {
		t.Errorf("digest must disclose it only summarises the tail:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Full output:") {
		t.Error("digest must still point at the complete log")
	}
}
