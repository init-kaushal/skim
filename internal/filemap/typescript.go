package filemap

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kaushal/skim/internal/digest"
)

// TypeScript/JavaScript patterns checked against the first non-whitespace
// content of each line. All are anchored to the start of the trimmed line.
var (
	tsImportLine = regexp.MustCompile(`^import\s+`)
	tsRequire    = regexp.MustCompile(`^(?:const|let|var)\s+\w+\s*=\s*require\s*\(`)

	// Named exports: export [default] [abstract] [async] function|class|interface|type|enum|const|let|var Name
	tsExportNamed = regexp.MustCompile(
		`^export\s+(?:default\s+)?(?:abstract\s+)?(?:async\s+)?(function\*?|class|interface|type|enum|const|let|var)\s+(\w+)`)
	// export default function/class (anonymous or named)
	tsExportDefault = regexp.MustCompile(`^export\s+default\s+(?:async\s+)?(?:function\*?|class)\b`)
	// Unexported top-level function or class
	tsFuncDecl  = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\*?\s+(\w+)`)
	tsClassDecl = regexp.MustCompile(`^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)`)
	// Arrow / const that may define a React component or utility fn
	tsArrowConst = regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*[:=]`)

	// Generated-file markers (same heuristic as Go).
	tsGenerated = regexp.MustCompile(`@generated|DO NOT EDIT|Code generated`)
)

// parseTSFile builds a FileMap from a TypeScript or JavaScript source file
// using regex-based line scanning. It is intentionally coarser than the Go
// AST parser: ranges are approximate and some nested constructs are elided.
// The goal is a useful structural digest, not perfect fidelity.
func parseTSFile(src, filePath string) (digest.FileMap, error) {
	lines := splitLines(src)
	if len(lines) == 0 {
		return digest.FileMap{}, fmt.Errorf("filemap: empty file")
	}

	// Validity guard: reject files that have no recognizable JS/TS structure.
	// Minified files, binary garbage, or random text fall through to the worker.
	if !hasTSStructure(lines) {
		return digest.FileMap{}, fmt.Errorf("filemap: no recognizable JS/TS structure")
	}

	generated := false
	for _, l := range lines[:min(len(lines), 5)] {
		if tsGenerated.MatchString(l) {
			generated = true
			break
		}
	}

	// Section scanner: walk lines, detect top-level declarations.
	type section struct {
		start int
		end   int // inclusive, 1-based
		kind  string
		name  string
	}

	var sections []section
	var symbols []string
	seenSymbols := map[string]bool{}

	addSymbol := func(name string) {
		if name != "" && !seenSymbols[name] && len(symbols) < 40 {
			symbols = append(symbols, name)
			seenSymbols[name] = true
		}
	}

	// importEnd: last line of the import block; 0 means not yet seen.
	importStart, importEnd := 0, 0

	// Brace-depth tracker: once we enter a top-level block (depth > 0), we
	// count braces until we return to depth 0.
	depth := 0
	declStart := 0
	declKind := ""
	declName := ""
	inDecl := false

	closeSection := func(endLine int) {
		if inDecl && declStart > 0 {
			sections = append(sections, section{declStart, endLine, declKind, declName})
			inDecl = false
			declStart = 0
			declKind = ""
			declName = ""
		}
	}

	for i, raw := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			// Empty lines extend the current section but don't start one.
			if inDecl {
				for _, ch := range raw {
					if ch == '{' {
						depth++
					} else if ch == '}' {
						depth--
						if depth < 0 {
							depth = 0
						}
					}
				}
				if depth == 0 && inDecl {
					closeSection(lineNo)
				}
			}
			continue
		}

		// Count braces on this line.
		opened, closed := countBraces(raw)

		if inDecl {
			// Still inside a block: just track depth.
			depth += opened - closed
			if depth < 0 {
				depth = 0
			}
			if depth == 0 {
				closeSection(lineNo)
			}
			continue
		}

		// Not in a decl: look for a new top-level declaration.

		// Import / require lines
		if tsImportLine.MatchString(trimmed) || tsRequire.MatchString(trimmed) {
			if importStart == 0 {
				importStart = lineNo
			}
			importEnd = lineNo
			continue
		}

		// If import block was active and we hit something that's not an import,
		// the import block is closed (importEnd is already set).

		// Named export
		if m := tsExportNamed.FindStringSubmatch(trimmed); m != nil {
			keyword, name := m[1], m[2]
			addSymbol(name)
			kind := tsKindLabel(keyword, name, trimmed)
			declStart = lineNo
			declKind = kind
			declName = name
			depth = opened - closed
			if depth <= 0 {
				// Single-line declaration (type alias, interface with no body, etc.)
				sections = append(sections, section{lineNo, lineNo, kind, name})
				depth = 0
			} else {
				inDecl = true
			}
			continue
		}

		// export default function/class
		if tsExportDefault.MatchString(trimmed) {
			declStart = lineNo
			declKind = "export default"
			declName = ""
			depth = opened - closed
			if depth <= 0 {
				sections = append(sections, section{lineNo, lineNo, "export default", ""})
				depth = 0
			} else {
				inDecl = true
			}
			continue
		}

		// Unexported function
		if m := tsFuncDecl.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(trimmed, "export") {
			name := m[1]
			addSymbol(name)
			declStart = lineNo
			declKind = "function " + name
			declName = name
			depth = opened - closed
			if depth <= 0 {
				sections = append(sections, section{lineNo, lineNo, "function " + name, name})
				depth = 0
			} else {
				inDecl = true
			}
			continue
		}

		// Unexported class
		if m := tsClassDecl.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(trimmed, "export") {
			name := m[1]
			addSymbol(name)
			declStart = lineNo
			declKind = "class " + name
			declName = name
			depth = opened - closed
			if depth <= 0 {
				sections = append(sections, section{lineNo, lineNo, "class " + name, name})
				depth = 0
			} else {
				inDecl = true
			}
			continue
		}

		// const/let/var arrow functions (not already caught by tsExportNamed)
		if m := tsArrowConst.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(trimmed, "export") {
			name := m[1]
			// Only track if the value looks like a function.
			if strings.Contains(raw, "=>") || strings.Contains(raw, "function") {
				addSymbol(name)
				declStart = lineNo
				declKind = "const " + name
				declName = name
				depth = opened - closed
				if depth <= 0 {
					sections = append(sections, section{lineNo, lineNo, "const " + name, name})
					depth = 0
				} else {
					inDecl = true
				}
			}
			continue
		}
	}

	// Close any still-open section at EOF.
	if inDecl {
		closeSection(len(lines))
	}

	// Build entries.
	var entries []digest.MapEntry

	if importStart > 0 {
		entries = append(entries, digest.MapEntry{
			Lines: lineRange(importStart, importEnd),
			Kind:  "imports",
		})
	}

	for _, s := range sections {
		entries = append(entries, digest.MapEntry{
			Lines: lineRange(s.start, s.end),
			Kind:  s.kind,
		})
	}

	// Fallback: if we got nothing useful, one entry covering the whole file.
	if len(entries) == 0 {
		entries = []digest.MapEntry{{Lines: lineRange(1, len(lines)), Kind: "module body"}}
	}

	// Summary: count exports for a concise description.
	exportCount := 0
	for _, sym := range symbols {
		_ = sym
		exportCount++
	}

	ext := strings.ToLower(filePath[strings.LastIndexByte(filePath, '.')+1:])
	lang := "JavaScript"
	if ext == "ts" || ext == "tsx" {
		lang = "TypeScript"
	}

	var summaryParts []string
	summaryParts = append(summaryParts, fmt.Sprintf("%s module", lang))
	if exportCount > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d exported symbol(s)", exportCount))
	}
	if generated {
		summaryParts = append(summaryParts, "generated — do not edit directly")
	}
	summary := strings.Join(summaryParts, " — ")

	var notes string
	if generated {
		notes = "generated file — edit the generator, not this file"
	}

	return digest.FileMap{
		Summary: summary,
		Map:     entries,
		Symbols: symbols,
		Notes:   notes,
	}, nil
}

