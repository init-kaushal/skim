package handler

import (
	"context"
	"io"
	"time"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

// Deps holds the collaborators ReadHook (and future hooks) need. Every field is
// injectable so tests can drive each branch without real I/O. The zero value is
// not usable; callers wire the "usually" implementations noted per field.
type Deps struct {
	Cfg      config.Config
	Env      func(string) string                       // usually os.Getenv
	CacheKey func(path, model string) (string, error)  // usually cache.Key
	CacheGet func(key string) (digest.FileMap, bool)   // usually cache.Get
	CachePut func(key string, fm digest.FileMap) error // usually cache.Put
	Record   func(metrics.Entry)                       // usually func(e){ _ = metrics.Record(e) }
	Logf     func(format string, args ...any)          // appends to skim.log
	Now      func() time.Time
	Stdout   io.Writer

	// Summarize runs one nested worker call, returning the raw digest JSON and
	// what the call billed. Usually worker.Run.
	Summarize func(ctx context.Context, req worker.Request) (result []byte, u worker.Usage, err error)

	// CountMatches reports how many Grep hits gi would produce. Usually backed
	// by rg; an error (e.g. rg not installed) degrades open.
	CountMatches func(gi hookio.GrepInput) (total int, err error)

	// Sample returns a capped (~32 KB) rg context dump for gi, used as the
	// worker input, plus fullBytes: how large that output was BEFORE capping.
	// fullBytes is the honest baseline for what the Grep would have put in the
	// context — the capped sample is skim's own input, and measuring savings
	// against it credited skim with shrinking its own prompt rather than the
	// tool result it replaced. It is deliberately separate from CountMatches:
	// only a Grep that clears the threshold needs a sample, and gathering one
	// costs a second full rg pass. An error degrades open, same as CountMatches.
	Sample func(gi hookio.GrepInput) (sample string, fullBytes int, err error)
}

// applyUsage copies a worker call's billing into a metrics entry. Kept in one
// place so every intercepting path records the same fields: the per-tier counts
// are what make the cost figure auditable after the fact, and the flat sum is
// retained only as a size diagnostic.
func applyUsage(e *metrics.Entry, u worker.Usage) {
	e.WorkerTokens = u.Tokens()
	e.WorkerInputTokens = u.InputTokens
	e.WorkerOutputTokens = u.OutputTokens
	e.WorkerCacheWriteTokens = u.CacheWriteTokens
	e.WorkerCacheReadTokens = u.CacheReadTokens
	e.WorkerCostUSD = u.CostUSD
}
