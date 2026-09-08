package hookio

import (
	"bytes"
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
