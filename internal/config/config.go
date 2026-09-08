package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kaushal/skim/internal/paths"
)

type Config struct {
	Disabled          bool     `json:"disabled"`
	ReadMaxLines      int      `json:"read_max_lines"`
	ReadMaxBytes      int      `json:"read_max_bytes"`
	GrepMaxMatches    int      `json:"grep_max_matches"`
	BashNoisyPatterns []string `json:"bash_noisy_patterns"`
	PassthroughGlobs  []string `json:"passthrough_globs"`
	Model             string   `json:"model"`
	WorkerTimeoutSec  int      `json:"worker_timeout_sec"`
}

func Default() Config {
	return Config{
		Disabled:       false,
		ReadMaxLines:   300,
		ReadMaxBytes:   60000,
		GrepMaxMatches: 60,
		BashNoisyPatterns: []string{
			`\bcat\s`, `\bcurl\s`, `\btail\s+-n\s+\d{3,}`,
			`npm\s+(run\s+)?test`, `jest`, `pytest`, `go\s+test`,
		},
		PassthroughGlobs: []string{"**/*.md", "**/go.mod", ".claude/**"},
		Model:            "claude-haiku-4-5-20251001",
		WorkerTimeoutSec: 45,
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
