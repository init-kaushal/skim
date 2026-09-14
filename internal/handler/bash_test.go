package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/hookio"
)

func bashInput(t *testing.T, cmd string) hookio.Input {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]string{"command": cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	in, err := hookio.Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// denyReason runs BashHook and returns the permissionDecisionReason it emitted,
// or "" if the command was allowed.
func denyReason(t *testing.T, cmd string) string {
	t.Helper()
	var out bytes.Buffer
	d := baseDeps(t, &out)
	if err := BashHook(context.Background(), bashInput(t, cmd), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		return ""
	}
	var dec struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &dec); err != nil {
		t.Fatalf("deny output is not valid JSON: %v (%q)", err, out.String())
	}
	if dec.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("decision = %q, want deny", dec.HookSpecificOutput.PermissionDecision)
	}
	return dec.HookSpecificOutput.PermissionDecisionReason
}

// suggestedCommand pulls the indented command line out of a deny reason.
func suggestedCommand(t *testing.T, reason string) string {
	t.Helper()
	for _, line := range strings.Split(reason, "\n") {
		if strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no suggested command found in reason %q", reason)
	return ""
}

func TestBashHook_NoisyCommand_DeniesWithSkimRun(t *testing.T) {
	got := suggestedCommand(t, denyReason(t, "cat huge.log"))
	if !strings.HasSuffix(got, "run -- cat huge.log") {
		t.Fatalf("noisy command should be redirected to skim run, got %q", got)
	}
}

func TestBashHook_QuietCommand_Allows(t *testing.T) {
	if r := denyReason(t, "git status"); r != "" {
		t.Fatalf("quiet command should allow, got reason %q", r)
	}
}

// TestBashHook_SuggestionIsAbsolutePath is C2: skim is not on PATH, so a bare
// `skim run --` suggestion would fail with "command not found". The suggested
// command must name the running binary by absolute path.
func TestBashHook_SuggestionIsAbsolutePath(t *testing.T) {
	got := suggestedCommand(t, denyReason(t, "cat huge.log"))
	bin := strings.Fields(got)[0]
	if !filepath.IsAbs(bin) {
		t.Fatalf("suggested binary %q is not an absolute path (reason: %q)", bin, got)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("suggested binary %q does not exist: %v", bin, err)
	}
}

// TestBashHook_SuggestionIsNotItselfDenied is C1, the escalation bug: the deny
// reason embeds the original command, so without a skim-command guard the
// suggested replacement re-matches the same pattern and is denied again —
// forever. Feed the suggestion straight back in and require an allow.
func TestBashHook_SuggestionIsNotItselfDenied(t *testing.T) {
	for _, cmd := range []string{"cat huge.log", "go test ./...", "npm run test"} {
		t.Run(cmd, func(t *testing.T) {
			suggestion := suggestedCommand(t, denyReason(t, cmd))
			if r := denyReason(t, suggestion); r != "" {
				t.Fatalf("skim's own suggestion %q was denied again:\n%s", suggestion, r)
			}
		})
	}
}

// TestBashHook_SkimCatAllowed proves `skim cat <path>` — the Read digest's
// escape hatch — is not eaten by the \bcat\s noisy pattern.
func TestBashHook_SkimCatAllowed(t *testing.T) {
	for _, cmd := range []string{
		"skim cat /tmp/big.go",
		"/opt/plugins/skim/bin/skim cat /tmp/big.go",
		execPath() + " cat /tmp/big.go",
	} {
		if r := denyReason(t, cmd); r != "" {
			t.Errorf("%q should be allowed, got reason %q", cmd, r)
		}
	}
}

// TestBashHook_ShellMetacharacters_WrappedInShC is I5: `skim run --` execs argv
// directly with no shell, so suggesting `skim run -- go test ./... | tail -50`
// would make the OUTER shell pipe skim's own output — silently changing the
// command. Such commands must be wrapped explicitly.
func TestBashHook_ShellMetacharacters_WrappedInShC(t *testing.T) {
	got := suggestedCommand(t, denyReason(t, "go test ./... 2>&1 | tail -50"))
	if !strings.Contains(got, `run -- sh -c '`) {
		t.Fatalf("piped command should be wrapped in sh -c, got %q", got)
	}
	if !strings.HasSuffix(got, `sh -c 'go test ./... 2>&1 | tail -50'`) {
		t.Fatalf("wrapped command body is wrong: %q", got)
	}
}

// TestBashHook_SingleQuoteEscaping proves an embedded single quote survives the
// sh -c wrapping intact.
func TestBashHook_SingleQuoteEscaping(t *testing.T) {
	got := suggestedCommand(t, denyReason(t, `go test ./... | grep 'it'\''s'`))
	// POSIX '\'' escaping: the body must be one shell word with no bare quote.
	if !strings.Contains(got, `'\''`) {
		t.Fatalf("single quotes should be POSIX-escaped, got %q", got)
	}
	if r := denyReason(t, got); r != "" {
		t.Fatalf("escaped suggestion should not be denied again: %s", r)
	}
}

func TestShellSingleQuote(t *testing.T) {
	cases := map[string]string{
		"plain":     `'plain'`,
		"it's":      `'it'\''s'`,
		`a'b'c`:     `'a'\''b'\''c'`,
		"":          `''`,
		"a | b > c": `'a | b > c'`,
	}
	for in, want := range cases {
		if got := shellSingleQuote(in); got != want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
