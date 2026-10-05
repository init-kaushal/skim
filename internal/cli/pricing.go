package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/cost"
)

// Pricing prints the current model pricing table and calibration statistics.
// It shows:
//   - Per-model pricing (from KnownPricing, noting any config overrides)
//   - Current session and worker model configuration
//   - Calibration statistics (observed compression ratios per tool kind)
func Pricing(w io.Writer, cal *cost.Calibration, cfg config.Config) error {
	fmt.Fprintf(w, "Model pricing (USD per million tokens):\n\n")
	fmt.Fprintf(w, "%-30s  %6s  %7s  %12s  %11s\n",
		"Model", "Input", "Output", "Cache-Write", "Cache-Read")
	fmt.Fprintf(w, "%-30s  %6s  %7s  %12s  %11s\n",
		"-----", "-----", "------", "-----------", "----------")

	// Print canonical models (not aliases) in a deterministic order.
	canonical := []string{
		"claude-haiku-4-5-20251001",
		"claude-sonnet-5-5",
		"claude-opus-5-5",
	}
	for _, m := range canonical {
		p, ok := cost.KnownPricing[m]
		if !ok {
			continue
		}
		suffix := ""
		if _, overridden := cfg.ModelPricingOverrides[m]; overridden {
			suffix = " *"
		}
		fmt.Fprintf(w, "%-30s  %6.2f  %7.2f  %12.2f  %11.2f%s\n",
			m, p.InputPer1M, p.OutputPer1M, p.CacheWritePer1M, p.CacheReadPer1M, suffix)
	}
	if len(cfg.ModelPricingOverrides) > 0 {
		fmt.Fprintf(w, "  (* overridden by model_pricing_overrides in config.json)\n")
	}

	fmt.Fprintf(w, "\nActive configuration:\n")
	fmt.Fprintf(w, "  Worker model:   %s\n", cfg.Model)
	fmt.Fprintf(w, "  Session model:  %s (assumed; cannot detect from hooks)\n", cfg.SessionModel)
	fmt.Fprintf(w, "  Safety margin:  %.1f× (intercept only when savings > cost × %.1f)\n",
		cfg.SafetyMargin, cfg.SafetyMargin)
	fmt.Fprintf(w, "  Min savings:    $%.4f per operation\n", cfg.MinSavingsUSD)
	if cfg.DryRun {
		fmt.Fprintf(w, "  Mode:           dry-run (decisions logged, no interceptions)\n")
	}

	if cal == nil {
		return nil
	}

	stats := cal.Stats()
	if len(stats) == 0 {
		fmt.Fprintf(w, "\nCalibration: no data yet (using default %.0f%% output ratio)\n",
			cost.DefaultOutputRatio*100)
		return nil
	}

	fmt.Fprintf(w, "\nCalibration (observed output_tokens / input_tokens):\n\n")
	fmt.Fprintf(w, "%-20s  %8s  %10s  %10s  %10s\n",
		"Tool kind", "Samples", "Mean ratio", "Std dev", "Conservative")
	fmt.Fprintf(w, "%-20s  %8s  %10s  %10s  %10s\n",
		"---------", "-------", "----------", "-------", "------------")

	kinds := make([]string, 0, len(stats))
	for k := range stats {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	for _, k := range kinds {
		s := stats[k]
		fmt.Fprintf(w, "%-20s  %8d  %9.1f%%  %9.1f%%  %9.1f%%\n",
			k, s.Count,
			s.MeanRatio*100,
			s.StdDev()*100,
			s.ConservativeRatio()*100)
	}

	return nil
}
