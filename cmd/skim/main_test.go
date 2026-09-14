package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/config"
)

func TestRun_NoArgs_PrintsUsageAndExits2(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(nil, &out, &errb)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: skim") {
		t.Fatalf("stderr = %q, want it to contain %q", errb.String(), "usage: skim")
	}
}

func TestRun_UnknownSubcommand_Exits2(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"wat"}, &out, &errb); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

// TestRun_OffOnSubcommands_Dispatch covers the kill switch advertised in
// plugin/commands/skim.md, which passes $ARGUMENTS through verbatim: a bare
// `skim off` / `skim on` must toggle config.disabled, not print usage and exit 2.
func TestRun_OffOnSubcommands_Dispatch(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())

	for _, tc := range []struct {
		arg  string
		want bool
	}{{"off", true}, {"on", false}} {
		var out, errb bytes.Buffer
		if code := run([]string{tc.arg}, &out, &errb); code != 0 {
			t.Fatalf("skim %s: exit = %d, want 0 (stderr: %s)", tc.arg, code, errb.String())
		}
		c, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		if c.Disabled != tc.want {
			t.Errorf("after `skim %s`, Disabled = %v, want %v", tc.arg, c.Disabled, tc.want)
		}
	}
}

func TestRun_KnownHookSubcommand_Exits0(t *testing.T) {
	// Hook subcommands must always exit 0. With empty stdin they allow (no-op).
	var out, errb bytes.Buffer
	if code := run([]string{"read-hook"}, &out, &errb); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}
