package router_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/router"
	"github.com/kaushal/skim/internal/worker"
)

func makeRun(responses map[string][]byte, err error) router.RunFunc {
	return func(_ context.Context, req worker.Request) ([]byte, worker.Usage, error) {
		if err != nil {
			return nil, worker.Usage{}, err
		}
		if resp, ok := responses[req.Model]; ok {
			return resp, worker.Usage{InputTokens: 1000, OutputTokens: len(resp), CostUSD: 0.001}, nil
		}
		return []byte(`{"summary":"ok","map":[{"lines":"1","kind":"body"}]}`), worker.Usage{InputTokens: 1000, OutputTokens: 50, CostUSD: 0.001}, nil
	}
}

const validFileMap = `{"summary":"test file","map":[{"lines":"1-10","kind":"body"}]}`

func TestRouter_SingleTier_Success(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = false
	r := router.New(cfg)

	res, err := r.Summarize(context.Background(), worker.Request{Model: cfg.Model}, makeRun(nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Escalated {
		t.Error("single-tier should not escalate")
	}
	if res.Tier.Strategy != cost.StrategyCheapWorker {
		t.Errorf("expected CHEAP_WORKER, got %s", res.Tier.Strategy)
	}
}

func TestRouter_EscalationFired(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true
	// Default worker is Haiku → chain: [Haiku, Sonnet]

	responses := map[string][]byte{
		cfg.Model:               []byte(`{"escalate": true}`),
		cost.DefaultSonnetModel: []byte(validFileMap),
	}
	r := router.New(cfg)
	res, err := r.Summarize(context.Background(), worker.Request{Model: cfg.Model}, makeRun(responses, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Escalated {
		t.Error("expected Escalated=true when first tier signaled")
	}
	if res.Tier.Strategy != cost.StrategyNormalWorker {
		t.Errorf("escalated tier should be NORMAL_WORKER, got %s", res.Tier.Strategy)
	}
	if string(res.Raw) != validFileMap {
		t.Errorf("expected sonnet response, got: %s", res.Raw)
	}

	// Aggregate usage must include both calls. Each call costs $0.001 with
	// 1000 input tokens, so two calls = $0.002 and 2000 input tokens.
	if res.Usage.CostUSD != 0.002 {
		t.Errorf("aggregate CostUSD: want 0.002, got %.4f", res.Usage.CostUSD)
	}
	if res.Usage.InputTokens != 2000 {
		t.Errorf("aggregate InputTokens: want 2000, got %d", res.Usage.InputTokens)
	}
	// OutputTokens: only from the successful (Sonnet) tier.
	if res.Usage.OutputTokens != len(validFileMap) {
		t.Errorf("aggregate OutputTokens: want %d (success tier only), got %d", len(validFileMap), res.Usage.OutputTokens)
	}

	// SuccessUsage must be only Sonnet's — used for compression ratio calibration.
	if res.SuccessUsage.InputTokens != 1000 {
		t.Errorf("SuccessUsage.InputTokens: want 1000 (Sonnet only), got %d", res.SuccessUsage.InputTokens)
	}
	if res.SuccessUsage.CostUSD != 0.001 {
		t.Errorf("SuccessUsage.CostUSD: want 0.001 (Sonnet only), got %.4f", res.SuccessUsage.CostUSD)
	}
}

func TestRouter_NoEscalationWhenFirstTierSucceeds(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true

	responses := map[string][]byte{
		cfg.Model: []byte(validFileMap),
	}
	r := router.New(cfg)
	res, err := r.Summarize(context.Background(), worker.Request{}, makeRun(responses, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Escalated {
		t.Error("should not escalate when first tier succeeds")
	}
	if res.Tier.Strategy != cost.StrategyCheapWorker {
		t.Errorf("expected CHEAP_WORKER, got %s", res.Tier.Strategy)
	}
}

func TestRouter_WorkerErrorDegrades(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true
	workerErr := errors.New("network timeout")

	r := router.New(cfg)
	_, err := r.Summarize(context.Background(), worker.Request{}, makeRun(nil, workerErr))
	if err == nil {
		t.Error("expected error to propagate, got nil")
	}
	if !errors.Is(err, workerErr) {
		t.Errorf("expected workerErr, got: %v", err)
	}
}

func TestRouter_LastTierNeverEscalates(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true

	// Both tiers return escalation; last tier should still be called with
	// AllowEscalation=false, so if the last tier returns escalation the router
	// exhausts and returns an error.
	alwaysEscalate := func(_ context.Context, req worker.Request) ([]byte, worker.Usage, error) {
		if req.AllowEscalation {
			return []byte(`{"escalate": true}`), worker.Usage{}, nil
		}
		// Last tier: must not escape with escalation signal — returns a result.
		return []byte(validFileMap), worker.Usage{OutputTokens: 50}, nil
	}
	r := router.New(cfg)
	res, err := r.Summarize(context.Background(), worker.Request{}, alwaysEscalate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Raw) != validFileMap {
		t.Errorf("expected fallback result from last tier, got: %s", res.Raw)
	}
}

func TestRouter_SonnetChain(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true
	cfg.Model = "claude-sonnet-5-5"

	r := router.New(cfg)
	if len(r.Tiers) != 2 {
		t.Fatalf("sonnet chain should have 2 tiers, got %d", len(r.Tiers))
	}
	if r.Tiers[0].Strategy != cost.StrategyNormalWorker {
		t.Errorf("first tier should be NORMAL_WORKER, got %s", r.Tiers[0].Strategy)
	}
	if r.Tiers[1].Strategy != cost.StrategyDeepWorker {
		t.Errorf("second tier should be DEEP_WORKER, got %s", r.Tiers[1].Strategy)
	}
}

func TestRouter_OpusChain(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true
	cfg.Model = "claude-opus-5-5"

	r := router.New(cfg)
	if len(r.Tiers) != 1 {
		t.Fatalf("opus chain should have 1 tier, got %d", len(r.Tiers))
	}
	if r.Tiers[0].Strategy != cost.StrategyDeepWorker {
		t.Errorf("opus tier should be DEEP_WORKER, got %s", r.Tiers[0].Strategy)
	}
}

func TestRouter_AllowEscalationSetOnlyOnNonTerminalTiers(t *testing.T) {
	cfg := config.Default()
	cfg.EscalationEnabled = true

	var seenAllowEscalation []bool
	tracking := func(_ context.Context, req worker.Request) ([]byte, worker.Usage, error) {
		seenAllowEscalation = append(seenAllowEscalation, req.AllowEscalation)
		return []byte(validFileMap), worker.Usage{}, nil
	}
	r := router.New(cfg)
	_, err := r.Summarize(context.Background(), worker.Request{}, tracking)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the first tier should be called (it succeeds immediately).
	// AllowEscalation must be true for non-terminal tiers, false for terminal.
	if len(seenAllowEscalation) != 1 {
		t.Errorf("expected 1 call, got %d", len(seenAllowEscalation))
	}
	// Single-call success: first tier is non-terminal (Haiku chain has Sonnet after),
	// so AllowEscalation should be true.
	if !seenAllowEscalation[0] {
		t.Error("non-terminal tier should have AllowEscalation=true")
	}
}
