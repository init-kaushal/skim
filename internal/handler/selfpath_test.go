package handler

import (
	"strings"
	"testing"
)

// TestHasShellMeta_Newline is Minor #2: a multi-line Bash command (e.g.
// "npm run test\necho done") relies on the newline as a command separator just
// like ';' or '|' would, so it must trigger the sh -c wrapped suggestion form
// too — otherwise the outer shell re-parsing the unwrapped suggestion loses
// everything after the first line.
func TestHasShellMeta_Newline(t *testing.T) {
	cases := []string{
		"npm run test\necho done",
		"echo a\recho b",
		"echo a\r\necho b",
	}
	for _, cmd := range cases {
		if !hasShellMeta(cmd) {
			t.Errorf("hasShellMeta(%q) = false, want true (embedded newline/CR must trigger sh -c wrapping)", cmd)
		}
	}
}

// TestBashHook_EmbeddedNewline_WrappedInShC proves the newline fix end to end
// through BashHook: a multi-line noisy command must produce a deny reason
// wrapped in `sh -c '...'` that still carries every line, not just the first.
func TestBashHook_EmbeddedNewline_WrappedInShC(t *testing.T) {
	reason := denyReason(t, "npm run test\necho done")
	if reason == "" {
		t.Fatal("multi-line noisy command should be denied")
	}
	if !strings.Contains(reason, "run -- sh -c '") {
		t.Fatalf("newline command should be wrapped in sh -c, got %q", reason)
	}
	if !strings.Contains(reason, "npm run test") || !strings.Contains(reason, "echo done") {
		t.Fatalf("wrapped suggestion should contain both lines, got %q", reason)
	}
}

// TestIsSkimCommand_SpacedSelfPath is Minor #1 / a C1 regression: when the
// resolved skim binary's absolute path contains whitespace, the suggested
// command must shell-quote it (quoteSelfIfNeeded), and isSkimCommand must
// still recognize the quoted form as self-invoking — otherwise the suggestion
// gets denied again, forever.
func TestIsSkimCommand_SpacedSelfPath(t *testing.T) {
	self := "/Users/x/dir with space/bin/skim"
	for _, cmd := range []string{
		quoteSelfIfNeeded(self) + " run -- cat huge.log",
		quoteSelfIfNeeded(self) + " cat /tmp/big.go",
	} {
		if !isSkimCommand(cmd, self) {
			t.Errorf("isSkimCommand(%q, %q) = false, want true", cmd, self)
		}
	}
}

// TestIsSkimCommand_UnspacedSelfPath guards the common case still works
// unchanged: no quoting is needed or expected when self has no whitespace.
func TestIsSkimCommand_UnspacedSelfPath(t *testing.T) {
	self := "/opt/plugins/skim/bin/skim"
	if q := quoteSelfIfNeeded(self); q != self {
		t.Errorf("quoteSelfIfNeeded(%q) = %q, want unchanged (no whitespace)", self, q)
	}
	if !isSkimCommand(self+" run -- cat huge.log", self) {
		t.Error("unquoted self-invoking command should still be recognized")
	}
}

// TestBashHook_SpacedSelfPath_RoundTrips is the full C1-style regression: with
// executablePath overridden to a path containing a space, a noisy command
// denied by BashHook must produce a suggestion that, fed straight back in, is
// recognized as self-invoking and allowed — not denied again forever.
func TestBashHook_SpacedSelfPath_RoundTrips(t *testing.T) {
	orig := executablePath
	defer func() { executablePath = orig }()
	spaced := "/tmp/skim test dir/bin/skim"
	executablePath = func() (string, error) { return spaced, nil }

	reason := denyReason(t, "cat huge.log")
	if reason == "" {
		t.Fatal("noisy command should be denied")
	}
	suggestion := suggestedCommand(t, reason)
	if !strings.Contains(suggestion, shellSingleQuote(spaced)) {
		t.Fatalf("suggestion should shell-quote the spaced self path, got %q", suggestion)
	}
	if r := denyReason(t, suggestion); r != "" {
		t.Fatalf("spaced self-path suggestion %q was denied again: %s", suggestion, r)
	}
}
