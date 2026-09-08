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

// maxWorkerInputBytes caps how much of a large file we ship to the nested
// summariser. The digest only needs enough to map structure, not the whole file.
const maxWorkerInputBytes = 400 * 1024

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

	lines, bytesLen, err := detect.FileSize(ri.FilePath)
	if err != nil {
		d.Logf("read-hook: stat %s: %v", ri.FilePath, err)
		return hookio.Allow(d.Stdout)
	}

	if !detect.ExceedsThreshold(lines, bytesLen, d.Cfg) {
		return hookio.Allow(d.Stdout)
	}

	key := ""
	if k, kerr := d.CacheKey(ri.FilePath, d.Cfg.Model); kerr == nil {
		key = k
	} else {
		d.Logf("read-hook: cache key %s: %v", ri.FilePath, kerr)
	}

	var fm digest.FileMap
	hit := false
	if key != "" {
		fm, hit = d.CacheGet(key)
	}

	if !hit {
		content, rerr := readCapped(ri.FilePath, maxWorkerInputBytes)
		if rerr != nil {
			d.Logf("read-hook: read %s: %v", ri.FilePath, rerr)
			return hookio.Allow(d.Stdout)
		}
		raw, werr := d.Summarize(ctx, worker.Request{
			Model:   d.Cfg.Model,
			Kind:    worker.KindFileMap,
			Content: content,
			Meta:    ri.FilePath,
			Timeout: time.Duration(d.Cfg.WorkerTimeoutSec) * time.Second,
		})
		if werr != nil {
			d.Logf("read-hook: worker %s: %v", ri.FilePath, werr)
			return hookio.Allow(d.Stdout)
		}
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

	reason := digest.RenderFileMap(fm, ri.FilePath)
	origEst := metrics.EstimateTokens(bytesLen)
	digEst := metrics.EstimateTokens(len(reason))
	d.Record(metrics.Entry{
		TS:              d.Now().UTC().Format(time.RFC3339),
		Tool:            "Read",
		OrigTokensEst:   origEst,
		DigestTokensEst: digEst,
		CacheHit:        hit,
		SavedEst:        origEst - digEst,
	})
	return hookio.Deny(d.Stdout, reason)
}

// readCapped returns up to max bytes from the start of path. Files shorter than
// max return their full contents (an empty file yields "" and nil); only an open
// failure or a genuine read error surfaces, and the caller degrades open on it.
func readCapped(path string, max int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	content, err := io.ReadAll(io.LimitReader(f, int64(max)))
	if err != nil {
		return "", err
	}
	return string(content), nil
}
