package handler

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/kaushal/skim/internal/cache"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/detect"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/observe"
	"github.com/kaushal/skim/internal/worker"
)

// maxWorkerInputLines caps how much of a large file we ship to the nested
// summariser, and is deliberately set to Claude Code's own Read cap: the Read
// tool returns at most 2000 lines. Digesting more than that cannot save
// anything, because the extra was never going to reach the session's context in
// the first place — it only adds worker cost. Measured against Opus 5 input
// pricing, with the old 400 KB cap:
//
//	 60 KB / 1,933 lines   skim $0.062  vs  Read $0.115   skim wins 1.9x
//	120 KB / 3,867 lines   skim $0.096  vs  Read $0.118   skim wins 1.2x
//	200 KB / 6,445 lines   skim $0.159  vs  Read $0.120   skim LOSES 1.3x
//	400 KB / 12,890 lines  skim $0.311  vs  Read $0.119   skim LOSES 2.6x
//
// The losses are entirely the cost of reading past line 2000. Capping by lines
// instead of bytes removes them.
const maxWorkerInputLines = 2000

// maxWorkerInputBytes is a byte backstop for pathological shapes — minified
// bundles, single-line JSON, generated data — where 2000 lines could still be
// megabytes. It is not the primary bound; maxWorkerInputLines is.
const maxWorkerInputBytes = 128 * 1024

