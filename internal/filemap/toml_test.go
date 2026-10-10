package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_TOML_CargoManifest(t *testing.T) {
	src := `[package]
name = "my-crate"
version = "0.1.0"
edition = "2021"
authors = ["Alice <alice@example.com>"]
description = "A sample Rust crate"

[dependencies]
serde = { version = "1", features = ["derive"] }
tokio = { version = "1", features = ["full"] }
clap = "4"

[dev-dependencies]
tempfile = "3"

[build-dependencies]
cc = "1"

[[bin]]
name = "my-app"
path = "src/main.rs"

[[bin]]
name = "my-tool"
path = "src/bin/tool.rs"

[profile.release]
opt-level = 3
lto = true
`
	fm, ok := filemap.Generate(src, "Cargo.toml")
	if !ok {
		t.Fatal("expected deterministic map for Cargo.toml")
	}

	if !strings.Contains(fm.Summary, "Cargo") {
		t.Errorf("expected Cargo summary, got: %s", fm.Summary)
	}

	// Key sections must appear as symbols.
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"[package]", "[dependencies]", "[dev-dependencies]"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}

	// [package] entry should list its keys.
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "[package]") {
			if !strings.Contains(e.Kind, "name") || !strings.Contains(e.Kind, "version") {
				t.Errorf("[package] entry should list keys, got: %s", e.Kind)
			}
			return
		}
	}
	t.Errorf("no [package] entry found: %v", fm.Map)
}

func TestGenerate_TOML_ArrayOfTablesMerged(t *testing.T) {
	// Multiple [[bin]] entries must appear as one merged symbol/entry, not N duplicates.
	src := `[package]
name = "multi-bin"
version = "0.1.0"

[[bin]]
name = "tool-a"
path = "src/a.rs"

[[bin]]
name = "tool-b"
path = "src/b.rs"

[[bin]]
name = "tool-c"
path = "src/c.rs"
`
	fm, ok := filemap.Generate(src, "Cargo.toml")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	binCount := 0
	for _, s := range fm.Symbols {
		if strings.Contains(s, "bin") {
			binCount++
		}
	}
	if binCount != 1 {
		t.Errorf("multiple [[bin]] entries should be merged to one symbol, got %d: %v", binCount, fm.Symbols)
	}
}

func TestGenerate_TOML_Pyproject(t *testing.T) {
	src := `[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[project]
name = "my-package"
version = "0.1.0"
description = "A Python package"
requires-python = ">=3.11"
dependencies = [
    "httpx>=0.27",
    "pydantic>=2",
]

[project.optional-dependencies]
dev = ["pytest>=8", "ruff"]

[tool.hatch.version]
path = "src/my_package/__init__.py"

[tool.ruff]
line-length = 100
`
	fm, ok := filemap.Generate(src, "pyproject.toml")
	if !ok {
		t.Fatal("expected deterministic map for pyproject.toml")
	}

	if !strings.Contains(fm.Summary, "Python") {
		t.Errorf("expected Python project summary, got: %s", fm.Summary)
	}

	// build-system and project must appear.
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"[build-system]", "[project]"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}
}

func TestGenerate_TOML_LineRangesPresent(t *testing.T) {
	src := `[package]
name = "test"
version = "0.1.0"

[dependencies]
serde = "1"
`
	fm, ok := filemap.Generate(src, "Cargo.toml")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}

	// [package] starts on line 1.
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "[package]") && e.Lines != "1-4" && e.Lines != "1-3" {
			// Allow either; blank line between sections may shift the count.
			if !strings.HasPrefix(e.Lines, "1") {
				t.Errorf("[package] should start on line 1, got: %s", e.Lines)
			}
		}
	}
}

func TestGenerate_TOML_RootKeysWithoutSection(t *testing.T) {
	// A TOML file that's all bare keys (no table headers) should still parse.
	src := `name = "flat"
version = "1.0"
debug = true
workers = 4
`
	fm, ok := filemap.Generate(src, "config.toml")
	if !ok {
		t.Fatal("expected deterministic map for flat TOML")
	}

	if !strings.Contains(fm.Map[0].Kind, "name") {
		t.Errorf("flat TOML root entry should list keys, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_TOML_FallsThrough_NoStructure(t *testing.T) {
	_, ok := filemap.Generate(strings.Repeat("just plain text here\n", 30), "notes.toml")
	if ok {
		t.Error("expected fallthrough for file with no TOML structure")
	}
}

func TestGenerate_TOML_FallsThrough_EmptyFile(t *testing.T) {
	_, ok := filemap.Generate("", "config.toml")
	if ok {
		t.Error("expected fallthrough for empty file")
	}
}
