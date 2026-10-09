package handler

import (
	"context"
	"io"
	"time"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/decision"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/observe"
	"github.com/kaushal/skim/internal/router"
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

	// Engine is the cost-aware routing engine. When nil, hooks fall back to
	// threshold-only decisions (backward-compatible behaviour).
	Engine *decision.Engine

	// Router is the escalation chain. When non-nil and the plan calls for a
	// worker, the router tries each tier in order and escalates when the initial
	// tier signals uncertainty. When nil, Summarize is called directly (no
	// escalation).
	Router *router.Router

	// Calibration holds compression performance history used for the telemetry
	// feedback loop: after every worker call we record the actual compression
	// ratio so future predictions improve. When nil, no feedback is recorded.
	Calibration *cost.Calibration

	// ObserveRecord logs a routing decision. Usually observe.Record.
	// When nil, decisions are not logged (no explain output, but still works).
	ObserveRecord func(observe.Entry)
}

// summarizeWithRouter calls the router when it is wired; otherwise falls back
// to d.Summarize. It returns the raw digest bytes, usage, and which strategy
// was ultimately used (may be higher than the plan's initial strategy when
// escalation fired).
func (d Deps) summarizeWithRouter(ctx context.Context, req worker.Request, planned cost.Strategy) ([]byte, worker.Usage, cost.Strategy, error) {
	if d.Router != nil {
		res, err := d.Router.Summarize(ctx, req, d.Summarize)
		if err != nil {
			return nil, worker.Usage{}, planned, err
		}
		return res.Raw, res.Usage, res.Tier.Strategy, nil
	}
	raw, u, err := d.Summarize(ctx, req)
	return raw, u, planned, err
}

// applyUsage copies a worker call's billing into a metrics entry and optionally
// records the actual compression ratio into the calibration system for the
// telemetry feedback loop. kind identifies the compressor (e.g. "go_test") or
// "" for a generic worker call. workerInputTokens is the content-only token
// count (before system overhead) used to compute the ratio.
func applyUsage(e *metrics.Entry, u worker.Usage) {
	e.WorkerTokens = u.Tokens()
	e.WorkerInputTokens = u.InputTokens
	e.WorkerOutputTokens = u.OutputTokens
	e.WorkerCacheWriteTokens = u.CacheWriteTokens
	e.WorkerCacheReadTokens = u.CacheReadTokens
	e.WorkerCostUSD = u.CostUSD
}

// recordCalibration feeds actual worker output into the calibration system.
// contentTokens is the token count of the content sent to the worker (excluding
// system prompt overhead). If cal is nil, this is a no-op.
func recordCalibration(cal *cost.Calibration, kind string, contentTokens, outputTokens int) {
	if cal == nil || contentTokens <= 0 {
		return
	}
	cal.RecordActual(kind, contentTokens, outputTokens)
}
