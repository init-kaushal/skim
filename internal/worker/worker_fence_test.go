package worker

import (
	"context"
	"testing"
	"time"

	"github.com/kaushal/skim/internal/digest"
)

// TestStripCodeFence_Unfenced_Untouched verifies plain (unfenced) JSON is
// returned trimmed but otherwise byte-identical.
func TestStripCodeFence_Unfenced_Untouched(t *testing.T) {
	in := `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`
	got := stripCodeFence(in)
	if got != in {
		t.Fatalf("got %q, want %q", got, in)
	}
}

// TestStripCodeFence_Unfenced_TrimsWhitespace verifies surrounding whitespace
// on an unfenced result is trimmed.
func TestStripCodeFence_Unfenced_TrimsWhitespace(t *testing.T) {
	in := "  \n" + `{"summary":"ok","map":[]}` + "\n  "
	want := `{"summary":"ok","map":[]}`
	got := stripCodeFence(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestStripCodeFence_JSONLangTag verifies a ```json ... ``` fence is stripped
// down to the clean inner JSON.
func TestStripCodeFence_JSONLangTag(t *testing.T) {
	in := "```json\n" + `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}` + "\n```"
	want := `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`
	got := stripCodeFence(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestStripCodeFence_PlainFence verifies a fence with no language tag
// (``` ... ```) is also stripped.
func TestStripCodeFence_PlainFence(t *testing.T) {
	in := "```\n" + `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}` + "\n```"
	want := `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`
	got := stripCodeFence(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestStripCodeFence_OnlyOuterFenceStripped verifies that only the OUTER
// fence delimiters are removed, not any internal triple-backtick sequences
// that might (unusually) appear inside the JSON string value itself.
func TestStripCodeFence_OnlyOuterFenceStripped(t *testing.T) {
	in := "```json\n" + `{"summary":"has a ` + "```" + ` inside","map":[{"lines":"1","kind":"x"}]}` + "\n```"
	want := `{"summary":"has a ` + "```" + ` inside","map":[{"lines":"1","kind":"x"}]}`
	got := stripCodeFence(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestStripCodeFence_LoneFence_LeftAsIs verifies a lone ``` with nothing
// after it is left as-is (the caller will fail to parse and degrade open,
// rather than the helper panicking or corrupting it further).
func TestStripCodeFence_LoneFence_LeftAsIs(t *testing.T) {
	in := "```"
	got := stripCodeFence(in)
	if got != in {
		t.Fatalf("got %q, want %q", got, in)
	}
}

// TestRun_FencedResult_StripsFence is an integration-level test proving Run
// plumbs env.Result through the fence-stripper before returning it. The fake
// claude stub requires the fixture file to be single-line (no embedded
// newlines), so this uses a fence collapsed onto one line:
// ```json{...}``` with no newlines at all.
func TestRun_FencedResult_StripsFence(t *testing.T) {
	inner := `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`
	fenced := "```json" + inner + "```"
	withFakeClaude(t, "ok", fenced)

	got, err := Run(context.Background(), Request{
		Model: "claude-haiku-4-5-20251001", Kind: KindFileMap,
		Content: "package main", Meta: "/x/main.go", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != inner {
		t.Fatalf("got %q, want %q", got, inner)
	}

	// Downstream check: digest.ParseFileMap must succeed on the cleaned bytes.
	if _, err := digest.ParseFileMap(got); err != nil {
		t.Fatalf("ParseFileMap on stripped result: %v", err)
	}
}
