package handler

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/worker"
)

func grepInput(t *testing.T, pattern string) hookio.Input {
	t.Helper()
	in, err := hookio.Parse(bytes.NewReader([]byte(
		`{"hook_event_name":"PreToolUse","tool_name":"Grep","tool_input":{"pattern":"` + pattern + `","output_mode":"content"}}`)))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestGrepHook_FewMatches_Allows(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.CountMatches = func(hookio.GrepInput) (int, string, error) { return 5, "sample", nil }
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("few matches should allow, got %q", out.String())
	}
}

func TestGrepHook_ManyMatches_DeniesWithClusters(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.CountMatches = func(hookio.GrepInput) (int, string, error) { return 500, "big sample", nil }
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, error) {
		return []byte(`{"summary":"scattered","clusters":[{"where":"a/","matches":300,"gist":"x"}],"total":500,"representative_files":["a/x.go"]}`), nil
	}
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !strings.Contains(out.String(), "scattered") {
		t.Fatalf("many matches should deny with cluster digest, got %q", out.String())
	}
}

func TestGrepHook_RgMissing_DegradesOpen(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.CountMatches = func(hookio.GrepInput) (int, string, error) { return 0, "", context.DeadlineExceeded }
	if err := GrepHook(context.Background(), grepInput(t, "foo"), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("rg failure must degrade open")
	}
}
