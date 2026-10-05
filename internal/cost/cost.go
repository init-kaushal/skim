// Package cost provides pricing-aware interception decisions.
//
// The central question is: does intercepting this tool output save more than
// it costs? A cheap worker call that saves ten cents is worthwhile; one that
// saves two cents is not.
package cost

import "fmt"

// ModelPricing holds per-token prices for a single model. All prices are in
// USD per million tokens.
type ModelPricing struct {
	InputPer1M      float64
	OutputPer1M     float64
	CacheWritePer1M float64
	CacheReadPer1M  float64
}

// KnownPricing holds pricing for models skim interacts with. The session model
// (the expensive one Claude Code runs) determines how much keeping content in
// context actually costs. The worker model determines the optimization cost.
// Prices may change; override via Config.ModelPricing.
var KnownPricing = map[string]ModelPricing{
	"claude-haiku-4-5-20251001": {0.80, 4.00, 1.00, 0.08},
	"claude-haiku-4-5":          {0.80, 4.00, 1.00, 0.08},
	"claude-sonnet-5-5":         {3.00, 15.00, 3.75, 0.30},
	"claude-sonnet-5":           {3.00, 15.00, 3.75, 0.30},
	"claude-opus-5-5":           {15.00, 75.00, 18.75, 1.50},
	"claude-opus-5":             {15.00, 75.00, 18.75, 1.50},
	// Aliases used by skim stats
	"opus-5":    {15.00, 75.00, 18.75, 1.50},
	"sonnet-5":  {3.00, 15.00, 3.75, 0.30},
	"haiku-4.5": {0.80, 4.00, 1.00, 0.08},
}

// Strategy names the chosen optimization path.
type Strategy string

const (
	StrategyPassthrough   Strategy = "PASSTHROUGH"    // allowed without analysis
	StrategyDeterministic Strategy = "DETERMINISTIC"  // compacted without LLM
	StrategyCheapWorker   Strategy = "CHEAP_WORKER"   // Haiku worker
	StrategyDirect        Strategy = "DIRECT"         // savings don't justify cost
)

// Decision is the outcome of an interception evaluation.
type Decision struct {
	ShouldIntercept  bool
	Strategy         Strategy
	EstOrigCostUSD   float64
	EstWorkerCostUSD float64
	EstSavingsUSD    float64
	Confidence       float64
	Reason           string
}

// usd converts a token count and a per-million price to USD.
func usd(tokens int, pricePer1M float64) float64 {
	return float64(tokens) * pricePer1M / 1_000_000
}

// pricingFor returns pricing for the named model, with a fallback to Haiku when
// the model is unrecognised. Callers that want to detect unknown models should
// check KnownPricing directly.
func pricingFor(model string) ModelPricing {
	if p, ok := KnownPricing[model]; ok {
		return p
	}
	return KnownPricing["claude-haiku-4-5-20251001"]
}

// Evaluate returns an interception decision.
//
// origTokens: estimated tokens in the raw content.
// workerInputTokens: tokens sent to the worker (content + system prompt overhead).
// workerOutputTokens: expected worker response size.
// workerModel: model used for the worker call (usually Haiku).
// sessionModel: model running the Claude Code session (Opus or Sonnet).
// safetyMargin: worker_cost is multiplied by this before comparing (e.g. 1.5).
// minSavingsUSD: minimum net savings required to intercept (e.g. 0.001).
func Evaluate(
	origTokens, workerInputTokens, workerOutputTokens int,
	workerModel, sessionModel string,
	safetyMargin, minSavingsUSD float64,
) Decision {
	session := pricingFor(sessionModel)
	wkr := pricingFor(workerModel)

	if origTokens <= 0 {
		return Decision{
			Strategy: StrategyPassthrough,
			Reason:   "content too small to cost anything",
		}
	}

	// The "direct cost" is what those tokens cost if they ride along in the
	// session's context. We use input pricing because in the compounding model,
	// every future turn re-sends the cached prefix at the cache-read rate; here
	// we account for just the one immediate turn to stay conservative.
	origCostUSD := usd(origTokens, session.InputPer1M)

	workerCostUSD := usd(workerInputTokens, wkr.InputPer1M) +
		usd(workerOutputTokens, wkr.OutputPer1M)

	// The worker output IS the compressed content that enters the session context
	// — the digest replaces the original. workerOutputTokens must be set by the
	// caller using calibration data (see decision.Engine.PlanRead).
	compressedTokens := workerOutputTokens
	savedTokens := origTokens - compressedTokens
	if savedTokens <= 0 {
		return Decision{
			Strategy:         StrategyDirect,
			EstOrigCostUSD:   origCostUSD,
			EstWorkerCostUSD: workerCostUSD,
			Reason:           "worker output ≥ original: no savings possible",
		}
	}
	estimatedSavingsUSD := usd(savedTokens, session.InputPer1M)
	netSavingsUSD := estimatedSavingsUSD - workerCostUSD

	threshold := workerCostUSD * safetyMargin
	if safetyMargin <= 0 {
		threshold = workerCostUSD * 1.5
	}

	if origCostUSD <= 0 {
		return Decision{
			Strategy: StrategyPassthrough,
			Reason:   "content too small to cost anything",
		}
	}

	if estimatedSavingsUSD <= threshold {
		return Decision{
			Strategy:         StrategyDirect,
			EstOrigCostUSD:   origCostUSD,
			EstWorkerCostUSD: workerCostUSD,
			EstSavingsUSD:    netSavingsUSD,
			Confidence:       0.9,
			Reason:           fmt.Sprintf("estimated savings ($%.4f) ≤ worker cost × margin ($%.4f)", estimatedSavingsUSD, threshold),
		}
	}

	minUSD := minSavingsUSD
	if minUSD <= 0 {
		minUSD = 0.001
	}
	if netSavingsUSD < minUSD {
		return Decision{
			Strategy:         StrategyDirect,
			EstOrigCostUSD:   origCostUSD,
			EstWorkerCostUSD: workerCostUSD,
			EstSavingsUSD:    netSavingsUSD,
			Confidence:       0.8,
			Reason:           fmt.Sprintf("net savings ($%.4f) < minimum ($%.4f)", netSavingsUSD, minUSD),
		}
	}

	return Decision{
		ShouldIntercept:  true,
		Strategy:         StrategyCheapWorker,
		EstOrigCostUSD:   origCostUSD,
		EstWorkerCostUSD: workerCostUSD,
		EstSavingsUSD:    netSavingsUSD,
		Confidence:       0.85,
		Reason:           fmt.Sprintf("savings ($%.4f) justify worker cost ($%.4f)", netSavingsUSD, workerCostUSD),
	}
}
