package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_WritesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKIM_HOME", dir)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("Model = %q, want default", c.Model)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("expected config.json to be written: %v", err)
	}
}

func TestLoad_OverlaysFileOntoDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKIM_HOME", dir)
	os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"read_max_lines": 42}`), 0o644)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ReadMaxLines != 42 {
		t.Fatalf("ReadMaxLines = %d, want 42 (from file)", c.ReadMaxLines)
	}
	if c.GrepMaxMatches != Default().GrepMaxMatches {
		t.Fatalf("GrepMaxMatches = %d, want default (unspecified in file)", c.GrepMaxMatches)
	}
}

func TestValidate_RejectsEmptyModelAndBadThresholds(t *testing.T) {
	c := Default()
	c.Model = ""
	if c.Validate() == nil {
		t.Fatal("empty model should be invalid")
	}
	c = Default()
	c.ReadMaxBytes = 0
	if c.Validate() == nil {
		t.Fatal("zero ReadMaxBytes should be invalid")
	}
	c = Default()
	c.BashNoisyPatterns = []string{"("}
	if c.Validate() == nil {
		t.Fatal("uncompilable regex should be invalid")
	}
}

func TestNotActiveReason_Precedence(t *testing.T) {
	c := Default()
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if r := c.NotActiveReason(env(map[string]string{"SKIM_ACTIVE": "1"})); r != "recursion guard" {
		t.Fatalf("got %q", r)
	}
	if r := c.NotActiveReason(env(map[string]string{"SKIM_DISABLED": "1"})); r != "SKIM_DISABLED" {
		t.Fatalf("got %q", r)
	}
	if r := c.NotActiveReason(env(nil)); r != "" {
		t.Fatalf("default config should be active, got %q", r)
	}
	bad := Default()
	bad.Model = ""
	if r := bad.NotActiveReason(env(nil)); r == "" {
		t.Fatal("invalid config should be not-active")
	}
}
