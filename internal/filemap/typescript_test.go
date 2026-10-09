package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_TypeScript_BasicExports(t *testing.T) {
	src := `import { useState } from 'react';
import type { FC } from 'react';

export interface ButtonProps {
  label: string;
  onClick: () => void;
}

export const Button: FC<ButtonProps> = ({ label, onClick }) => {
  return <button onClick={onClick}>{label}</button>;
};

export function formatDate(d: Date): string {
  return d.toISOString().split('T')[0];
}

export class DateFormatter {
  format(d: Date): string {
    return d.toISOString();
  }
}
`
	fm, ok := filemap.Generate(src, "button.tsx")
	if !ok {
		t.Fatal("expected deterministic map for .tsx file")
	}

	if !strings.Contains(fm.Summary, "TypeScript") {
		t.Errorf("summary should mention TypeScript, got: %s", fm.Summary)
	}

	// Expect symbols: ButtonProps, Button, formatDate, DateFormatter
	wantSymbols := []string{"ButtonProps", "Button", "formatDate", "DateFormatter"}
	for _, sym := range wantSymbols {
		found := false
		for _, s := range fm.Symbols {
			if s == sym {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected symbol %q in %v", sym, fm.Symbols)
		}
	}

	// Expect at least an imports entry and several declaration entries.
	if len(fm.Map) < 3 {
		t.Errorf("expected >= 3 map entries, got %d: %v", len(fm.Map), fm.Map)
	}

	// First entry should be imports.
	if len(fm.Map) > 0 && fm.Map[0].Kind != "imports" {
		t.Errorf("first map entry should be imports, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_TypeScript_FallsThrough_NoStructure(t *testing.T) {
	// A file with no recognizable JS/TS structure should fall through.
	src := strings.Repeat("some random text here\n", 50)
	_, ok := filemap.Generate(src, "notes.ts")
	if ok {
		t.Error("expected fallthrough for file with no JS/TS structure")
	}
}

func TestGenerate_TypeScript_Minified(t *testing.T) {
	// A single very long line (minified) should fall through.
	src := strings.Repeat("x", 5000) + "\n"
	_, ok := filemap.Generate(src, "bundle.js")
	if ok {
		t.Error("expected fallthrough for minified JS")
	}
}

func TestGenerate_JavaScript_CommonJS(t *testing.T) {
	src := `const path = require('path');
const fs = require('fs');

function readConfig(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

class ConfigLoader {
  constructor(basePath) {
    this.basePath = basePath;
  }

  load(name) {
    return readConfig(path.join(this.basePath, name + '.json'));
  }
}

module.exports = { ConfigLoader, readConfig };
`
	fm, ok := filemap.Generate(src, "config.js")
	if !ok {
		t.Fatal("expected deterministic map for .js file")
	}

	if !strings.Contains(fm.Summary, "JavaScript") {
		t.Errorf("expected JavaScript in summary, got: %s", fm.Summary)
	}

	// readConfig and ConfigLoader should be identified.
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["readConfig"] {
		t.Errorf("expected readConfig in symbols, got: %v", fm.Symbols)
	}
	if !symbolSet["ConfigLoader"] {
		t.Errorf("expected ConfigLoader in symbols, got: %v", fm.Symbols)
	}
}

func TestGenerate_TypeScript_EnumAndType(t *testing.T) {
	src := `export enum Status {
  Active = 'active',
  Inactive = 'inactive',
  Pending = 'pending',
}

export type UserId = string;

export interface User {
  id: UserId;
  name: string;
  status: Status;
}
`
	fm, ok := filemap.Generate(src, "types.ts")
	if !ok {
		t.Fatal("expected deterministic map for .ts file")
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}

	for _, want := range []string{"Status", "UserId", "User"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, symbols: %v", want, fm.Symbols)
		}
	}
}

func TestGenerate_TypeScript_ImportsBlock(t *testing.T) {
	src := `import React from 'react';
import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';

export const App = () => <div>hello</div>;
`
	fm, ok := filemap.Generate(src, "app.tsx")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	if len(fm.Map) == 0 || fm.Map[0].Kind != "imports" {
		t.Errorf("expected first entry to be imports, got: %v", fm.Map)
	}
	// Import block should span lines 1-3.
	if fm.Map[0].Lines != "1-3" {
		t.Errorf("import block lines: want 1-3, got %s", fm.Map[0].Lines)
	}
}

func TestGenerate_UnsupportedExtension_NoTS(t *testing.T) {
	src := `export function foo() { return 1; }`
	_, ok := filemap.Generate(src, "script.rb")
	if ok {
		t.Error("expected fallthrough for unsupported extension .rb")
	}
}
