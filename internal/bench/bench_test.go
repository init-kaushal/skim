// Package bench provides reproducible cost-comparison benchmarks for skim's
// routing strategies. Run with:
//
//	go test ./internal/bench/ -run TestBenchmark -v
//
// The results show estimated costs for:
//   - Baseline: all content passes through to the session model unchanged
//   - Deterministic: structural compressors only (no LLM, zero worker cost)
//   - CheapWorker: Haiku worker call (current skim behaviour)
//
// These numbers use model pricing from cost.KnownPricing and EstimateTokens.
// They are projections, not invoiced figures.
package bench_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/compress"
	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/metrics"
)

// fixture describes one benchmark scenario.
type fixture struct {
	Name  string
	Input string
}

// Fixtures represents common tool output shapes skim encounters. They are
// sized to be realistic rather than minimal so the cost comparison is meaningful.
var fixtures = []fixture{
	{
		Name:  "go_test_25_failures",
		Input: buildGoTestOutput(25),
	},
	{
		Name:  "git_diff_50_hunks",
		Input: buildGitDiff(50),
	},
	{
		Name:  "build_errors_30_files",
		Input: buildBuildErrors(30),
	},
	{
		Name:  "large_source_file",
		Input: strings.Repeat("// comment\nfunc DoSomething() {\n\treturn\n}\n", 150),
	},
}

// TestBenchmark prints a cost comparison table for each fixture. It does not
// call real APIs — all costs are computed from token estimates and pricing tables.
func TestBenchmark(t *testing.T) {
	sessionModel := "claude-opus-5-5"
	workerModel := "claude-haiku-4-5-20251001"
	compressionRatio := cost.DefaultOutputRatio // 0.30 — conservative estimate

	session := cost.KnownPricing[sessionModel]
	worker := cost.KnownPricing[workerModel]

	// Header
	fmt.Printf("\n%-30s  %10s  %12s  %12s  %10s  %10s\n",
		"Fixture", "OrigTokens", "BaselineCost", "DeterminCost", "WorkerCost", "NetSaving")
	fmt.Printf("%-30s  %10s  %12s  %12s  %10s  %10s\n",
		strings.Repeat("-", 30), strings.Repeat("-", 10), strings.Repeat("-", 12),
		strings.Repeat("-", 12), strings.Repeat("-", 10), strings.Repeat("-", 10))

	for _, f := range fixtures {
		origTokens := metrics.EstimateTokens(len(f.Input))
		cr := compress.Compress(f.Input)

		// Baseline: content enters session unchanged.
		baselineCost := costUSD(origTokens, session.InputPer1M)

		// Deterministic path: use the compressor output if available.
		deterministicCost := baselineCost // same as baseline if no compressor
		deterministicLabel := "(no compressor)"
		if cr.Reduced {
			compressedTokens := metrics.EstimateTokens(len(cr.Output))
			deterministicCost = costUSD(compressedTokens, session.InputPer1M)
			deterministicLabel = fmt.Sprintf("(%s,%.0f%%)", cr.Kind, cr.Ratio*100)
		}

		// CheapWorker path: Haiku produces a digest of compressionRatio size.
		workerInputTokens := origTokens + 650 // 650 = workerSystemOverhead
		workerOutputTokens := int(float64(origTokens) * compressionRatio)
		digestInSessionTokens := workerOutputTokens
		workerCallCost := costUSD(workerInputTokens, worker.InputPer1M) +
			costUSD(workerOutputTokens, worker.OutputPer1M)
		sessionWithDigestCost := costUSD(digestInSessionTokens, session.InputPer1M)
		cheapWorkerTotalCost := workerCallCost + sessionWithDigestCost
		netSaving := baselineCost - cheapWorkerTotalCost

		fmt.Printf("%-30s  %10d  $%11.5f  $%11.5f %s\n",
			f.Name, origTokens, baselineCost, deterministicCost, deterministicLabel)
		fmt.Printf("%-30s  %10s  %12s  $%11.5f  $%9.5f\n",
			"", "", "worker total:", cheapWorkerTotalCost, netSaving)

		// Validation: deterministic should never cost more than baseline.
		if deterministicCost > baselineCost+1e-9 {
			t.Errorf("%s: deterministic cost ($%.5f) > baseline ($%.5f) — compressor expanded the output",
				f.Name, deterministicCost, baselineCost)
		}
	}
	fmt.Println()
}

func costUSD(tokens int, pricePer1M float64) float64 {
	return float64(tokens) * pricePer1M / 1_000_000
}

// buildGoTestOutput generates realistic go test output with the given number
// of failing tests. Used as a benchmark fixture for the go_test compressor.
func buildGoTestOutput(failures int) string {
	var sb strings.Builder
	for i := range failures {
		sb.WriteString(fmt.Sprintf("=== RUN   TestFunction%d\n", i))
		sb.WriteString(fmt.Sprintf("    testfile_%d_test.go:%d: expected %d, got %d\n", i, i*3+10, i, i+1))
		sb.WriteString(fmt.Sprintf("--- FAIL: TestFunction%d (0.00s)\n", i))
	}
	// Add some passing tests for realism.
	for i := range 50 {
		sb.WriteString(fmt.Sprintf("=== RUN   TestPass%d\n", i))
		sb.WriteString(fmt.Sprintf("--- PASS: TestPass%d (0.00s)\n", i))
	}
	sb.WriteString("FAIL\n")
	sb.WriteString("FAIL\tgithub.com/example/pkg\t0.234s\n")
	return sb.String()
}

// buildGitDiff generates a realistic git diff with the given number of hunks.
func buildGitDiff(hunks int) string {
	var sb strings.Builder
	sb.WriteString("diff --git a/internal/service/handler.go b/internal/service/handler.go\n")
	sb.WriteString("index abc1234..def5678 100644\n")
	sb.WriteString("--- a/internal/service/handler.go\n")
	sb.WriteString("+++ b/internal/service/handler.go\n")
	for i := range hunks {
		start := i*20 + 1
		sb.WriteString(fmt.Sprintf("@@ -%d,6 +%d,8 @@\n", start, start))
		sb.WriteString(" func existingFunc() {\n")
		sb.WriteString(" \treturn nil\n")
		sb.WriteString(" }\n")
		sb.WriteString(fmt.Sprintf("+func newFeature%d() error {\n", i))
		sb.WriteString("+\t// Implementation\n")
		sb.WriteString("+\treturn nil\n")
		sb.WriteString("+}\n")
	}
	return sb.String()
}

// buildBuildErrors generates compiler error output for the given number of files.
func buildBuildErrors(fileCount int) string {
	var sb strings.Builder
	for i := range fileCount {
		line := i*5 + 10
		sb.WriteString(fmt.Sprintf("internal/pkg/file%d.go:%d:%d: undefined: SomeMissingSymbol%d\n",
			i, line, 4, i))
		sb.WriteString(fmt.Sprintf("internal/pkg/file%d.go:%d:%d: cannot use int as string\n",
			i, line+1, 12))
	}
	return sb.String()
}
