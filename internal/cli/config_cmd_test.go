package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/config"
)

func TestConfig_NoArgs_PrintsConfig(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	var out bytes.Buffer
	if err := Config(&out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"model"`) {
		t.Fatalf("expected config JSON, got %q", out.String())
	}
}

func TestConfig_SetIntField(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	if err := Config(&bytes.Buffer{}, []string{"set", "read_max_lines", "150"}); err != nil {
		t.Fatal(err)
	}
	c, _ := config.Load()
	if c.ReadMaxLines != 150 {
		t.Fatalf("ReadMaxLines = %d, want 150", c.ReadMaxLines)
	}
}

func TestConfig_OffOn(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	Config(&bytes.Buffer{}, []string{"off"})
	if c, _ := config.Load(); !c.Disabled {
		t.Fatal("off should set disabled=true")
	}
	Config(&bytes.Buffer{}, []string{"on"})
	if c, _ := config.Load(); c.Disabled {
		t.Fatal("on should set disabled=false")
	}
}

func TestConfig_RejectsUnknownKey(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	if err := Config(&bytes.Buffer{}, []string{"set", "nope", "1"}); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
