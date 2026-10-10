package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_JSON_PackageJSON(t *testing.T) {
	src := `{
  "name": "my-app",
  "version": "1.2.3",
  "description": "A sample application",
  "scripts": {
    "build": "tsc",
    "test": "jest",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "typescript": "^5.0.0",
    "jest": "^29.0.0"
  }
}
`
	fm, ok := filemap.Generate(src, "package.json")
	if !ok {
		t.Fatal("expected deterministic map for package.json")
	}

	if !strings.Contains(fm.Summary, "npm package manifest") {
		t.Errorf("expected npm package manifest summary, got: %s", fm.Summary)
	}

	// Top-level keys must be in symbols.
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"name", "version", "scripts", "dependencies", "devDependencies"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}

	// scripts entry should describe it as an object.
	hasScripts := false
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "scripts:") {
			hasScripts = true
			if !strings.Contains(e.Kind, "object") && !strings.Contains(e.Kind, "build") {
				t.Errorf("scripts entry should describe object contents, got: %s", e.Kind)
			}
		}
	}
	if !hasScripts {
		t.Errorf("expected scripts entry in map: %v", fm.Map)
	}
}

func TestGenerate_JSON_LineRangesAccurate(t *testing.T) {
	src := `{
  "first": "one",
  "second": {
    "a": 1,
    "b": 2
  },
  "third": [1, 2, 3]
}
`
	fm, ok := filemap.Generate(src, "data.json")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	// "first" should be on line 2.
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "first:") {
			if e.Lines != "2" {
				t.Errorf("first key: want line 2, got %s", e.Lines)
			}
		}
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}
}

func TestGenerate_JSON_FallsThrough_InvalidJSON(t *testing.T) {
	_, ok := filemap.Generate(`{not valid json`, "config.json")
	if ok {
		t.Error("expected fallthrough for invalid JSON")
	}
}

func TestGenerate_JSON_FallsThrough_LockFile(t *testing.T) {
	// package-lock.json must never be digested — it must be read verbatim.
	src := `{"lockfileVersion": 3, "packages": {}}`
	_, ok := filemap.Generate(src, "package-lock.json")
	if ok {
		t.Error("package-lock.json must fall through to the worker")
	}
}

func TestGenerate_JSON_FallsThrough_ComposerLock(t *testing.T) {
	src := `{"_readme": ["This file is locked"], "packages": []}`
	_, ok := filemap.Generate(src, "composer.lock")
	if ok {
		t.Error("composer.lock must fall through to the worker")
	}
}

func TestGenerate_JSON_TSConfig(t *testing.T) {
	src := `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "outDir": "./dist",
    "rootDir": "./src"
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
`
	fm, ok := filemap.Generate(src, "tsconfig.json")
	if !ok {
		t.Fatal("expected deterministic map for tsconfig.json")
	}

	if !strings.Contains(fm.Summary, "TypeScript") {
		t.Errorf("expected TypeScript summary, got: %s", fm.Summary)
	}
}

func TestGenerate_JSON_Array(t *testing.T) {
	src := `[
  {"id": 1, "name": "Alice"},
  {"id": 2, "name": "Bob"},
  {"id": 3, "name": "Carol"}
]
`
	fm, ok := filemap.Generate(src, "users.json")
	if !ok {
		t.Fatal("expected deterministic map for JSON array")
	}

	if !strings.Contains(fm.Summary, "3") {
		t.Errorf("array summary should mention element count, got: %s", fm.Summary)
	}
}

func TestGenerate_JSON_EmptyObject(t *testing.T) {
	_, ok := filemap.Generate(`{}`, "empty.json")
	if ok {
		t.Error("expected fallthrough for empty JSON object")
	}
}

func TestGenerate_JSON_FallsThrough_TruncatedObject(t *testing.T) {
	// A partial object must never produce a map — the digest would silently
	// omit configuration that was present in the original file.
	cases := []string{
		`{"name": "foo", "version":`,
		`{"name": "foo"`,
		`{"name": `,
		`{`,
	}
	for _, src := range cases {
		_, ok := filemap.Generate(src, "config.json")
		if ok {
			t.Errorf("truncated JSON %q must fall through, got a map", src)
		}
	}
}

func TestGenerate_JSON_FallsThrough_TrailingGarbage(t *testing.T) {
	// Valid JSON followed by non-whitespace trailing content must fall through.
	// The decoder would silently ignore the extra content; we must reject it.
	cases := []string{
		`{"name": "foo"} garbage`,
		`{"name": "foo"}{"b": 2}`,
		`[1, 2, 3] extra`,
	}
	for _, src := range cases {
		_, ok := filemap.Generate(src, "config.json")
		if ok {
			t.Errorf("JSON with trailing content %q must fall through, got a map", src)
		}
	}
}

func TestGenerate_JSON_FallsThrough_TrailingWhitespaceAllowed(t *testing.T) {
	// Trailing whitespace after a valid document is normal and must not cause fallthrough.
	src := `{"name": "ok"}` + "\n   \n"
	_, ok := filemap.Generate(src, "config.json")
	if !ok {
		t.Error("trailing whitespace after valid JSON must not cause fallthrough")
	}
}

func TestGenerate_JSON_NestedKeysDeterministic(t *testing.T) {
	// The nested-object description must be stable across runs regardless of
	// Go map iteration order. Run multiple times and verify the output is
	// identical — a cache key built from this description must be stable.
	src := `{"opts": {"c": 3, "a": 1, "b": 2}}`
	var last string
	for i := 0; i < 20; i++ {
		fm, ok := filemap.Generate(src, "data.json")
		if !ok {
			t.Fatal("expected deterministic map")
		}
		got := fm.Map[0].Kind
		if last != "" && got != last {
			t.Errorf("iteration %d: got %q, prev run gave %q — output is non-deterministic", i, got, last)
		}
		last = got
	}
	// Also verify the keys appear in sorted order (a, b, c alphabetically).
	if !strings.Contains(last, "{a, b, c}") {
		t.Errorf("nested keys not in sorted order: %s (want {a, b, c})", last)
	}
}
