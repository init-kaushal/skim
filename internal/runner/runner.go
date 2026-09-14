// Package runner backs `skim run -- <cmd>`: it executes a command with its
// combined stdout+stderr streamed to a log file, keeps the tail in a capped ring
// buffer, and asks the nested worker to reduce that tail to a digest. The command
// always runs to completion and `skim run` itself exits 0 — the child's exit code
// is reported inside the digest, never propagated.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

// Deps holds the collaborators Run needs. Every field is injectable so tests can
// drive each branch without a real worker. It is a distinct type from
// handler.Deps and shares nothing with it.
type Deps struct {
	// Summarize returns the raw digest JSON and what the call billed.
	// Usually worker.Run.
	Summarize  func(ctx context.Context, req worker.Request) ([]byte, worker.Usage, error)
	Model      string
	TimeoutSec int
	Now        func() time.Time
	Stdout     io.Writer
	RunsDir    string

	// Record logs one interception to the metrics ledger. `skim run` is where
	// the Bash path's tokens are actually saved and its worker cost actually
	// incurred, so without this the entire Bash side of `skim stats` was blank:
	// the bash-hook's own entry carries only a timestamp. Usually
	// func(e){ _ = metrics.Record(e) }; a nil Record is tolerated so callers
	// that only want the digest need not wire it.
	Record func(metrics.Entry)
}

// tailCap bounds the ring buffer that feeds the worker: the last ~16 KB of
// output is plenty to summarise a run, and the full stream is on disk anyway.
const tailCap = 16 * 1024

// Run executes argv (command + already-split args), capturing combined output to
// a log under d.RunsDir and printing a digest to d.Stdout. It returns nil
// whenever the child ran to completion — including on a non-zero child exit or a
// worker failure. Only an empty argv or a failure to create the log file returns
// an error.
func Run(ctx context.Context, argv []string, d Deps) error {
	if len(argv) == 0 {
		return fmt.Errorf("skim run: no command given")
	}

	if err := os.MkdirAll(d.RunsDir, 0o755); err != nil {
		return err
	}

	idb := make([]byte, 6)
	if _, err := rand.Read(idb); err != nil {
		return fmt.Errorf("skim run: generate run id: %w", err)
	}
	logPath := filepath.Join(d.RunsDir, hex.EncodeToString(idb)+".log")

	logf, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logf.Close()

	ring := &ringBuffer{cap: tailCap}
	sink := io.MultiWriter(logf, ring)

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = sink
	cmd.Stderr = sink
	// Errors from the child (non-zero exit, signal, even failure to start) are
	// intentionally swallowed: skim run reports status through the digest.
	_ = cmd.Run()
	exit := cmd.ProcessState.ExitCode()

	tail := ring.String()

	fallback := func() {
		if tail != "" {
			fmt.Fprintf(d.Stdout, "%s\n", tail)
		}
		fmt.Fprintf(d.Stdout, "Full output: %s\nexit %d\n", logPath, exit)
	}

	var timeout time.Duration
	if d.TimeoutSec > 0 {
		timeout = time.Duration(d.TimeoutSec) * time.Second
	}

	// full is everything the command emitted; tail is only the last tailCap of
	// it. Both matter: full is the baseline the saving is measured against,
	// tail is what the worker actually sees.
	full := ring.Total()

	raw, use, werr := d.Summarize(ctx, worker.Request{
		Model:   d.Model,
		Kind:    worker.KindRun,
		Content: tail,
		Meta:    strings.Join(argv, " "),
		Timeout: timeout,
		Partial: full > len(tail),
	})
	if werr != nil {
		fallback()
		return nil
	}

	r, perr := digest.ParseRun(raw)
	if perr != nil {
		fallback()
		return nil
	}

	r.LogPath = logPath
	r.ExitCode = exit
	// Tell the model when the digest only saw the tail. Without this the digest
	// read as a summary of the whole run: `cat`ing a large file through
	// `skim run` described only its final 16KB, with nothing saying so.
	out := digest.RenderRun(r, digest.Coverage{
		SeenBytes: len(tail), TotalBytes: full,
	})
	fmt.Fprint(d.Stdout, out)
	d.record(full, len(out), use)
	return nil
}

// record writes this run's entry to the metrics ledger. origBytes is everything
// the command emitted — the honest baseline, since that is what would have gone
// into the context had the Bash call been allowed through.
//
// Caveat worth knowing when reading `skim stats`: Claude Code's Bash tool
// truncates very long output by an amount skim does not model, so for enormous
// output this baseline is an upper bound rather than an exact counterfactual.
// It is still far better than the previous behaviour, which recorded nothing at
// all and left the Bash path's savings and cost entirely invisible.
func (d Deps) record(origBytes, digestBytes int, use worker.Usage) {
	if d.Record == nil {
		return
	}
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	origEst := metrics.EstimateTokens(origBytes)
	digEst := metrics.EstimateTokens(digestBytes)
	e := metrics.Entry{
		TS:              now().UTC().Format(time.RFC3339),
		Tool:            "Run",
		OrigTokensEst:   origEst,
		DigestTokensEst: digEst,
		SavedEst:        origEst - digEst,
		// CacheHit stays nil: `skim run` has no digest cache — the same command
		// run twice can legitimately produce different output.
		WorkerTokens:           use.Tokens(),
		WorkerInputTokens:      use.InputTokens,
		WorkerOutputTokens:     use.OutputTokens,
		WorkerCacheWriteTokens: use.CacheWriteTokens,
		WorkerCacheReadTokens:  use.CacheReadTokens,
		WorkerCostUSD:          use.CostUSD,
	}
	d.Record(e)
}

// ringBuffer is a byte sink that retains only the last cap bytes written to it,
// while counting every byte that passes through. It is written from a single
// goroutine (the os/exec output pump) so it needs no locking.
type ringBuffer struct {
	cap   int
	buf   []byte
	total int
}

// Write appends p and trims the buffer back down to the last cap bytes. It never
// reports a short write, so it is safe as one leg of an io.MultiWriter.
func (r *ringBuffer) Write(p []byte) (int, error) {
	r.total += len(p)
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.cap {
		r.buf = r.buf[len(r.buf)-r.cap:]
	}
	return len(p), nil
}

// Total is every byte the command emitted, including what the ring dropped.
// Counted here rather than stat'ed off the log file so it needs no syscall and
// makes no assumption about when the log's writes have landed.
func (r *ringBuffer) Total() int { return r.total }

func (r *ringBuffer) String() string { return string(r.buf) }
