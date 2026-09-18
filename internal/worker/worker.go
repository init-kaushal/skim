// Package worker runs a nested `claude -p` process, pinned to a caller-chosen
// model, to summarise content that is too large to hand back verbatim.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

	// Partial says Content is only the leading portion of the underlying
	// source, not all of it. The prompt then bounds the model to the excerpt
	// instead of asking it to map a whole file it cannot see.
	Partial bool
}

// Usage is what one worker call billed. The four token counts are kept apart
// rather than summed because they are priced differently — on Haiku 4.5,
// measured against the CLI's own reported cost, a 1-hour cache write bills at
// 2.0x the input rate, a cache read at 0.1x, and output at 5x. Adding them into
// a single "tokens" figure and subtracting it from tokens-saved, as skim did
// originally, compares quantities that are not in the same unit.
//
// CostUSD is the authoritative figure and the one `skim stats` reports. It is
// taken from the CLI's own `total_cost_usd`, deliberately not derived from a
// price table compiled into skim: prices change, and the CLI already knows the
// real number for whatever model and auth mode is in play.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
	CostUSD          float64
}

// Tokens is the flat sum of every billed token variant. It is a diagnostic for
// sizing a call, NOT a cost — use CostUSD for anything that compares against a
// saving.
func (u Usage) Tokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheWriteTokens + u.CacheReadTokens
}

// ErrTimeout is returned (wrapped) when the nested claude call exceeds the
// deadline. Callers should test with errors.Is(err, ErrTimeout).
var ErrTimeout = errors.New("worker: timed out")

// httpClient is the client the direct transport uses. It carries no Timeout of
// its own: the deadline comes from the per-call context, which Run derives from
// Request.Timeout. A client-level timeout would apply to the whole exchange and
// fight that. Package-level so connections stay warm across the several
// interceptions one session makes.
var httpClient = &http.Client{}

// transport is the seam tests use to assert which path Run chose without
// needing either real credentials or a fake CLI on PATH.
type transport int

const (
	transportDirect transport = iota
	transportCLI
)

// chooseTransport reports how this call will reach the model. Direct wins
// whenever credentials exist, because it avoids Claude Code's system prompt
// entirely; otherwise the CLI is the only thing that can authenticate.
func chooseTransport(env func(string) string) transport {
	if _, ok := credentials(env); ok {
		return transportDirect
	}
	return transportCLI
}

// Run produces one digest, over whichever transport is available.
//
// Direct HTTP to /v1/messages is preferred: it skips the ~5.5K-token Claude
// Code system prompt that every nested `claude -p` call pays for (billed as a
// 1-hour cache write at 2x input, about $0.011 per cold call before any file
// content), skips a child process, and needs no recursion guard. It requires
// ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN.
//
// Without those, Run falls back to the CLI, which is what a Claude Code
// subscription install has. Both transports return the same Request/Usage
// shapes, and both degrade open the same way, so callers do not branch.
func Run(ctx context.Context, req Request) (result []byte, u Usage, err error) {
	if chooseTransport(osEnv) == transportDirect {
		c, _ := credentials(osEnv)
		if req.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, req.Timeout)
			defer cancel()
		}
		return runDirect(ctx, req, c, baseURL(osEnv), httpClient)
	}
	return runCLI(ctx, req)
}

// cliSystemPrompt replaces Claude Code's default system prompt on the worker
// call. The default one is the CLI transport's largest fixed cost — and it is
// the transport every subscription install uses, where no API key exists to
// switch to the direct path, so this is the only way to cut it for them.
//
// Measured on a 17.5 KB file, thinking already off, same prompt and model:
//
//	default system prompt:  $0.027770  10,864 cache-write tokens
//	this system prompt:     $0.020860   6,654 cache-write tokens
//
// 4,210 fewer input tokens per call. Billed as a 1-hour cache write at 2x
// input, that is about $0.0084 saved on every interception, roughly 25% of a
// small file's digest cost.
//
// It also isolates the worker properly: with an explicit system prompt, Claude
// Code stops injecting its dynamic sections, so the project's own CLAUDE.md and
// environment details no longer leak into a call that only has to describe one
// file's structure.
const cliSystemPrompt = "You extract structure from files and command output, " +
	"and reply with raw JSON only."

