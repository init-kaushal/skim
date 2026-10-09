// Package router implements the worker escalation chain: Haiku → Sonnet → Opus.
//
// When the configured cheap worker (Haiku) signals it cannot produce a confident
// summary, the router retries with the next tier up. This keeps the common case
// cheap while ensuring complex or domain-specific content still gets a useful
// digest rather than a poor-quality one.
//
// The escalation chain is derived from config, not hardcoded:
//
//	worker model = Haiku  → chain: [Haiku, Sonnet]
//	worker model = Sonnet → chain: [Sonnet, Opus]
//	worker model = Opus   → chain: [Opus]            (already at top)
//
// Escalation only fires when the worker emits {"escalate": true}. Worker errors
// and timeouts degrade open (the original allow path) rather than escalating,
// because a network or auth problem won't improve with a more expensive model.
package router

import (
	"context"
	"fmt"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/worker"
)

// Tier is one step in the escalation chain.
type Tier struct {
	Model    string
	Strategy cost.Strategy
}

// Router runs a chain of worker tiers in order, escalating when a tier signals
// it cannot produce a confident digest.
type Router struct {
	Tiers []Tier
}

// Result is the outcome of a Summarize call.
type Result struct {
	Raw      []byte
	// Usage is the aggregate across every attempted tier — including tiers that
	// escalated. CostUSD is the sum of all calls; InputTokens and cache variants
	// are likewise cumulative. OutputTokens holds only the successful tier's
	// output (the escalated calls produced no useful output). Use Usage for
	// billing/savings reporting so the full cost of the operation is visible.
	Usage worker.Usage
	// SuccessUsage is the successful tier's usage in isolation. Use it for
	// calibration (compression ratio = SuccessUsage.OutputTokens / content
	// tokens) so escalated-call noise does not corrupt the ratio estimates.
	SuccessUsage worker.Usage
	Tier         Tier
	Escalated    bool // true if any tier was skipped before success
}

// RunFunc is the signature of the worker function the router calls. It matches
// worker.Run so callers can pass it directly.
type RunFunc func(ctx context.Context, req worker.Request) ([]byte, worker.Usage, error)

// New builds a Router from the configured worker model. The escalation chain
// is derived automatically: the configured model is the starting tier and the
// chain ascends from there. If EscalationEnabled is false in config (the
// default until Phase 3 is wired), returns a single-tier router that never
// escalates.
func New(cfg config.Config) *Router {
	if !cfg.EscalationEnabled {
		return &Router{Tiers: []Tier{{Model: cfg.Model, Strategy: cost.StrategyCheapWorker}}}
	}
	return &Router{Tiers: chainFor(cfg.Model)}
}

// chainFor builds the escalation chain starting from model. The chain ascends
// to at most Sonnet (two hops) — Opus is only included if model is already
// Sonnet. This keeps automatic escalation costs bounded.
func chainFor(model string) []Tier {
	switch modelFamily(model) {
	case "haiku":
		return []Tier{
			{Model: model, Strategy: cost.StrategyCheapWorker},
			{Model: cost.DefaultSonnetModel, Strategy: cost.StrategyNormalWorker},
		}
	case "sonnet":
		return []Tier{
			{Model: model, Strategy: cost.StrategyNormalWorker},
			{Model: cost.DefaultOpusModel, Strategy: cost.StrategyDeepWorker},
		}
	default: // opus or unknown
		return []Tier{
			{Model: model, Strategy: cost.StrategyDeepWorker},
		}
	}
}

// Summarize runs the escalation chain. It tries each tier in order and returns
// the first successful result. A tier is skipped — and the next tried — only
// when it emits an explicit escalation signal. Worker errors always degrade
// open (returned as-is) without escalating.
//
// Every attempted tier's cost is accumulated into Result.Usage so the caller
// sees the true cost of the operation, not just the successful tier's bill.
func (r *Router) Summarize(ctx context.Context, req worker.Request, fn RunFunc) (Result, error) {
	var agg worker.Usage

	for i, tier := range r.Tiers {
		req.Model = tier.Model
		// Only non-terminal tiers may signal escalation; the last tier must
		// commit to a result regardless of confidence.
		req.AllowEscalation = i < len(r.Tiers)-1

		raw, u, err := fn(ctx, req)
		if err != nil {
			// Worker error: degrade open, do not escalate — a different model
			// won't fix a network problem or a bad prompt.
			return Result{}, err
		}

		// Accumulate this tier's cost unconditionally. An escalated call was
		// still billed even though it produced no useful output.
		agg.CostUSD += u.CostUSD
		agg.InputTokens += u.InputTokens
		agg.CacheWriteTokens += u.CacheWriteTokens
		agg.CacheReadTokens += u.CacheReadTokens

		if worker.IsEscalation(raw) {
			// Explicit escalation signal: try the next tier.
			continue
		}

		// Success: add this tier's output tokens to the aggregate. Output is
		// meaningful only from the tier that produced a result.
		agg.OutputTokens = u.OutputTokens

		return Result{
			Raw:          raw,
			Usage:        agg,
			SuccessUsage: u,
			Tier:         tier,
			Escalated:    i > 0,
		}, nil
	}

	// All tiers signaled escalation. This should not happen (the last tier has
	// AllowEscalation=false so it must return a real response), but degrade
	// open rather than panicking.
	return Result{}, fmt.Errorf("router: all %d tiers exhausted without a result", len(r.Tiers))
}

// modelFamily returns "haiku", "sonnet", or "opus" for a model ID string.
func modelFamily(model string) string {
	for _, frag := range []string{"haiku", "sonnet", "opus"} {
		// Simple substring check is sufficient: all known model IDs embed the
		// family name (claude-haiku-4-5, claude-sonnet-5-5, claude-opus-5-5).
		for i := 0; i+len(frag) <= len(model); i++ {
			if model[i:i+len(frag)] == frag {
				return frag
			}
		}
	}
	return "unknown"
}
