package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Makefile_BasicTargets(t *testing.T) {
	src := `.PHONY: build test clean lint

build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf dist/
`
	fm, ok := filemap.Generate(src, "Makefile")
	if !ok {
		t.Fatal("expected deterministic map for Makefile")
	}

	if !strings.Contains(fm.Summary, "4") {
		t.Errorf("expected 4-target summary, got: %s", fm.Summary)
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"build", "test", "lint", "clean"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q in %v", want, fm.Symbols)
		}
	}

	// PHONY targets should be flagged in the kind label.
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "build") && !strings.Contains(e.Kind, "phony") {
			t.Errorf("build is PHONY but label doesn't say so: %s", e.Kind)
		}
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}
}

func TestGenerate_Makefile_RecipeInKind(t *testing.T) {
	src := `build:
	go build -ldflags="-s -w" ./cmd/server
`
	fm, ok := filemap.Generate(src, "Makefile")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Map[0].Kind, "go build") {
		t.Errorf("build target should include first recipe line, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Makefile_FallsThrough_NoTargets(t *testing.T) {
	// A file with only variable assignments should fall through.
	src := "CC = gcc\nCFLAGS = -O2\n"
	_, ok := filemap.Generate(src, "Makefile")
	if ok {
		t.Error("expected fallthrough for Makefile with no targets")
	}
}

func TestGenerate_Makefile_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "Makefile")
	if ok {
		t.Error("expected fallthrough for empty Makefile")
	}
}

func TestGenerate_Makefile_DotMKExtension(t *testing.T) {
	src := "build:\n\tgcc -o app main.c\n"
	fm, ok := filemap.Generate(src, "rules.mk")
	if !ok {
		t.Fatal("expected deterministic map for .mk extension")
	}
	if !strings.Contains(fm.Symbols[0], "build") {
		t.Errorf("symbol should be 'build', got: %v", fm.Symbols)
	}
}

func TestGenerate_Makefile_LineRangesPresent(t *testing.T) {
	src := `build:
	go build ./...

test:
	go test ./...
`
	fm, ok := filemap.Generate(src, "Makefile")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}
	// build starts on line 1.
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("build target should start on line 1, got: %s", fm.Map[0].Lines)
	}
}
