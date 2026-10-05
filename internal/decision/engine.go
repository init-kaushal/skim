// Package decision wraps the cost model and compressor to produce a single
// Plan for each hook invocation. Handlers call Evaluate; they don't embed
// cost math themselves.
package decision

import (
	"fmt"

	"github.com/kaushal/skim/internal/compress"
	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/metrics"
)

// Plan is the routing decision for one hook invocation.
type Plan struct {
	Strategy        cost.Strategy
	CompressResult  compress.Result
	CostDecision    cost.Decision
	WorkerInput     string // content to send to worker (possibly compressed)
	Reason          string
}

// Engine produces Plans. It is stateless for Phase 1; read-count tracking
// for reuse prediction is deferred to Phase 4.
type Engine struct {
	Cfg config.Config
}

// New returns an Engine ready for use.
func New(cfg config.Config) *Engine { return &Engine{Cfg: cfg} }

// workerSystemOverhead is the approximate token count of skim's system prompt
// plus structural overhead added to every worker call.
const workerSystemOverhead = 650

// workerOutputEst is the expected token count of a typical worker response.
const workerOutputEst = 450

// PlanRead returns the routing decision for a Read hook interception.
// content is the (possibly capped) file content that skim would send to the
// worker. lines and bytes are derived from the file measurement.
func (e *Engine) PlanRead(filePath, content string, lines, bytes int) Plan {
	// Quick fast-path: files the threshold check already cleared.
	// (Callers should have run ExceedsThreshold before calling PlanRead,
	// but we enforce it here for safety.)
	if !exceedsThreshold(lines, bytes, e.Cfg) {
		return Plan{
			Strategy: cost.StrategyPassthrough,
			Reason:   "below threshold",
		}
	}

	origTokens := metrics.EstimateTokens(bytes)
	workerInputTokens := origTokens + workerSystemOverhead

	dec := cost.Evaluate(
		origTokens, workerInputTokens, workerOutputEst,
		e.Cfg.Model, e.Cfg.SessionModel,
		e.Cfg.SafetyMargin, e.Cfg.MinSavingsUSD,
	)

	if !dec.ShouldIntercept {
		return Plan{
			Strategy:     cost.StrategyDirect,
			CostDecision: dec,
			WorkerInput:  content,
			Reason:       fmt.Sprintf("cost engine: %s", dec.Reason),
		}
	}

	return Plan{
		Strategy:     cost.StrategyCheapWorker,
		CostDecision: dec,
		WorkerInput:  content,
		Reason:       fmt.Sprintf("cost engine: %s", dec.Reason),
	}
}

// PlanGrep returns the routing decision for a Grep hook interception.
// sample is the rg output excerpt. fullBytes is the full match output size.
func (e *Engine) PlanGrep(sample string, fullBytes int) Plan {
	origTokens := metrics.EstimateTokens(fullBytes)
	workerInputTokens := metrics.EstimateTokens(len(sample)) + workerSystemOverhead

	dec := cost.Evaluate(
		origTokens, workerInputTokens, workerOutputEst,
		e.Cfg.Model, e.Cfg.SessionModel,
		e.Cfg.SafetyMargin, e.Cfg.MinSavingsUSD,
	)

	if !dec.ShouldIntercept {
		return Plan{
			Strategy:     cost.StrategyDirect,
			CostDecision: dec,
			WorkerInput:  sample,
			Reason:       fmt.Sprintf("cost engine: %s", dec.Reason),
		}
	}

	return Plan{
		Strategy:     cost.StrategyCheapWorker,
		CostDecision: dec,
		WorkerInput:  sample,
		Reason:       dec.Reason,
	}
}

// PlanRun returns the routing decision for the skim run path (Bash output).
// It runs a deterministic compressor first; if complete, no LLM is needed.
func (e *Engine) PlanRun(commandOutput string) Plan {
	cr := compress.Compress(commandOutput)

	if cr.Complete && cr.Reduced {
		return Plan{
			Strategy:       cost.StrategyDeterministic,
			CompressResult: cr,
			WorkerInput:    cr.Output,
			Reason:         fmt.Sprintf("deterministic compressor %q: ratio %.2f", cr.Kind, cr.Ratio),
		}
	}

	// Use the (possibly reduced) content for cost evaluation
	input := commandOutput
	if cr.Reduced {
		input = cr.Output
	}
	origTokens := metrics.EstimateTokens(len(commandOutput))
	workerInputTokens := metrics.EstimateTokens(len(input)) + workerSystemOverhead

	dec := cost.Evaluate(
		origTokens, workerInputTokens, workerOutputEst,
		e.Cfg.Model, e.Cfg.SessionModel,
		e.Cfg.SafetyMargin, e.Cfg.MinSavingsUSD,
	)

	if !dec.ShouldIntercept && !cr.Reduced {
		return Plan{
			Strategy:       cost.StrategyDirect,
			CompressResult: cr,
			CostDecision:   dec,
			WorkerInput:    input,
			Reason:         dec.Reason,
		}
	}

	// Even if cost engine says no, if we have a partial deterministic reduction,
	// use it to reduce the worker input.
	return Plan{
		Strategy:       cost.StrategyCheapWorker,
		CompressResult: cr,
		CostDecision:   dec,
		WorkerInput:    input,
		Reason:         dec.Reason,
	}
}

func exceedsThreshold(lines, bytes int, cfg config.Config) bool {
	return lines > cfg.ReadMaxLines || bytes > cfg.ReadMaxBytes
}
