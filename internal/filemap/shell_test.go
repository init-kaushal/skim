package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Shell_POSIXFunctions(t *testing.T) {
	src := `#!/bin/sh
# Utility helpers

log() {
	echo "[$(date)] $*" >&2
}

die() {
	log "FATAL: $*"
	exit 1
}

usage() {
	echo "Usage: $0 <command>"
	exit 0
}
`
	fm, ok := filemap.Generate(src, "helpers.sh")
	if !ok {
		t.Fatal("expected deterministic map for POSIX shell script")
	}

	if !strings.Contains(fm.Summary, "Shell script") {
		t.Errorf("expected Shell script summary, got: %s", fm.Summary)
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"log", "die", "usage"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q in %v", want, fm.Symbols)
		}
	}

	if len(fm.Map) != 3 {
		t.Errorf("expected 3 map entries, got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_Shell_BashStyleFunctions(t *testing.T) {
	src := `#!/usr/bin/env bash

function setup_env {
	export APP_ENV=${APP_ENV:-development}
	export LOG_LEVEL=${LOG_LEVEL:-info}
}

function start_server {
	setup_env
	./bin/server --port 8080
}

function run_tests {
	go test ./...
}
`
	fm, ok := filemap.Generate(src, "dev.sh")
	if !ok {
		t.Fatal("expected deterministic map for bash-style functions")
	}

	names := map[string]bool{}
	for _, s := range fm.Symbols {
		names[s] = true
	}
	for _, want := range []string{"setup_env", "start_server", "run_tests"} {
		if !names[want] {
			t.Errorf("expected function %q in symbols %v", want, fm.Symbols)
		}
	}
}

func TestGenerate_Shell_LineRangesNoOverlap(t *testing.T) {
	src := `#!/bin/bash

alpha() {
	echo alpha
}

beta() {
	echo beta
}

gamma() {
	echo gamma
}
`
	fm, ok := filemap.Generate(src, "script.sh")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(fm.Map))
	}
	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("entry has empty Lines: %+v", e)
		}
	}
	// alpha must start on line 3.
	if !strings.HasPrefix(fm.Map[0].Lines, "3") {
		t.Errorf("alpha should start on line 3, got: %s", fm.Map[0].Lines)
	}
}

func TestGenerate_Shell_NoFunctions_ScriptBody(t *testing.T) {
	// A script with only top-level statements (no functions) still gets a map.
	src := `#!/bin/sh
export PATH="$HOME/.local/bin:$PATH"
export GOPATH="$HOME/go"
source ~/.aliases
`
	fm, ok := filemap.Generate(src, "env.sh")
	if !ok {
		t.Fatal("expected deterministic map for function-free script")
	}
	if len(fm.Map) != 1 {
		t.Errorf("expected single script body entry, got %d: %v", len(fm.Map), fm.Map)
	}
	if fm.Map[0].Kind != "script body" {
		t.Errorf("expected kind 'script body', got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Shell_ZshKind(t *testing.T) {
	src := `#!/usr/bin/env zsh
greet() { echo "hello $1" }
`
	fm, ok := filemap.Generate(src, "greet.zsh")
	if !ok {
		t.Fatal("expected deterministic map for .zsh file")
	}
	if !strings.Contains(fm.Summary, "Zsh") {
		t.Errorf("expected Zsh summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Shell_SourcesInNotes(t *testing.T) {
	src := `#!/bin/bash
. /etc/os-release
source "$HOME/.config/common.sh"

main() {
	echo "Running on $ID"
}
`
	fm, ok := filemap.Generate(src, "bootstrap.sh")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	// Source statements should appear in Notes.
	if fm.Notes == "" || !strings.Contains(fm.Notes, "sources:") {
		t.Errorf("expected sources in Notes, got: %q", fm.Notes)
	}
}

func TestGenerate_Shell_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "script.sh")
	if ok {
		t.Error("expected fallthrough for empty shell script")
	}
}

func TestGenerate_Shell_FallsThrough_NoShellStructure(t *testing.T) {
	// A plain .sh file with no shebang and no functions should fall through.
	src := "just some text\nno functions here\n"
	_, ok := filemap.Generate(src, "notes.sh")
	if ok {
		t.Error("expected fallthrough for .sh file with no shell structure")
	}
}

func TestGenerate_Shell_BashrcBasename(t *testing.T) {
	src := `# ~/.bashrc
export EDITOR=vim

greet_user() {
	echo "Welcome, $USER"
}
`
	fm, ok := filemap.Generate(src, "/home/user/.bashrc")
	if !ok {
		t.Fatal("expected deterministic map for .bashrc")
	}
	if !strings.Contains(fm.Summary, "Bash") {
		t.Errorf("expected Bash summary for .bashrc, got: %s", fm.Summary)
	}
}

// TestGenerate_Shell_GeneratedMarkerAfterShebang is the regression test for the
// bug where the generated-detection loop broke on the shebang and never
// examined subsequent lines.
func TestGenerate_Shell_GeneratedMarkerAfterShebang(t *testing.T) {
	src := `#!/usr/bin/env bash
# @generated — DO NOT EDIT

do_thing() {
	echo "thing"
}
`
	fm, ok := filemap.Generate(src, "generated.sh")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in Summary for @generated script, got: %s", fm.Summary)
	}
}

// TestGenerate_Shell_BraceInString is the regression test for the known
// limitation that brace counting is not shell-aware. A closing brace inside
// a double-quoted string can cause the function's recorded end line to be
// shorter than the true function body. The test documents the current
// behaviour; a full fix is deferred.
func TestGenerate_Shell_BraceInString(t *testing.T) {
	src := `#!/bin/bash
# function that contains a brace inside a string
describe() {
	echo "result: }"
	echo "done"
}

other() {
	echo "other"
}
`
	fm, ok := filemap.Generate(src, "brace.sh")
	// The map may or may not be accurate here — the test only checks that the
	// parser does not crash and still produces entries for both functions.
	if !ok {
		t.Fatal("expected deterministic map (even if line ranges are imprecise)")
	}
	names := map[string]bool{}
	for _, s := range fm.Symbols {
		names[s] = true
	}
	if !names["describe"] {
		t.Errorf("expected 'describe' in symbols %v", fm.Symbols)
	}
	if !names["other"] {
		t.Errorf("expected 'other' in symbols %v", fm.Symbols)
	}
}

func TestGenerate_Shell_DotShExtension(t *testing.T) {
	src := `#!/bin/sh
build() { go build ./...; }
test_all() { go test ./...; }
`
	fm, ok := filemap.Generate(src, "ci.sh")
	if !ok {
		t.Fatal("expected deterministic map for .sh extension")
	}
	if len(fm.Symbols) < 2 {
		t.Errorf("expected at least 2 symbols, got: %v", fm.Symbols)
	}
}
