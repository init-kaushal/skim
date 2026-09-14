package handler

import (
	"context"
	"time"

	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

// GrepHook is the PreToolUse interception for the Grep tool. It always returns
// nil after writing exactly one decision to d.Stdout: an allow (nothing) or a
// deny carrying a compact cluster digest. Every error or negative check degrades
// open — a broken optimiser must never block the tool call.
func GrepHook(ctx context.Context, in hookio.Input, d Deps) error {
	if d.Cfg.NotActiveReason(d.Env) != "" {
		return hookio.Allow(d.Stdout)
	}

	gi, err := in.Grep()
	if err != nil {
		d.Logf("grep-hook: bad tool_input: %v", err)
		return hookio.Allow(d.Stdout)
	}

	// These modes and limits are already bounded by Claude Code itself — a
	// count or a file list returns a handful of lines however many matches
	// exist, and an explicit small head_limit caps the result at or below the
	// threshold. Intercepting them spends a worker call to shrink nothing.
	// An empty OutputMode means the model omitted output_mode entirely — Claude
	// Code itself defaults that to "files_with_matches", so PreToolUse must
	// treat "" the same way or every omitted-output_mode Grep call gets
	// falsely intercepted.
	if gi.OutputMode == "" || gi.OutputMode == "count" || gi.OutputMode == "files_with_matches" {
		return hookio.Allow(d.Stdout)
	}
	if gi.HeadLimit > 0 && gi.HeadLimit <= d.Cfg.GrepMaxMatches {
		return hookio.Allow(d.Stdout)
	}

	total, err := d.CountMatches(gi)
	if err != nil {
		d.Logf("grep-hook: count matches: %v", err)
		return hookio.Allow(d.Stdout)
	}

	if total <= d.Cfg.GrepMaxMatches {
		return hookio.Allow(d.Stdout)
	}

	// Only now is a sample worth its own rg pass. fullBytes is how much the
	// match output ran to before skim capped it for the prompt.
	sample, fullBytes, err := d.Sample(gi)
	if err != nil {
		d.Logf("grep-hook: sample: %v", err)
		return hookio.Allow(d.Stdout)
	}

	raw, use, err := d.Summarize(ctx, worker.Request{
		Model:   d.Cfg.Model,
		Kind:    worker.KindClusters,
		Content: sample,
		Meta:    gi.Pattern,
		Timeout: time.Duration(d.Cfg.WorkerTimeoutSec) * time.Second,
		Partial: fullBytes > len(sample),
	})
	if err != nil {
		d.Logf("grep-hook: worker: %v", err)
		return hookio.Allow(d.Stdout)
	}

	cl, err := digest.ParseClusters(raw)
	if err != nil {
		d.Logf("grep-hook: parse digest: %v", err)
		return hookio.Allow(d.Stdout)
	}

	reason := digest.RenderClusters(cl)
	// Measured against the full match output, not against `sample`. sample is
	// skim's own capped prompt input, so using it booked "savings" for shrinking
	// skim's prompt rather than the tool result the model would have received.
	origEst := metrics.EstimateTokens(fullBytes)
	digEst := metrics.EstimateTokens(len(reason))
	entry := metrics.Entry{
		TS:              d.Now().UTC().Format(time.RFC3339),
		Tool:            "Grep",
		OrigTokensEst:   origEst,
		DigestTokensEst: digEst,
		SavedEst:        origEst - digEst,
		// CacheHit stays nil: the Grep path has no digest cache, and reporting
		// it as a miss is what made the cache hit rate meaningless.
	}
	applyUsage(&entry, use)
	d.Record(entry)
	return hookio.Deny(d.Stdout, reason)
}
