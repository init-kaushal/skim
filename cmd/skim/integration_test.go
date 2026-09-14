package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegration_ReadHook_LargeFile_EmitsDeny drives the whole read-hook path
// end to end: real config, real cache, real metrics, a real nested worker exec
// against testdata/fakeclaude, and a real deny decision on stdout.
func TestIntegration_ReadHook_LargeFile_EmitsDeny(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SKIM_HOME", home)

	repoRoot, _ := filepath.Abs("../..")
	t.Setenv("PATH", filepath.Join(repoRoot, "testdata", "fakeclaude")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE_MODE", "ok")
	resFile := filepath.Join(home, "res.json")
	if err := os.WriteFile(resFile, []byte(`{"summary":"a big file","map":[{"lines":"1-9000","kind":"repeated x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CLAUDE_RESULT_FILE", resFile)

	big := filepath.Join(home, "big.go")
	if err := os.WriteFile(big, []byte(strings.Repeat("x\n", 9000)), 0o644); err != nil {
		t.Fatal(err)
	}

	stdinJSON := `{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"` + big + `","offset":0,"limit":0}}`

	var out, errb bytes.Buffer
	code := runWithStdin(strings.NewReader(stdinJSON), []string{"read-hook"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errb.String())
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !strings.Contains(out.String(), "a big file") {
		t.Fatalf("expected deny with digest, got: %q", out.String())
	}
}

// TestIntegration_ReadHook_RecursionGuard proves SKIM_ACTIVE=1 short-circuits
// the hook: exit 0, nothing written to stdout, no nested worker spawned.
func TestIntegration_ReadHook_RecursionGuard(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	t.Setenv("SKIM_ACTIVE", "1")
	stdinJSON := `{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"/anything","offset":0,"limit":0}}`
	var out, errb bytes.Buffer
	if code := runWithStdin(strings.NewReader(stdinJSON), []string{"read-hook"}, &out, &errb); code != 0 || out.Len() != 0 {
		t.Fatalf("guard: code=%d out=%q", code, out.String())
	}
}
