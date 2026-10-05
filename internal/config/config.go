package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kaushal/skim/internal/cost"
	"github.com/kaushal/skim/internal/paths"
)

// maxWorkerTimeoutSec is the safety ceiling for worker_timeout_sec. The hook
// must finish, process startup included, inside the 60s timeout skim declares
// for itself in plugin/hooks/hooks.json. (Claude Code's own default for command
// hooks is 600s, so the binding constraint is skim's declared value, not the
// harness — a hook that stalls a session for ten minutes is the worse failure.)
//
// The default stays at 45s rather than something tighter because that is what
// real work needs: a live digest of a 92 KB source file measured 22s, and the
// same file timed out at a 30s budget. Below ~45s skim stops digesting exactly
// the large files it exists for and silently degrades open instead.
const maxWorkerTimeoutSec = 55

type Config struct {
	Disabled          bool     `json:"disabled"`
	ReadMaxLines      int      `json:"read_max_lines"`
	ReadMaxBytes      int      `json:"read_max_bytes"`
	GrepMaxMatches    int      `json:"grep_max_matches"`
	BashNoisyPatterns []string `json:"bash_noisy_patterns"`
	PassthroughGlobs  []string `json:"passthrough_globs"`
	Model             string   `json:"model"`
	WorkerTimeoutSec  int      `json:"worker_timeout_sec"`

	// Cost-aware routing (Phase 1)
	// SessionModel is the Claude Code session model used to price savings.
	// Defaults to claude-opus-5-5 (the most common expensive model).
	SessionModel string `json:"session_model"`
	// SafetyMargin multiplies the worker cost before comparing to expected
	// savings. 1.5 means we only intercept when savings are 50% more than cost.
	SafetyMargin float64 `json:"safety_margin"`
	// MinSavingsUSD is the minimum net saving (savings − worker_cost) required
	// to justify an interception. Prevents micro-interceptions.
	MinSavingsUSD float64 `json:"min_savings_usd"`
	// DryRun runs the decision engine but always allows the tool call through.
	// Decisions are still logged to decisions.jsonl for `skim explain`.
	DryRun bool `json:"dry_run"`

	// ModelPricingOverrides overrides pricing for specific models. Useful when
	// Anthropic changes prices or you have committed pricing from a contract.
	// Keys must match the model IDs used in Model and SessionModel.
	// Overrides are merged into KnownPricing at startup; unset models keep defaults.
	// Example: {"claude-opus-5-5": {"input_per_1m": 12.00, "output_per_1m": 60.00, ...}}
	ModelPricingOverrides map[string]cost.ModelPricing `json:"model_pricing_overrides,omitempty"`
}

func Default() Config {
	return Config{
		Disabled:       false,
		ReadMaxLines:   150,
		ReadMaxBytes:   60000,
		GrepMaxMatches: 60,
		BashNoisyPatterns: []string{
			`\bcat\s`, `\bcurl\s`, `\btail\s+-n\s+\d{3,}`,
			`npm\s+(run\s+)?test`, `jest`, `pytest`, `go\s+test`,
		},
		PassthroughGlobs: []string{"**/*.md", "**/go.mod", ".claude/**"},
		Model:            "claude-haiku-4-5-20251001",
		WorkerTimeoutSec: 45,
		SessionModel:     "claude-opus-5-5",
		SafetyMargin:     1.5,
		MinSavingsUSD:    0.001,
		DryRun:           false,
	}
}

func Load() (Config, error) {
	c := Default()
	b, err := os.ReadFile(paths.Config())
	if errors.Is(err, os.ErrNotExist) {
		return c, Save(c)
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", paths.Config(), err)
	}
	return c, nil
}

func Save(c Config) error {
	if err := os.MkdirAll(filepath.Dir(paths.Config()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.Config(), append(b, '\n'), 0o644)
}

func (c Config) Validate() error {
	if c.Model == "" {
		return errors.New("model must not be empty")
	}
	if c.ReadMaxLines <= 0 || c.ReadMaxBytes <= 0 ||
		c.GrepMaxMatches <= 0 || c.WorkerTimeoutSec <= 0 {
		return errors.New("thresholds and timeout must be > 0")
	}
	if c.SafetyMargin < 0 {
		return errors.New("safety_margin must be >= 0")
	}
	if c.MinSavingsUSD < 0 {
		return errors.New("min_savings_usd must be >= 0")
	}
	// A worker allowed to outlive the hook timeout skim declares for itself
	// gets killed by the harness mid-call instead of degrading open cleanly.
	if c.WorkerTimeoutSec > maxWorkerTimeoutSec {
		return fmt.Errorf("worker_timeout_sec must be <= %d (the hook timeout skim declares)", maxWorkerTimeoutSec)
	}
	for _, p := range c.BashNoisyPatterns {
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("bad noisy pattern %q: %w", p, err)
		}
	}
	return nil
}

func (c Config) NotActiveReason(env func(string) string) string {
	if env("SKIM_ACTIVE") == "1" {
		return "recursion guard"
	}
	if env("SKIM_DISABLED") == "1" {
		return "SKIM_DISABLED"
	}
	if c.Disabled {
		return "config.disabled"
	}
	if err := c.Validate(); err != nil {
		return "invalid config: " + err.Error()
	}
	return ""
}
