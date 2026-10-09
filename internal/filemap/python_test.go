package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Python_BasicModule(t *testing.T) {
	src := `"""Utility functions for data processing."""

import os
import json
from typing import Optional, List

def load_config(path: str) -> dict:
    """Load configuration from a JSON file."""
    with open(path) as f:
        return json.load(f)

def save_config(path: str, config: dict) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, 'w') as f:
        json.dump(config, f, indent=2)

class ConfigManager:
    """Manages application configuration."""

    def __init__(self, base_path: str):
        self.base_path = base_path

    def load(self, name: str) -> dict:
        return load_config(os.path.join(self.base_path, name))

    def save(self, name: str, config: dict) -> None:
        save_config(os.path.join(self.base_path, name), config)
`
	fm, ok := filemap.Generate(src, "config.py")
	if !ok {
		t.Fatal("expected deterministic map for .py file")
	}

	// Summary should come from the docstring.
	if !strings.Contains(fm.Summary, "Utility functions") {
		t.Errorf("expected docstring summary, got: %s", fm.Summary)
	}

	// Symbols: load_config, save_config, ConfigManager
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"load_config", "save_config", "ConfigManager"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}

	// Should have imports entry.
	hasImports := false
	for _, e := range fm.Map {
		if e.Kind == "imports" {
			hasImports = true
			break
		}
	}
	if !hasImports {
		t.Errorf("expected imports entry in map: %v", fm.Map)
	}

	// ConfigManager class entry should mention its methods.
	hasClass := false
	for _, e := range fm.Map {
		if strings.Contains(e.Kind, "class ConfigManager") {
			hasClass = true
			break
		}
	}
	if !hasClass {
		t.Errorf("expected class ConfigManager entry in map: %v", fm.Map)
	}
}

func TestGenerate_Python_FallsThrough_NoStructure(t *testing.T) {
	src := strings.Repeat("not python at all\n", 30)
	_, ok := filemap.Generate(src, "notes.py")
	if ok {
		t.Error("expected fallthrough for file with no Python structure")
	}
}

func TestGenerate_Python_EmptyFile(t *testing.T) {
	_, ok := filemap.Generate("", "empty.py")
	if ok {
		t.Error("expected fallthrough for empty file")
	}
}

func TestGenerate_Python_ClassWithMethods(t *testing.T) {
	src := `class Router:
    def __init__(self):
        self.routes = {}

    def get(self, path, handler):
        self.routes[path] = handler

    def post(self, path, handler):
        self.routes[path] = handler

    def handle(self, request):
        handler = self.routes.get(request.path)
        if handler:
            return handler(request)
        return None
`
	fm, ok := filemap.Generate(src, "router.py")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["Router"] {
		t.Errorf("expected Router in symbols: %v", fm.Symbols)
	}

	// Class entry should list some methods.
	for _, e := range fm.Map {
		if strings.Contains(e.Kind, "class Router") {
			if !strings.Contains(e.Kind, "methods:") {
				t.Errorf("class entry should list methods, got: %s", e.Kind)
			}
			return
		}
	}
	t.Errorf("no class Router entry found: %v", fm.Map)
}

func TestGenerate_Python_OnlyImports(t *testing.T) {
	// A file with only imports (e.g., __init__.py) should produce a map.
	src := `from .config import Config
from .router import Router
from .utils import helpers

__all__ = ['Config', 'Router', 'helpers']
`
	fm, ok := filemap.Generate(src, "__init__.py")
	if !ok {
		t.Fatal("expected deterministic map for __init__.py")
	}

	hasImports := false
	for _, e := range fm.Map {
		if e.Kind == "imports" {
			hasImports = true
		}
	}
	if !hasImports {
		t.Errorf("expected imports entry: %v", fm.Map)
	}
}

func TestGenerate_Python_AsyncFunctions(t *testing.T) {
	src := `import asyncio

async def fetch_data(url: str) -> dict:
    async with aiohttp.ClientSession() as session:
        async with session.get(url) as resp:
            return await resp.json()

async def process_batch(items: list) -> list:
    tasks = [fetch_data(item) for item in items]
    return await asyncio.gather(*tasks)
`
	fm, ok := filemap.Generate(src, "client.py")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"fetch_data", "process_batch"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}
}
