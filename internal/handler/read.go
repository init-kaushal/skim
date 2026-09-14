package handler

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/kaushal/skim/internal/detect"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
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
	// The saving is measured against what allowing the call would actually have
	// put in the context — Claude Code's Read returns at most maxWorkerInputLines
	// lines, so that prefix is the baseline, not the whole file. Using sz.Bytes
	// here would book savings for bytes that were never going to arrive.
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
