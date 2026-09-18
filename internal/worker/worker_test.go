package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withFakeClaude(t *testing.T, mode, resultJSON string) {
	t.Helper()
	repoRoot, _ := filepath.Abs("../..")
	t.Setenv("PATH", filepath.Join(repoRoot, "testdata", "fakeclaude")+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE_MODE", mode)
	if resultJSON != "" {
		f := filepath.Join(t.TempDir(), "result.json")
		os.WriteFile(f, []byte(resultJSON), 0o644)
		t.Setenv("FAKE_CLAUDE_RESULT_FILE", f)
	}
}

func TestRun_OK_ReturnsResultBytes(t *testing.T) {
	want := `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`
	withFakeClaude(t, "ok", want)

	got, _, err := Run(context.Background(), Request{
		Model: "claude-haiku-4-5-20251001", Kind: KindFileMap,
		Content: "package main", Meta: "/x/main.go", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRun_NonZeroExit_Errors(t *testing.T) {
	withFakeClaude(t, "exit1", "")
	if _, _, err := Run(context.Background(), Request{Model: "m", Kind: KindRun, Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}

func TestRun_Timeout(t *testing.T) {
	withFakeClaude(t, "hang", "")
	_, _, err := Run(context.Background(), Request{Model: "m", Kind: KindRun, Timeout: 200 * time.Millisecond})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestRun_GarbageEnvelope_Errors(t *testing.T) {
	withFakeClaude(t, "garbage", "")
	if _, _, err := Run(context.Background(), Request{Model: "m", Kind: KindRun, Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected error on unparseable envelope")
	}
}

// TestRun_ReportsUsageTokens proves Run surfaces the `usage` block from the
// claude -p JSON envelope, which is the cost side of the skim stats ledger.
func TestRun_ReportsUsageTokens(t *testing.T) {
	withFakeClaude(t, "usage", `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`)

	_, use, err := Run(context.Background(), Request{
		Model: "m", Kind: KindFileMap, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The fake reports input 11 + output 22 + cache_creation 33 + cache_read 44.
	// Each is kept separately now, because they bill at different rates: summing
	// them and subtracting from tokens-saved compared unlike units.
	if use.InputTokens != 11 || use.OutputTokens != 22 ||
		use.CacheWriteTokens != 33 || use.CacheReadTokens != 44 {
		t.Fatalf("per-tier usage = %+v, want 11/22/33/44", use)
	}
	if use.Tokens() != 110 {
		t.Fatalf("Tokens() = %d, want 110", use.Tokens())
	}
	// The dollar figure is taken from the CLI rather than computed from the
	// counts above, so that a price change upstream cannot silently make skim's
	// reported cost wrong.
	if use.CostUSD != 0.0425 {
		t.Fatalf("CostUSD = %v, want 0.0425 (the fake's total_cost_usd)", use.CostUSD)
	}
}

// TestRun_MissingUsage_TokensZeroNotError proves an envelope with no `usage`
// block still succeeds: the token count is a stats nicety and must never become
// a new degrade-open trigger.
func TestRun_MissingUsage_TokensZeroNotError(t *testing.T) {
	withFakeClaude(t, "ok", `{"summary":"ok","map":[{"lines":"1-2","kind":"x"}]}`)

	_, use, err := Run(context.Background(), Request{
		Model: "m", Kind: KindFileMap, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("missing usage must not error: %v", err)
	}
	if use.Tokens() != 0 || use.CostUSD != 0 {
		t.Fatalf("usage = %+v, want zero", use)
	}
}

// NOTE: the recursion guard (SKIM_ACTIVE=1 in the child env) is asserted by
// TestReadHook_RecursionGuard_Allows in internal/handler and by
// TestIntegration_ReadHook_RecursionGuard in cmd/skim. A worker-level test that
// only checked Run returned no error asserted nothing and has been removed.

// TestRunCLI_PassesMinimalSystemPrompt guards the CLI transport's largest cost
// saving. Claude Code's default system prompt is ~5.5K tokens, billed as a
// 1-hour cache write at 2x input, and the CLI is the transport every
// subscription install uses — there is no API key there to switch to the direct
// path, so replacing the system prompt is the only lever available. Dropping
// this flag would silently restore ~4,210 input tokens per interception.
func TestRunCLI_PassesMinimalSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")

	// A stand-in `claude` that records its own argv, then answers like the real
	// CLI so runCLI gets a parseable envelope. The reply is written with a
	// quoted heredoc so the shell does no substitution on the JSON.
	const stub = `#!/bin/sh
printf '%s\n' "$@" > ARGS_FILE
cat > /dev/null
cat <<'JSON'
{"type":"result","result":"{\"summary\":\"s\",\"map\":[{\"lines\":\"1-2\",\"kind\":\"k\"}]}"}
JSON
`
	script := strings.Replace(stub, "ARGS_FILE", argsFile, 1)
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend rather than replace: the stub itself shells out, and the existing
	// withFakeClaude helper prepends for the same reason.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, _, err := runCLI(context.Background(), Request{
		Model: "m", Kind: KindFileMap, Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimRight(string(got), "\n"), "\n")

	idx := -1
	for i, a := range args {
		if a == "--system-prompt" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("--system-prompt not passed; argv was %v", args)
	}
	if idx+1 >= len(args) || args[idx+1] != cliSystemPrompt {
		t.Errorf("--system-prompt value = %q, want %q", args[idx+1], cliSystemPrompt)
	}
	// The prompt must be short: its whole purpose is to displace a large one.
	if len(cliSystemPrompt) > 200 {
		t.Errorf("cliSystemPrompt is %d bytes — too long to be saving anything", len(cliSystemPrompt))
	}
	// --tools "" must survive alongside it; it is what keeps the worker from
	// touching the filesystem and halves the prompt again.
	if !strings.Contains(strings.Join(args, " "), "--tools") {
		t.Errorf("--tools dropped; argv was %v", args)
	}
}
