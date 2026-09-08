package handler

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/hookio"
)

func bashInput(t *testing.T, cmd string) hookio.Input {
	t.Helper()
	esc := strings.ReplaceAll(cmd, `"`, `\"`)
	in, err := hookio.Parse(bytes.NewReader([]byte(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"` + esc + `"}}`)))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestBashHook_NoisyCommand_DeniesWithSkimRun(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	if err := BashHook(context.Background(), bashInput(t, "cat huge.log"), d); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"permissionDecision":"deny"`) || !strings.Contains(got, "skim run -- cat huge.log") {
		t.Fatalf("noisy command should be redirected to skim run, got %q", got)
	}
}

func TestBashHook_QuietCommand_Allows(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	if err := BashHook(context.Background(), bashInput(t, "git status"), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("quiet command should allow, got %q", out.String())
	}
}
