// Package decision wraps the cost model and compressor to produce a single
// Plan for each hook invocation. Handlers call Plan*; they don't embed cost
// math themselves.
package decision

import (
	"fmt"

	"github.com/kaushal/skim/internal/compress"
	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/filemap"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/session"
)

// Plan is the routing decision for one hook invocation.
type Plan struct {
	Strategy       cost.Strategy
	CompressResult compress.Result
	CostDecision   cost.Decision
	WorkerInput    string // content to send to worker (possibly compressed)
	Reason         string
	// ReuseCount is the number of times this target has been seen in the current
	// session (1 = first time, 2 = second time seen, etc.). Zero if session
	// tracking is not available.
	ReuseCount int
	// PredictedRatio is the output_tokens/input_tokens estimate used for cost
	// math. Carried through so observe entries can record it for telemetry.
	PredictedRatio float64
	// DeterministicFileMap is set when the plan uses the DETERMINISTIC strategy
	// for a Read interception — the file map was built from the AST without a
	// worker call. Nil for all other strategies.
	DeterministicFileMap *digest.FileMap
}

// Engine produces Plans. It is initialized with calibration data and session
// state so decisions improve over time as compression ratios are observed.
type Engine struct {
	Cfg config.Config
	Cal *cost.Calibration // nil → uses DefaultOutputRatio for all tools
	Ses *session.State    // nil → reuse tracking disabled
}

// New returns an Engine loaded with current calibration and session state.
// Both are loaded from disk; failures produce empty defaults (fail-open).
func New(cfg config.Config) *Engine {
	return &Engine{
		Cfg: cfg,
		Cal: cost.LoadCalibration(),
		Ses: session.Load(),
	}
}

// workerSystemOverhead is the approximate token count of skim's system prompt
// plus structural overhead added to every worker call.
const workerSystemOverhead = 650

// outputRatio returns the calibration-based output ratio for a given tool kind.
// Falls back to DefaultOutputRatio when no calibration data is available.
func (e *Engine) outputRatio(kind string) float64 {
	if e.Cal != nil {
		return e.Cal.EstimatedOutputRatio(kind)
	}
	return cost.DefaultOutputRatio
}

// PlanRead returns the routing decision for a Read hook interception.
// content is the (possibly capped) file content that skim would send to the
// worker. lines and bytes are derived from the file measurement.
func (e *Engine) PlanRead(filePath, content string, lines, bytes int) Plan {
	// Quick fast-path: files the threshold check already cleared.
	if !exceedsThreshold(lines, bytes, e.Cfg) {
		return Plan{
			Strategy: cost.StrategyPassthrough,
			Reason:   "below threshold",
		}
	}

	reuseCount := 0
	if e.Ses != nil {
		reuseCount = e.Ses.RecordRead(filePath)
	}

	// Deterministic tier: try to produce a file map from the AST before paying
	// for a worker call. For supported file types (currently .go), this is free,
	// instant, and produces the same FileMap schema the worker would return.
	if fm, ok := filemap.Generate(content, filePath); ok {
		return Plan{
			Strategy:             cost.StrategyDeterministic,
			DeterministicFileMap: &fm,
			WorkerInput:          content,
			Reason:               fmt.Sprintf("deterministic file map (%s) — no worker needed", filePath),
			ReuseCount:           reuseCount,
		}
	}

	origTokens := metrics.EstimateTokens(bytes)
	workerInputTokens := origTokens + workerSystemOverhead
	ratio := e.outputRatio("") // generic worker call
	workerOutputEst := int(float64(origTokens) * ratio)

	dec := cost.Evaluate(
		origTokens, workerInputTokens, workerOutputEst,
		e.Cfg.Model, e.Cfg.SessionModel,
		e.Cfg.SafetyMargin, e.Cfg.MinSavingsUSD,
	)

	if !dec.ShouldIntercept {
		return Plan{
			Strategy:       cost.StrategyDirect,
			CostDecision:   dec,
			WorkerInput:    content,
			Reason:         fmt.Sprintf("cost engine: %s", dec.Reason),
			ReuseCount:     reuseCount,
			PredictedRatio: ratio,
		}
	}

	return Plan{
		Strategy:       cost.StrategyCheapWorker,
		CostDecision:   dec,
		WorkerInput:    content,
		Reason:         fmt.Sprintf("cost engine: %s", dec.Reason),
		ReuseCount:     reuseCount,
		PredictedRatio: ratio,
	}
}