// runCLI execs `claude -p --model <Model> --output-format json --max-turns 1
// --tools "" --system-prompt <minimal>`, feeds it promptFor(req) on stdin with
// SKIM_ACTIVE=1 added to the
// child env, and parses the `{"result": "<string>", "usage": {…}}` envelope
// from stdout. It returns the inner result string as bytes plus what the call
// billed, so `skim stats` can weigh the cost of the digest against what it kept
// out of the main context.
//
// This is the fallback transport. It works wherever Claude Code works —
// including on a subscription, where no API credentials exist — at the cost of
// a child process and Claude Code's own ~5.5K-token system prompt per call.
// Run prefers the direct transport when credentials allow it.
//
// `--tools ""` disables every built-in tool, which spec §4.4 requires (the
// worker summarises text; it has no business touching the filesystem) and which
// also halves the worker's system prompt. Verified against Claude Code 2.1.263:
// with the flag the model reports it has no tools available.
//
// A timeout yields an error satisfying errors.Is(err, ErrTimeout); a non-zero
// exit yields an error including stderr; an unparseable or empty envelope
// yields an error. A missing or unparseable `usage` block is NOT an error — the
// accounting is a stats nicety and degrades to a zero Usage rather than failing
// the call.
func runCLI(ctx context.Context, req Request) (result []byte, u Usage, err error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "claude", "-p",
		"--model", req.Model, "--output-format", "json", "--max-turns", "1",
		"--tools", "", "--system-prompt", cliSystemPrompt)
	// MAX_THINKING_TOKENS=0 turns off the worker's extended thinking. Producing
	// a structural digest is mechanical extraction, not reasoning, and thinking
	// dominated both the bill and the latency. Measured on a 17.5 KB file,
	// same prompt, same model:
	//
	//	thinking on:  $0.046684  31.8s  4,987 output tokens (4,369 of them thinking)
	//	thinking off: $0.016165   5.7s    502 output tokens (0 thinking)
	//
	// 65% cheaper and 5.6x faster, and the digest came back *more* accurate:
	// with thinking on, the model emitted map ranges running to line 556 of a
	// 411-line file; with it off, the last range was 410-420. `--effort low`
	// was also tried and does not reduce thinking on Haiku 4.5.
	//
	// Duplicate keys are safe here: os/exec documents that the last value for a
	// repeated key wins, so this overrides any inherited MAX_THINKING_TOKENS.
	cmd.Env = append(os.Environ(), "SKIM_ACTIVE=1", "MAX_THINKING_TOKENS=0")
	cmd.Stdin = bytes.NewReader([]byte(promptFor(req)))

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, Usage{}, fmt.Errorf("%w after %s", ErrTimeout, req.Timeout)
	}
	if runErr != nil {
		return nil, Usage{}, fmt.Errorf("worker: claude failed: %w (stderr: %s)", runErr, stderr.String())
	}

	var env struct {
		Result string `json:"result"`
		// The CLI reports the real dollar cost of the call; prefer it over
		// anything skim could compute from token counts and a stale price list.
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, Usage{}, fmt.Errorf("worker: bad claude envelope: %w", err)
	}

	use := Usage{
		InputTokens:      env.Usage.InputTokens,
		OutputTokens:     env.Usage.OutputTokens,
		CacheWriteTokens: env.Usage.CacheCreationInputTokens,
		CacheReadTokens:  env.Usage.CacheReadInputTokens,
		CostUSD:          env.TotalCostUSD,
	}

	body := stripCodeFence(env.Result)
	if body == "" {
		return nil, use, errors.New("worker: empty result")
	}
	return []byte(body), use, nil
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
