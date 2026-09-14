package hookio

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseAndReadAccessor(t *testing.T) {
	f, _ := os.Open("testdata/read_input.json")
	defer f.Close()

	in, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if in.ToolName != "Read" {
		t.Fatalf("ToolName = %q", in.ToolName)
	}
	ri, err := in.Read()
	if err != nil {
		t.Fatal(err)
	}
	if ri.FilePath != "/home/u/proj/big.go" || ri.Offset != 0 || ri.Limit != 0 {
		t.Fatalf("ReadInput = %+v", ri)
	}
}

func TestParse_RejectsGarbage(t *testing.T) {
	if _, err := Parse(strings.NewReader("not json")); err == nil {
		t.Fatal("expected error")
	}
}

func TestAllow_WritesNothing(t *testing.T) {
	var b bytes.Buffer
	if err := Allow(&b); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 0 {
		t.Fatalf("Allow wrote %q, want nothing", b.String())
	}
}

func TestDeny_WritesDecisionJSON(t *testing.T) {
	var b bytes.Buffer
	if err := Deny(&b, "use offset/limit"); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, want := range []string{
		`"permissionDecision":"deny"`,
		`"permissionDecisionReason":"use offset/limit"`,
		`"hookEventName":"PreToolUse"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Deny output %q missing %q", got, want)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("Deny output should end with newline")
	}
}

// TestDeny_EscapesQuotesAndNewlines round-trips a reason containing both a
// double quote and newlines. Deny reasons routinely carry multi-line suggested
// shell commands (which are single-quoted and may embed quotes), so a naive
// concatenation here would emit malformed JSON and break the hook contract.
func TestDeny_EscapesQuotesAndNewlines(t *testing.T) {
	reason := "skim: matched /\\bcat\\s/\nRun this instead:\n\n  /p/bin/skim run -- sh -c 'echo \"hi\" | tail -5'\n\ttabbed\\backslash\n"

	var b bytes.Buffer
	if err := Deny(&b, reason); err != nil {
		t.Fatal(err)
	}

	var got decision
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("Deny emitted invalid JSON: %v\n%s", err, b.String())
	}
	if got.HookSpecificOutput.PermissionDecisionReason != reason {
		t.Errorf("reason did not round-trip:\n got %q\nwant %q",
			got.HookSpecificOutput.PermissionDecisionReason, reason)
	}
	if got.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q, want deny", got.HookSpecificOutput.PermissionDecision)
	}
	// A raw newline inside a JSON string would be a syntax error; the encoder
	// must have escaped them.
	if strings.Contains(strings.TrimSuffix(b.String(), "\n"), "\n") {
		t.Error("decision JSON contains a raw newline inside the payload")
	}
}
