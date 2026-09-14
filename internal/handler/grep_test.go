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
	d.Sample = func(hookio.GrepInput) (string, int, error) {
		t.Fatal("sample must not be gathered for a Grep under the threshold")
		return "", 0, nil
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
	d.Sample = func(hookio.GrepInput) (string, int, error) { return "big sample", len("big sample"), nil }
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, worker.Usage, error) {
		return []byte(`{"summary":"scattered","clusters":[{"where":"a/","matches":300,"gist":"x"}],"total":500,"representative_files":["a/x.go"]}`), worker.Usage{InputTokens: 555, CostUSD: 0.002}, nil
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
	// nil, not false: the Grep path has no digest cache, and recording it as a
	// miss is what made the reported cache hit rate meaningless (one Read plus
	// three Bash interceptions read "0 hit / 4 miss").
	if e.CacheHit != nil {
		t.Errorf("CacheHit = %v, want nil (the Grep path has no cache)", *e.CacheHit)
	}
	if e.WorkerTokens != 555 {
		t.Errorf("WorkerTokens = %d, want 555 (what the worker reported)", e.WorkerTokens)
	}
	// The cost, not the token sum, is what the ledger reasons about.
	if e.WorkerCostUSD != 0.002 {
		t.Errorf("WorkerCostUSD = %v, want 0.002", e.WorkerCostUSD)
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
	d.Sample = func(hookio.GrepInput) (string, int, error) { return "", 0, context.DeadlineExceeded }
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, worker.Usage, error) {
		t.Fatal("worker must not run when the sample could not be gathered")
		return nil, worker.Usage{}, nil
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
		// Claude Code itself defaults output_mode to "files_with_matches" when
		// the model omits the key entirely, so PreToolUse sees OutputMode == ""
		// for that call and must treat it the same as the explicit default.
		"omitted output_mode": `"pattern":"foo"`,
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

// TestGrepHook_SavingMeasuredAgainstFullOutput is the regression guard for a
// saving measured against the wrong thing. origEst came from len(sample) — but
// sample is skim's own prompt input, capped at ~32KB — so the ledger recorded
// skim shrinking its own prompt rather than shrinking the tool result the model
// would have received. The baseline has to be the uncapped rg output.
func TestGrepHook_SavingMeasuredAgainstFullOutput(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	var recorded []metrics.Entry
	captureEntry(&d, &recorded)

	const sample = "a handful of matching lines"
	const fullBytes = 512 * 1024 // the real rg output was far larger than the sample

	d.CountMatches = func(hookio.GrepInput) (int, error) { return 5000, nil }
	d.Sample = func(hookio.GrepInput) (string, int, error) { return sample, fullBytes, nil }
	var gotReq worker.Request
	d.Summarize = func(_ context.Context, req worker.Request) ([]byte, worker.Usage, error) {
		gotReq = req
		return []byte(`{"summary":"scattered widely","clusters":[{"where":"internal/","matches":5000,"gist":"x"}],"total":5000}`),
			worker.Usage{InputTokens: 10, CostUSD: 0.004}, nil
	}

	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 {
		t.Fatalf("recorded %d entries, want 1", len(recorded))
	}
	e := recorded[0]

	if want := metrics.EstimateTokens(fullBytes); e.OrigTokensEst != want {
		t.Errorf("OrigTokensEst = %d, want %d (the full rg output, not the %d-byte sample)",
			e.OrigTokensEst, want, len(sample))
	}
	// The old behaviour would have produced a tiny — and in this case negative —
	// saving, because the sample is smaller than the rendered digest.
	if e.OrigTokensEst <= metrics.EstimateTokens(len(sample)) {
		t.Error("baseline must exceed the capped sample, or it is measuring skim's own prompt")
	}
	if e.SavedEst <= 0 {
		t.Errorf("SavedEst = %d, want a large positive saving on 512KB of matches", e.SavedEst)
	}
	// The worker only saw a slice of the matches, so it must be told.
	if !gotReq.Partial {
		t.Error("Partial should be set when the sample is a prefix of the match output")
	}
}
