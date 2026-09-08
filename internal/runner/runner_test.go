package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaushal/skim/internal/worker"
)

func TestRun_CapturesOutputAndPrintsDigest(t *testing.T) {
	runs := t.TempDir()
	var out bytes.Buffer
	d := Deps{
		Model: "m", TimeoutSec: 5, RunsDir: runs,
		Now:    func() time.Time { return time.Unix(0, 0) },
		Stdout: &out,
		Summarize: func(_ context.Context, req worker.Request) ([]byte, error) {
			if !strings.Contains(req.Content, "hello-from-cmd") {
				t.Fatalf("worker should receive captured output, got %q", req.Content)
			}
			return []byte(`{"summary":"printed a greeting","key_lines":["hello-from-cmd"],"exit_code":0}`), nil
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
		Summarize: func(_ context.Context, _ worker.Request) ([]byte, error) {
			return nil, context.DeadlineExceeded
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
