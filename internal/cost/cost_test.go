package cost_test

import (
	"testing"

	"github.com/kaushal/skim/internal/cost"
)

func TestEvaluate_TinyContent(t *testing.T) {
	dec := cost.Evaluate(0, 600, 400, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
	if dec.ShouldIntercept {
		t.Error("zero tokens should not intercept")
	}
	if dec.Strategy != cost.StrategyPassthrough {
		t.Errorf("expected PASSTHROUGH, got %s", dec.Strategy)
	}
}

func TestEvaluate_SmallFileHaikuSession(t *testing.T) {
	// 500 tokens on a Haiku session: $0.80/1M savings vs $0.0024 worker cost.
	// Savings ≈ $0.0005 which is below the $0.001 min_savings_usd threshold.
	dec := cost.Evaluate(500, 1100, 400, "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001", 1.5, 0.001)
	if dec.ShouldIntercept {
		t.Errorf("Haiku-session small file: savings too small, should not intercept, got: %s", dec.Reason)
	}
}

func TestEvaluate_LargeFileSavingsJustified(t *testing.T) {
	// 20,000 tokens ≈ 80KB file read by Opus session.
	// Savings: ~17,143 tokens × $15/1M = $0.257
	// Worker cost: 20,600 in × $0.80/1M + 400 out × $4.00/1M = $0.016 + $0.002 = $0.018
	dec := cost.Evaluate(20000, 20600, 400, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
	if !dec.ShouldIntercept {
		t.Errorf("large Opus file should justify interception, got: %s — %s", dec.Strategy, dec.Reason)
	}
	if dec.Strategy != cost.StrategyCheapWorker {
		t.Errorf("expected CHEAP_WORKER, got %s", dec.Strategy)
	}
}

func TestEvaluate_LargeFileSonnetSession(t *testing.T) {
	// Same file, but session is Sonnet ($3/1M instead of $15/1M).
	// Savings: ~17,143 × $3/1M = $0.051
	// Worker cost: ~$0.018 × 1.5 margin = $0.027 threshold
	// $0.051 > $0.027 → should still intercept
	dec := cost.Evaluate(20000, 20600, 400, "claude-haiku-4-5-20251001", "claude-sonnet-5-5", 1.5, 0.001)
	if !dec.ShouldIntercept {
		t.Errorf("large Sonnet file should justify interception, got: %s", dec.Reason)
	}
}

func TestEvaluate_CostFieldsNonNegative(t *testing.T) {
	dec := cost.Evaluate(5000, 5600, 400, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
	if dec.EstOrigCostUSD < 0 {
		t.Error("EstOrigCostUSD must not be negative")
	}
	if dec.EstWorkerCostUSD < 0 {
		t.Error("EstWorkerCostUSD must not be negative")
	}
}

func TestKnownPricing_HasRequiredModels(t *testing.T) {
	required := []string{
		"claude-haiku-4-5-20251001",
		"claude-sonnet-5-5",
		"claude-opus-5-5",
	}
	for _, m := range required {
		if _, ok := cost.KnownPricing[m]; !ok {
			t.Errorf("KnownPricing missing %q", m)
		}
	}
}
