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
	"github.com/kaushal/skim/internal/worker"
)

// Deps holds the collaborators Run needs. Every field is injectable so tests can
// drive each branch without a real worker. It is a distinct type from
// handler.Deps and shares nothing with it.
type Deps struct {
	// Summarize returns the raw digest JSON and the tokens the call billed.
	// Usually worker.Run; the token count is unused here (only hook
	// interceptions are metered) but keeps one signature across both callers.
	Summarize  func(ctx context.Context, req worker.Request) ([]byte, int, error)
	Model      string
	TimeoutSec int
	Now        func() time.Time
	Stdout     io.Writer
	RunsDir    string
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

	raw, _, werr := d.Summarize(ctx, worker.Request{
		Model:   d.Model,
		Kind:    worker.KindRun,
		Content: tail,
		Meta:    strings.Join(argv, " "),
		Timeout: timeout,
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
	fmt.Fprint(d.Stdout, digest.RenderRun(r))
	return nil
}

// ringBuffer is a byte sink that retains only the last cap bytes written to it.
// It is written from a single goroutine (the os/exec output pump) so it needs no
// locking.
type ringBuffer struct {
	cap int
	buf []byte
}

// Write appends p and trims the buffer back down to the last cap bytes. It never
// reports a short write, so it is safe as one leg of an io.MultiWriter.
func (r *ringBuffer) Write(p []byte) (int, error) {
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.cap {
		r.buf = r.buf[len(r.buf)-r.cap:]
	}
	return len(p), nil
}

func (r *ringBuffer) String() string { return string(r.buf) }
