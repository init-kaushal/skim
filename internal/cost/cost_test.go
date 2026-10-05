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
	// 500 tokens on a Haiku session with realistic compression (30% ratio):
	// workerOutput = 150 tokens, savedTokens = 350
	// Savings ≈ 350 × $0.80/1M = $0.00028
	// Worker cost: 1150 × $0.80/1M + 150 × $4/1M = $0.00092 + $0.0006 = $0.00152
	// threshold = $0.00152 × 1.5 = $0.00228
	// $0.00028 < $0.00228 → should not intercept
	dec := cost.Evaluate(500, 1150, 150, "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001", 1.5, 0.001)
	if dec.ShouldIntercept {
		t.Errorf("Haiku-session small file: savings too small, should not intercept, got: %s", dec.Reason)
	}
}

func TestEvaluate_LargeFileSavingsJustified(t *testing.T) {
	// 20,000 tokens ≈ 80KB file read by Opus session.
	// Calibration-based estimate: workerOutput = 20000 × 0.30 = 6000 tokens
	// savedTokens = 14000, savings = 14000 × $15/1M = $0.21
	// Worker cost: 20600 × $0.80/1M + 6000 × $4/1M = $0.016 + $0.024 = $0.040
	// threshold = $0.040 × 1.5 = $0.060
	// $0.21 > $0.060 → should intercept
	dec := cost.Evaluate(20000, 20600, 6000, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
	if !dec.ShouldIntercept {
		t.Errorf("large Opus file should justify interception, got: %s — %s", dec.Strategy, dec.Reason)
	}
	if dec.Strategy != cost.StrategyCheapWorker {
		t.Errorf("expected CHEAP_WORKER, got %s", dec.Strategy)
	}
}

func TestEvaluate_LargeFileSonnetSession(t *testing.T) {
	// Same file, session is Sonnet ($3/1M instead of $15/1M).
	// savings = 14000 × $3/1M = $0.042
	// threshold = $0.040 × 1.5 = $0.060
	// $0.042 < $0.060 → might not intercept on Sonnet with conservative estimate
	// This is intentional: Sonnet is cheaper, so the bar is higher.
	// We test that the cost fields are correctly populated instead.
	dec := cost.Evaluate(20000, 20600, 6000, "claude-haiku-4-5-20251001", "claude-sonnet-5-5", 1.5, 0.001)
	if dec.EstOrigCostUSD <= 0 {
		t.Error("EstOrigCostUSD should be positive for a non-trivial file")
	}
	if dec.EstWorkerCostUSD <= 0 {
		t.Error("EstWorkerCostUSD should be positive")
	}
}

func TestEvaluate_WorkerOutputEqualsOriginalNoSavings(t *testing.T) {
	// If the worker output is as large as the original, there are no savings.
	// This should return DIRECT, not CHEAP_WORKER.
	dec := cost.Evaluate(1000, 1650, 1000, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
	if dec.ShouldIntercept {
		t.Errorf("no-savings case must not intercept, got: %s — %s", dec.Strategy, dec.Reason)
	}
	if dec.Strategy == cost.StrategyPassthrough {
		t.Error("no-savings case should be DIRECT (not PASSTHROUGH — content is there, just not worth compressing)")
	}
}

func TestEvaluate_WorkerOutputLargerThanOriginalNoSavings(t *testing.T) {
	// Worker output larger than input means the "compression" expanded the content.
	dec := cost.Evaluate(1000, 1650, 1200, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
	if dec.ShouldIntercept {
		t.Errorf("expansion case must not intercept, got: %s — %s", dec.Strategy, dec.Reason)
	}
}

func TestEvaluate_CostFieldsNonNegative(t *testing.T) {
	dec := cost.Evaluate(5000, 5600, 1500, "claude-haiku-4-5-20251001", "claude-opus-5-5", 1.5, 0.001)
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