// PlanGrep returns the routing decision for a Grep hook interception.
// sample is the rg output excerpt. fullBytes is the full match output size.
func (e *Engine) PlanGrep(sample string, fullBytes int) Plan {
	origTokens := metrics.EstimateTokens(fullBytes)
	workerInputTokens := metrics.EstimateTokens(len(sample)) + workerSystemOverhead
	ratio := e.outputRatio("")
	workerOutputEst := int(float64(origTokens) * ratio)

	dec := cost.Evaluate(
		origTokens, workerInputTokens, workerOutputEst,
		e.Cfg.Model, e.Cfg.SessionModel,
		e.Cfg.SafetyMargin, e.Cfg.MinSavingsUSD,
	)

	if !dec.ShouldIntercept {
		return Plan{
			Strategy:       cost.StrategyDirect,
			CostDecision:   dec,
			WorkerInput:    sample,
			Reason:         fmt.Sprintf("cost engine: %s", dec.Reason),
			PredictedRatio: ratio,
		}
	}

	return Plan{
		Strategy:       cost.StrategyCheapWorker,
		CostDecision:   dec,
		WorkerInput:    sample,
		Reason:         dec.Reason,
		PredictedRatio: ratio,
	}
}

// PlanRun returns the routing decision for the skim run path (Bash output).
// It runs a deterministic compressor first; if complete, no LLM is needed.
//
// Decision hierarchy:
//  1. Deterministic compressor handles it completely → DETERMINISTIC (no LLM)
//  2. Cost engine says no (even accounting for any partial reduction) → DIRECT
//  3. Cost engine says yes → CHEAP_WORKER
func (e *Engine) PlanRun(commandOutput string) Plan {
	cr := compress.Compress(commandOutput)

	if cr.Complete && cr.Reduced {
		return Plan{
			Strategy:       cost.StrategyDeterministic,
			CompressResult: cr,
			WorkerInput:    cr.Output,
			Reason:         fmt.Sprintf("deterministic compressor %q: ratio %.2f", cr.Kind, cr.Ratio),
			PredictedRatio: cr.Ratio,
		}
	}

	// Use the (possibly reduced) content for cost evaluation.
	// If the deterministic compressor partially reduced the input, the worker
	// only pays to process the reduced version — but the savings are still
	// measured against the full original token count.
	input := commandOutput
	if cr.Reduced {
		input = cr.Output
	}
	origTokens := metrics.EstimateTokens(len(commandOutput))
	workerInputTokens := metrics.EstimateTokens(len(input)) + workerSystemOverhead
	ratio := e.outputRatio(cr.Kind)
	workerOutputEst := int(float64(origTokens) * ratio)

	dec := cost.Evaluate(
		origTokens, workerInputTokens, workerOutputEst,
		e.Cfg.Model, e.Cfg.SessionModel,
		e.Cfg.SafetyMargin, e.Cfg.MinSavingsUSD,
	)

	if !dec.ShouldIntercept {
		// Cost engine says no — partial deterministic reduction does not
		// override this. The worker call is not justified.
		return Plan{
			Strategy:       cost.StrategyDirect,
			CompressResult: cr,
			CostDecision:   dec,
			WorkerInput:    input,
			Reason:         dec.Reason,
			PredictedRatio: ratio,
		}
	}

	return Plan{
		Strategy:       cost.StrategyCheapWorker,
		CompressResult: cr,
		CostDecision:   dec,
		WorkerInput:    input,
		Reason:         dec.Reason,
		PredictedRatio: ratio,
	}
}

func exceedsThreshold(lines, bytes int, cfg config.Config) bool {
	return lines > cfg.ReadMaxLines || bytes > cfg.ReadMaxBytes
}