// hasTSStructure returns true when the source contains at least one recognizable
// JavaScript/TypeScript construct. Rejects minified files and binary noise.
func hasTSStructure(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if tsImportLine.MatchString(t) || tsRequire.MatchString(t) ||
			tsExportNamed.MatchString(t) || tsExportDefault.MatchString(t) ||
			tsFuncDecl.MatchString(t) || tsClassDecl.MatchString(t) {
			return true
		}
	}
	return false
}

// countBraces returns the number of { and } characters on a line, ignoring
// those inside string literals (best-effort: skips single, double, template
// quoted strings but does not handle escaped quotes inside them).
func countBraces(line string) (open, close int) {
	inStr := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inStr != 0 {
			if ch == '\\' {
				i++ // skip escaped char
				continue
			}
			if ch == inStr {
				inStr = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inStr = ch
		case '{':
			open++
		case '}':
			close++
		case '/':
			// Line comment: stop counting.
			if i+1 < len(line) && line[i+1] == '/' {
				return
			}
		}
	}
	return
}

// tsKindLabel builds the map entry kind label for a named export.
func tsKindLabel(keyword, name, line string) string {
	switch keyword {
	case "function", "function*":
		return fmt.Sprintf("export function %s", name)
	case "class":
		if strings.Contains(line, "abstract") {
			return fmt.Sprintf("export abstract class %s", name)
		}
		return fmt.Sprintf("export class %s", name)
	case "interface":
		return fmt.Sprintf("export interface %s", name)
	case "type":
		return fmt.Sprintf("export type %s", name)
	case "enum":
		return fmt.Sprintf("export enum %s", name)
	case "const", "let", "var":
		return fmt.Sprintf("export %s %s", keyword, name)
	}
	return fmt.Sprintf("export %s %s", keyword, name)
}

