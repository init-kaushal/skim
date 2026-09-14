// Package worker runs a nested `claude -p` process, pinned to a caller-chosen
// model, to summarise content that is too large to hand back verbatim.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Kind selects which prompt/schema promptFor builds.
type Kind int

const (
	KindFileMap Kind = iota
	KindClusters
	KindRun
)

// Request is a single summarisation job. Model and Timeout come from config via
// the caller; Content is the raw payload and Meta is the human label for it
// (a file path, a ripgrep pattern, a command line).
type Request struct {
	Model   string
	Kind    Kind
	Content string
	Meta    string
	Timeout time.Duration
}

// ErrTimeout is returned (wrapped) when the nested claude call exceeds the
// deadline. Callers should test with errors.Is(err, ErrTimeout).
var ErrTimeout = errors.New("worker: timed out")

// Run execs `claude -p --model <Model> --output-format json --max-turns 1
// --tools ""`, feeds it promptFor(req) on stdin with SKIM_ACTIVE=1 added to the
// child env, and parses the `{"result": "<string>", "usage": {…}}` envelope
// from stdout. It returns the inner result string as bytes plus the total
// tokens the worker call billed, so `skim stats` can weigh the cost of the
// digest against what it kept out of the main context.
//
// `--tools ""` disables every built-in tool, which spec §4.4 requires (the
// worker summarises text; it has no business touching the filesystem) and which
// also halves the worker's system prompt. Verified against Claude Code 2.1.263:
// with the flag the model reports it has no tools available.
//
// A timeout yields an error satisfying errors.Is(err, ErrTimeout); a non-zero
// exit yields an error including stderr; an unparseable or empty envelope
// yields an error. A missing or unparseable `usage` block is NOT an error — the
// token count is a stats nicety and degrades to 0 rather than failing the call.
func Run(ctx context.Context, req Request) (result []byte, workerTokens int, err error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "claude", "-p",
		"--model", req.Model, "--output-format", "json", "--max-turns", "1",
		"--tools", "")
	cmd.Env = append(os.Environ(), "SKIM_ACTIVE=1")
	cmd.Stdin = bytes.NewReader([]byte(promptFor(req)))

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, 0, fmt.Errorf("%w after %s", ErrTimeout, req.Timeout)
	}
	if runErr != nil {
		return nil, 0, fmt.Errorf("worker: claude failed: %w (stderr: %s)", runErr, stderr.String())
	}

	var env struct {
		Result string `json:"result"`
		Usage  struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, 0, fmt.Errorf("worker: bad claude envelope: %w", err)
	}

	// Every input variant is billed, so all of them count against the saving.
	u := env.Usage
	tokens := u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens

	body := stripCodeFence(env.Result)
	if body == "" {
		return nil, tokens, errors.New("worker: empty result")
	}
	return []byte(body), tokens, nil
}

// stripCodeFence removes a wrapping markdown code fence (```json ... ``` or
// ``` ... ```, with or without a newline separating the opening fence/language
// tag from the body) that models sometimes add despite being asked for raw
// JSON. Only the outer fence is touched — any triple-backtick sequence inside
// the body is left alone. If no wrapping fence is present, s is returned
// trimmed but otherwise unchanged. If s looks like an opening fence with no
// matching close, s is returned unchanged (untrimmed of the fence) so the
// caller fails to parse it and degrades open rather than this guessing.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	body := s[3:]

	// Drop an optional language tag (e.g. "json") immediately following the
	// opening fence: a run of identifier-ish characters up to the first
	// character that can't be part of one (JSON content always begins with
	// '{', '[', '"' or whitespace, none of which match).
	i := 0
	for i < len(body) && isFenceTagByte(body[i]) {
		i++
	}
	body = body[i:]
	body = strings.TrimPrefix(body, "\n")

	if !strings.HasSuffix(body, "```") {
		return s
	}
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimRight(body, "\n")
	return strings.TrimSpace(body)
}

func isFenceTagByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
}
