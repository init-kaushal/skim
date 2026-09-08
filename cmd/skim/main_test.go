package main

import (
	"bytes"
	"strings"
	"testing"
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

func TestRun_KnownHookSubcommand_Exits0(t *testing.T) {
	// Hook subcommands must always exit 0. With empty stdin they allow (no-op).
	var out, errb bytes.Buffer
	if code := run([]string{"read-hook"}, &out, &errb); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}
