package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDoctor_ClaudeMissing_Fails(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	var out bytes.Buffer
	code := Doctor(&out, func(string) (string, error) { return "", errors.New("not found") })
	if code != 1 {
		t.Fatalf("exit = %d, want 1 when claude missing", code)
	}
	if !strings.Contains(out.String(), "claude") {
		t.Fatalf("report should mention claude: %q", out.String())
	}
}

func TestDoctor_AllGood_Passes(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	var out bytes.Buffer
	code := Doctor(&out, func(name string) (string, error) { return "/usr/bin/" + name, nil })
	if code != 0 {
		t.Fatalf("exit = %d, want 0; report:\n%s", code, out.String())
	}
}
