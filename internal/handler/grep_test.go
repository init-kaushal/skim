package handler

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

func grepInput(t *testing.T, pattern string) hookio.Input {
	t.Helper()
	return grepInputRaw(t, `"pattern":"`+pattern+`","output_mode":"content"`)
}

// grepInputRaw builds a Grep PreToolUse input from raw tool_input JSON fields,
// so tests can exercise output_mode / head_limit variants.
func grepInputRaw(t *testing.T, fields string) hookio.Input {
	t.Helper()
	in, err := hookio.Parse(bytes.NewReader([]byte(
		`{"hook_event_name":"PreToolUse","tool_name":"Grep","tool_input":{` + fields + `}}`)))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestGrepHook_FewMatches_Allows(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.CountMatches = func(hookio.GrepInput) (int, error) { return 5, nil }
	d.Sample = func(hookio.GrepInput) (string, error) {
		t.Fatal("sample must not be gathered for a Grep under the threshold")
		return "", nil
	}
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("few matches should allow, got %q", out.String())
	}
}

func TestGrepHook_ManyMatches_DeniesWithClusters(t *testing.T) {
	var out bytes.Buffer
	var recorded []metrics.Entry
	d := baseDeps(t, &out)
	captureEntry(&d, &recorded)
	d.CountMatches = func(hookio.GrepInput) (int, error) { return 500, nil }
	d.Sample = func(hookio.GrepInput) (string, error) { return "big sample", nil }
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
		return []byte(`{"summary":"scattered","clusters":[{"where":"a/","matches":300,"gist":"x"}],"total":500,"representative_files":["a/x.go"]}`), 555, nil
	}
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !strings.Contains(out.String(), "scattered") {
		t.Fatalf("many matches should deny with cluster digest, got %q", out.String())
	}

	if len(recorded) != 1 {
		t.Fatalf("expected exactly one metrics entry, got %d", len(recorded))
	}
	e := recorded[0]
	if e.Tool != "Grep" {
		t.Errorf("Tool = %q, want Grep", e.Tool)
	}
	if e.CacheHit {
		t.Error("CacheHit = true, want false (Grep digests are never cached)")
	}
	if e.WorkerTokens != 555 {
		t.Errorf("WorkerTokens = %d, want 555 (what the worker reported)", e.WorkerTokens)
	}
	if e.SavedEst != e.OrigTokensEst-e.DigestTokensEst {
		t.Errorf("SavedEst = %d, want orig-digest = %d", e.SavedEst, e.OrigTokensEst-e.DigestTokensEst)
	}
}

func TestGrepHook_RgMissing_DegradesOpen(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.CountMatches = func(hookio.GrepInput) (int, error) { return 0, context.DeadlineExceeded }
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("rg failure must degrade open")
	}
}

// TestGrepHook_SampleFails_DegradesOpen proves a failed sample pass is surfaced
// rather than swallowed: the worker must never be handed an empty prompt.
func TestGrepHook_SampleFails_DegradesOpen(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.CountMatches = func(hookio.GrepInput) (int, error) { return 500, nil }
	d.Sample = func(hookio.GrepInput) (string, error) { return "", context.DeadlineExceeded }
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, int, error) {
		t.Fatal("worker must not run when the sample could not be gathered")
		return nil, 0, nil
	}
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("sample failure must degrade open, got %q", out.String())
	}
}

// TestGrepHook_BoundedOutputModes_Allow covers the shapes of Grep whose result
// size is already capped by Claude Code, whatever the match count: counting,
// listing filenames, and an explicit small head_limit. None should cost an rg
// pass, let alone a worker call.
func TestGrepHook_BoundedOutputModes_Allow(t *testing.T) {
	cases := map[string]string{
		"count":              `"pattern":"foo","output_mode":"count"`,
		"files_with_matches": `"pattern":"foo","output_mode":"files_with_matches"`,
		"small head_limit":   `"pattern":"foo","output_mode":"content","head_limit":20`,
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			d := baseDeps(t, &out)
			d.CountMatches = func(hookio.GrepInput) (int, error) {
				t.Fatal("an already-bounded Grep must not be counted")
				return 0, nil
			}
			if err := GrepHook(context.Background(), grepInputRaw(t, fields), d); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Fatalf("bounded Grep should allow, got %q", out.String())
			}
		})
	}
}

// TestGrepHook_LargeHeadLimit_StillIntercepted guards the boundary: a head_limit
// above the threshold does not bound the result usefully, so interception stands.
func TestGrepHook_LargeHeadLimit_StillIntercepted(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	counted := false
	d.CountMatches = func(hookio.GrepInput) (int, error) { counted = true; return 5, nil }
	if err := GrepHook(context.Background(),
		grepInputRaw(t, `"pattern":"foo","output_mode":"content","head_limit":5000`), d); err != nil {
		t.Fatal(err)
	}
	if !counted {
		t.Fatal("head_limit above grep_max_matches should not skip interception")
	}
}
