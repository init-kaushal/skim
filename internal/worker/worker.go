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

// Run execs `claude -p --model <Model> --output-format json --max-turns 1`,
// feeds it promptFor(req) on stdin with SKIM_ACTIVE=1 added to the child env,
// and parses the `{"result": "<string>"}` envelope from stdout, returning the
// inner result string as bytes.
//
// A timeout yields an error satisfying errors.Is(err, ErrTimeout); a non-zero
// exit yields an error including stderr; an unparseable or empty envelope
// yields an error.
func Run(ctx context.Context, req Request) ([]byte, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "claude", "-p",
		"--model", req.Model, "--output-format", "json", "--max-turns", "1")
	cmd.Env = append(os.Environ(), "SKIM_ACTIVE=1")
	cmd.Stdin = bytes.NewReader([]byte(promptFor(req)))

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%w after %s", ErrTimeout, req.Timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("worker: claude failed: %w (stderr: %s)", err, stderr.String())
	}

	var env struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("worker: bad claude envelope: %w", err)
	}
	if env.Result == "" {
		return nil, errors.New("worker: empty result")
	}
	return []byte(env.Result), nil
}
