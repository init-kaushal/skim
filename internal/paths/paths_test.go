package paths

import (
	"path/filepath"
	"testing"
)

func TestHome_UsesSkimHomeEnv(t *testing.T) {
	t.Setenv("SKIM_HOME", "/tmp/skim-x")
	if got := Home(); got != "/tmp/skim-x" {
		t.Fatalf("Home() = %q, want /tmp/skim-x", got)
	}
	if got := Config(); got != filepath.Join("/tmp/skim-x", "config.json") {
		t.Fatalf("Config() = %q", got)
	}
}