// ReadHook is the PreToolUse interception for the Read tool. It always returns
// nil after writing exactly one decision to d.Stdout: an allow (nothing) or a
// deny carrying a compact digest. Every error or negative check degrades open —
// a broken optimiser must never block the tool call.
func ReadHook(ctx context.Context, in hookio.Input, d Deps) error {
	if d.Cfg.NotActiveReason(d.Env) != "" {
		return hookio.Allow(d.Stdout)
	}

	ri, err := in.Read()
	if err != nil {
		d.Logf("read-hook: bad tool_input: %v", err)
		return hookio.Allow(d.Stdout)
	}

	if ri.Offset != 0 || ri.Limit != 0 {
		return hookio.Allow(d.Stdout)
	}

	if detect.MatchPassthrough(ri.FilePath, d.Cfg.PassthroughGlobs) {
		return hookio.Allow(d.Stdout)
	}

	if st, err := os.Stat(ri.FilePath); err != nil || !st.Mode().IsRegular() {
		return hookio.Allow(d.Stdout)
	}

	// Images, PDFs and other binaries are checked before the size threshold so
	// no binary reaches the worker at any size: the bytes are not valid UTF-8,
	// the digest would be useless, and denying the Read would stop the model
	// ever viewing the file. A sniff error means "can't tell" — fall through to
	// the normal path rather than letting an unreadable file skip interception.
	if bin, berr := detect.LooksBinary(ri.FilePath); berr != nil {
		d.Logf("read-hook: binary sniff %s: %v", ri.FilePath, berr)
	} else if bin {
		return hookio.Allow(d.Stdout)
	}

	sz, err := detect.Measure(ri.FilePath, maxWorkerInputLines)
	if err != nil {
		d.Logf("read-hook: stat %s: %v", ri.FilePath, err)
		return hookio.Allow(d.Stdout)
	}
	if !detect.ExceedsThreshold(sz.Lines, sz.Bytes, d.Cfg) {
		return hookio.Allow(d.Stdout)
	}

	// What the worker will actually be shown: a whole number of lines, bounded
	// by the Read-tool-matching line cap and then by the byte backstop.
	seen := min(sz.PrefixBytes, maxWorkerInputBytes)

	// Phase 1: cost-aware routing via the decision engine. When the engine is
	// wired in, we read the file once and let the engine decide whether the
	// worker call is worth its cost. Falls back to the threshold-only path
	// (engine == nil) for backward compatibility with tests that don't wire it.
	if d.Engine != nil {
		content, rerr := readCapped(ri.FilePath, seen)
		if rerr != nil {
			d.Logf("read-hook: read %s: %v", ri.FilePath, rerr)
			return hookio.Allow(d.Stdout)
		}

		cov := digest.Coverage{
			SeenBytes: seen, TotalBytes: sz.Bytes,
			SeenLines: min(sz.Lines, maxWorkerInputLines), TotalLines: sz.Lines,
		}

		// Deterministic cache check: if this exact content was parsed before, the
		// result is stored under a "deterministic" key. Checking it here, before
		// calling PlanRead, means we skip the AST parse on subsequent reads of the
		// same file — the write in the generation branch below populates the entry
		// and this branch consumes it on every read after the first.
		detKey := cache.KeyFromContent([]byte(content), "deterministic")
		if fm, hit := d.CacheGet(detKey); hit {
			reason := digest.RenderFileMap(fm, ri.FilePath, quoteSelfIfNeeded(execPath()), cov)
			origEst := metrics.EstimateTokens(sz.PrefixBytes)
			digEst := metrics.EstimateTokens(len(reason))
			if d.ObserveRecord != nil {
				d.ObserveRecord(observe.Entry{
					Tool:     "Read",
					Target:   ri.FilePath,
					Strategy: cost.StrategyDeterministic,
					DryRun:   d.Cfg.DryRun,
					Reason:   "deterministic cache hit",
				})
			}
			d.Record(metrics.Entry{
				TS:              d.Now().UTC().Format(time.RFC3339),
				Tool:            "Read",
				Strategy:        string(cost.StrategyDeterministic),
				OrigTokensEst:   origEst,
				DigestTokensEst: digEst,
				SavedEst:        origEst - digEst,
				CacheHit:        metrics.Hit(),
			})
			return hookio.Deny(d.Stdout, reason)
		}

		plan := d.Engine.PlanRead(ri.FilePath, content, sz.Lines, len(content))

		obs := observe.Entry{
			Tool:             "Read",
			Target:           ri.FilePath,
			Strategy:         plan.Strategy,
			DryRun:           d.Cfg.DryRun,
			EstOrigCostUSD:   plan.CostDecision.EstOrigCostUSD,
			EstWorkerCostUSD: plan.CostDecision.EstWorkerCostUSD,
			EstNetSavingsUSD: plan.CostDecision.EstSavingsUSD,
			PredictedRatio:   plan.PredictedRatio,
			ReuseCount:       plan.ReuseCount,
			Reason:           plan.Reason,
		}
		if d.ObserveRecord != nil {
			d.ObserveRecord(obs)
		}

		if d.Cfg.DryRun || plan.Strategy == cost.StrategyDirect || plan.Strategy == cost.StrategyPassthrough {
			return hookio.Allow(d.Stdout)
		}

		// Deterministic tier: AST-generated file map. Write to the deterministic
		// cache so the check above serves subsequent reads from cache, skipping
		// the AST parse.
		if plan.Strategy == cost.StrategyDeterministic && plan.DeterministicFileMap != nil {
			_ = d.CachePut(detKey, *plan.DeterministicFileMap)

			reason := digest.RenderFileMap(*plan.DeterministicFileMap, ri.FilePath, quoteSelfIfNeeded(execPath()), cov)
			origEst := metrics.EstimateTokens(sz.PrefixBytes)
			digEst := metrics.EstimateTokens(len(reason))
			d.Record(metrics.Entry{
				TS:              d.Now().UTC().Format(time.RFC3339),
				Tool:            "Read",
				Strategy:        string(plan.Strategy),
				OrigTokensEst:   origEst,
				DigestTokensEst: digEst,
				SavedEst:        origEst - digEst,
				CacheHit:        metrics.Miss(),
			})
			return hookio.Deny(d.Stdout, reason)
		}

		// Cost engine says intercept via worker: use content-hash key for cache lookup.
		contentKey := cache.KeyFromContent([]byte(content), d.Cfg.Model)

		var fm digest.FileMap
		var use worker.Usage
		hit := false
		// Check cache before paying for a worker call.
		fm, hit = d.CacheGet(contentKey)

		actualStrategy := plan.Strategy
		if !hit {
			raw, u, actualStrat, werr := d.summarizeWithRouter(ctx, worker.Request{
				Model:   d.Cfg.Model,
				Kind:    worker.KindFileMap,
				Content: content,
				Meta:    ri.FilePath,
				Timeout: time.Duration(d.Cfg.WorkerTimeoutSec) * time.Second,
				Partial: cov.Partial(),
			}, plan.Strategy)
			if werr != nil {
				d.Logf("read-hook: worker %s: %v", ri.FilePath, werr)
				return hookio.Allow(d.Stdout)
			}
			use = u
			actualStrategy = actualStrat
			parsed, perr := digest.ParseFileMap(raw)
			if perr != nil {
				d.Logf("read-hook: parse digest %s: %v", ri.FilePath, perr)
				return hookio.Allow(d.Stdout)
			}
			fm = parsed
			_ = d.CachePut(contentKey, fm)

			// Telemetry feedback: record actual compression ratio so future
			// predictions improve. Content tokens (no system overhead) vs output.
			contentTokens := metrics.EstimateTokens(len(content))
			recordCalibration(d.Calibration, "", contentTokens, use.OutputTokens)
		}

		reason := digest.RenderFileMap(fm, ri.FilePath, quoteSelfIfNeeded(execPath()), cov)
		origEst := metrics.EstimateTokens(sz.PrefixBytes)
		digEst := metrics.EstimateTokens(len(reason))
		entry := metrics.Entry{
			TS:                        d.Now().UTC().Format(time.RFC3339),
			Tool:                      "Read",
			Strategy:                  string(actualStrategy),
			OrigTokensEst:             origEst,
			DigestTokensEst:           digEst,
			SavedEst:                  origEst - digEst,
			CacheHit:                  metrics.Miss(),
			PredictedCompressionRatio: plan.PredictedRatio,
		}
		if hit {
			entry.CacheHit = metrics.Hit()
		} else if entry.WorkerInputTokens > 0 && use.OutputTokens > 0 {
			entry.ActualCompressionRatio = float64(use.OutputTokens) / float64(entry.WorkerInputTokens)
		}
		applyUsage(&entry, use)
		d.Record(entry)
		return hookio.Deny(d.Stdout, reason)
	}

	// Legacy path (no engine wired): threshold-only decision, mtime-based cache key.
	key := ""
	if k, kerr := d.CacheKey(ri.FilePath, d.Cfg.Model); kerr == nil {
		key = k
	} else {
		d.Logf("read-hook: cache key %s: %v", ri.FilePath, kerr)
	}

	var fm digest.FileMap
	hit := false
	var use worker.Usage
	if key != "" {
		fm, hit = d.CacheGet(key)
	}

	// How much of the file the digest can possibly describe. Derived from the
	// single measurement above rather than from len(content), so it is known on
	// a cache hit too — the rendered disclosure must not depend on whether this
	// particular call happened to run the worker.
	cov := digest.Coverage{
		SeenBytes: seen, TotalBytes: sz.Bytes,
		SeenLines: min(sz.Lines, maxWorkerInputLines), TotalLines: sz.Lines,
	}

	if !hit {
		content, rerr := readCapped(ri.FilePath, seen)
		if rerr != nil {
			d.Logf("read-hook: read %s: %v", ri.FilePath, rerr)
			return hookio.Allow(d.Stdout)
		}
		raw, u, werr := d.Summarize(ctx, worker.Request{
			Model:   d.Cfg.Model,
			Kind:    worker.KindFileMap,
			Content: content,
			Meta:    ri.FilePath,
			Timeout: time.Duration(d.Cfg.WorkerTimeoutSec) * time.Second,
			Partial: cov.Partial(),
		})
		if werr != nil {
			d.Logf("read-hook: worker %s: %v", ri.FilePath, werr)
			return hookio.Allow(d.Stdout)
		}
		use = u
		parsed, perr := digest.ParseFileMap(raw)
		if perr != nil {
			d.Logf("read-hook: parse digest %s: %v", ri.FilePath, perr)
			return hookio.Allow(d.Stdout)
		}
		fm = parsed
		if key != "" {
			_ = d.CachePut(key, fm)
		}
	}

	reason := digest.RenderFileMap(fm, ri.FilePath, quoteSelfIfNeeded(execPath()), cov)
	origEst := metrics.EstimateTokens(sz.PrefixBytes)
	digEst := metrics.EstimateTokens(len(reason))
	entry := metrics.Entry{
		TS:              d.Now().UTC().Format(time.RFC3339),
		Tool:            "Read",
		OrigTokensEst:   origEst,
		DigestTokensEst: digEst,
		SavedEst:        origEst - digEst,
		CacheHit:        metrics.Miss(),
	}
	if hit {
		entry.CacheHit = metrics.Hit()
	}
	applyUsage(&entry, use)
	d.Record(entry)
	return hookio.Deny(d.Stdout, reason)
}

// readCapped returns up to limit bytes from the start of path. Files shorter
// than limit return their full contents (an empty file yields "" and nil); only
// an open failure or a genuine read error surfaces, and the caller degrades open
// on it. The parameter is not named max: this file now uses the Go 1.21 min/max
// builtins, and shadowing one of them here would be a trap.
func readCapped(path string, limit int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	content, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return "", err
	}
	return string(content), nil
}
