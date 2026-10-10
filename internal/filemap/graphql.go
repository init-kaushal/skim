package filemap

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kaushal/skim/internal/digest"
)

var (
	// gqlTopBlock matches a top-level GraphQL definition opener.
	// Groups: (1) full keyword (may include "extend "), (2) name
	// "schema" and unnamed operations are handled separately.
	gqlTopBlock = regexp.MustCompile(
		`^((?:extend\s+)?(?:type|input|interface|enum|fragment|query|mutation|subscription))\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// gqlAnonymousOp matches anonymous operation definitions: "query {", "mutation {", "subscription {"
	gqlAnonymousOp = regexp.MustCompile(`^(query|mutation|subscription)\s*\{`)
	// gqlSchemaBlock matches the bare "schema {" block.
	gqlSchemaBlock = regexp.MustCompile(`^schema\s*\{`)
	// gqlUnion matches a union definition (no body braces).
	gqlUnion = regexp.MustCompile(`^(?:extend\s+)?union\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// gqlScalar matches a scalar definition (single line, no body).
	gqlScalar = regexp.MustCompile(`^(?:extend\s+)?scalar\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// gqlComment matches a GraphQL line comment.
	gqlComment = regexp.MustCompile(`^\s*#`)
	// gqlGenerated detects generated GraphQL files.
	gqlGenerated = regexp.MustCompile(`(?i)@generated|DO NOT EDIT|auto-?generated|do not edit|This file was automatically generated`)
)

// gqlBlock tracks one top-level GraphQL definition while parsing.
type gqlBlock struct {
	kind      string // "type", "input", "interface", "enum", "union", "scalar", "fragment", "query", "mutation", "subscription", "schema"
	name      string // empty for anonymous operations and schema block
	startLine int
	endLine   int
}

// parseGraphQLFile builds a FileMap from a GraphQL file. Each top-level
// definition (type, input, interface, enum, union, scalar, fragment, named
// or anonymous operation) becomes one map entry.
func parseGraphQLFile(src, _ string) (digest.FileMap, bool) {
	lines := splitLines(src)
	if len(lines) == 0 {
		return digest.FileMap{}, false
	}

	if !hasGQLStructure(lines) {
		return digest.FileMap{}, false
	}

	generated := false
	for _, l := range lines[:min(len(lines), 5)] {
		if gqlGenerated.MatchString(l) {
			generated = true
			break
		}
	}

	var blocks []*gqlBlock
	var cur *gqlBlock
	var curUnion *gqlBlock // tracks a possibly multiline union definition
	depth := 0
	inTripleQuote := false
	anonOpCount := 0

	closeBlock := func(endLine int) {
		if cur != nil {
			cur.endLine = endLine
			blocks = append(blocks, cur)
			cur = nil
		}
	}
	closeUnion := func() {
		if curUnion != nil {
			blocks = append(blocks, curUnion)
			curUnion = nil
		}
	}

	for i, raw := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" {
			// A blank line closes any pending multiline union.
			closeUnion()
			continue
		}

		// Triple-quoted block strings span multiple lines. Toggle state on each
		// line that has an odd number of """ delimiters (covers both the open
		// line and close line of a multi-line description).
		tripleCount := strings.Count(trimmed, `"""`)
		if inTripleQuote {
			if tripleCount%2 == 1 {
				inTripleQuote = false
			}
			continue
		}
		if tripleCount%2 == 1 {
			inTripleQuote = true
			continue
		}

		if gqlComment.MatchString(raw) {
			continue
		}

		// Multiline union continuation: lines starting with | extend the range.
		if curUnion != nil {
			if strings.HasPrefix(trimmed, "|") {
				curUnion.endLine = lineNo
				continue
			}
			// Any other non-blank, non-comment line ends the union; fall through
			// to process this line as the next definition.
			closeUnion()
		}

		if cur != nil {
			depth += gqlNetBraces(raw)
			if depth <= 0 {
				closeBlock(lineNo)
				depth = 0
			}
			continue
		}

		// Unions have no body braces but can span multiple lines with | members.
		// Enter curUnion state so continuation lines are captured.
		if m := gqlUnion.FindStringSubmatch(trimmed); m != nil {
			curUnion = &gqlBlock{kind: "union", name: m[1], startLine: lineNo, endLine: lineNo}
			continue
		}
		if m := gqlScalar.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, &gqlBlock{kind: "scalar", name: m[1], startLine: lineNo, endLine: lineNo})
			continue
		}

		// schema { } block
		if gqlSchemaBlock.MatchString(trimmed) {
			cur = &gqlBlock{kind: "schema", name: "", startLine: lineNo}
			depth = gqlNetBraces(raw)
			if depth <= 0 {
				closeBlock(lineNo)
				depth = 0
			}
			continue
		}

		// Anonymous operations: "query {", "mutation {", "subscription {"
		if m := gqlAnonymousOp.FindStringSubmatch(trimmed); m != nil {
			anonOpCount++
			cur = &gqlBlock{
				kind:      m[1],
				name:      fmt.Sprintf("(anonymous #%d)", anonOpCount),
				startLine: lineNo,
			}
			depth = gqlNetBraces(raw)
			if depth <= 0 {
				closeBlock(lineNo)
				depth = 0
			}
			continue
		}

		// Named definitions: type/input/interface/enum/fragment/query/mutation/subscription
		if m := gqlTopBlock.FindStringSubmatch(trimmed); m != nil {
			kw := m[1]
			kind := gqlNormalizeKind(kw)
			cur = &gqlBlock{kind: kind, name: m[2], startLine: lineNo}
			depth = gqlNetBraces(raw)
			if depth <= 0 {
				closeBlock(lineNo)
				depth = 0
			}
			continue
		}
	}
	closeUnion()
	closeBlock(len(lines))

	if len(blocks) == 0 {
		return digest.FileMap{}, false
	}

	var entries []digest.MapEntry
	var symbols []string

	for _, b := range blocks {
		label := gqlBlockLabel(b)
		if len(symbols) < 40 {
			sym := b.kind
			if b.name != "" {
				sym += " " + b.name
			}
			symbols = append(symbols, sym)
		}
		entries = append(entries, digest.MapEntry{
			Lines: lineRange(b.startLine, b.endLine),
			Kind:  label,
		})
	}

	summary := gqlSummary(blocks, generated)

	return digest.FileMap{
		Summary: summary,
		Map:     entries,
		Symbols: symbols,
	}, true
}

// gqlNetBraces counts net brace depth change for a GraphQL line, ignoring
// braces inside double-quoted strings and after # comments.
func gqlNetBraces(line string) int {
	depth := 0
	inStr := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inStr {
			if ch == '\\' {
				i++
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '#':
			return depth
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	return depth
}

// gqlNormalizeKind converts a matched keyword (which may include "extend ")
// into a concise kind string.
func gqlNormalizeKind(kw string) string {
	kw = strings.TrimSpace(kw)
	if strings.HasPrefix(kw, "extend ") {
		// "extend type" → "type" (we don't distinguish extends in the kind for brevity)
		return strings.TrimSpace(strings.TrimPrefix(kw, "extend "))
	}
	return kw
}

// gqlBlockLabel builds the display label for one GraphQL definition.
func gqlBlockLabel(b *gqlBlock) string {
	if b.name == "" || b.kind == "schema" {
		return b.kind
	}
	return b.kind + " " + b.name
}

// gqlSummary builds the top-level summary line.
func gqlSummary(blocks []*gqlBlock, generated bool) string {
	counts := map[string]int{}
	for _, b := range blocks {
		counts[b.kind]++
	}

	// Split into schema types and operations for a smarter prefix.
	schemaKinds := []string{"type", "input", "interface", "enum", "union", "scalar", "schema"}
	opKinds := []string{"query", "mutation", "subscription", "fragment"}

	schemaCount := 0
	for _, k := range schemaKinds {
		schemaCount += counts[k]
	}
	opCount := 0
	for _, k := range opKinds {
		opCount += counts[k]
	}

	prefix := "GraphQL"
	if schemaCount > 0 && opCount == 0 {
		prefix = "GraphQL schema"
	} else if opCount > 0 && schemaCount == 0 {
		prefix = "GraphQL operations"
	}

	ordered := []string{"type", "input", "interface", "enum", "union", "scalar", "query", "mutation", "subscription", "fragment", "schema"}
	var parts []string
	for _, t := range ordered {
		n := counts[t]
		if n == 0 {
			continue
		}
		if n == 1 {
			parts = append(parts, fmt.Sprintf("1 %s", t))
		} else {
			parts = append(parts, fmt.Sprintf("%d %ss", n, t))
		}
	}

	summary := prefix + " — " + strings.Join(parts, ", ")
	if generated {
		summary += " — generated, do not edit directly"
	}
	return summary
}

// hasGQLStructure returns true when at least one recognized GraphQL definition
// is present.
func hasGQLStructure(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if gqlTopBlock.MatchString(t) ||
			gqlAnonymousOp.MatchString(t) ||
			gqlSchemaBlock.MatchString(t) ||
			gqlUnion.MatchString(t) ||
			gqlScalar.MatchString(t) {
			return true
		}
	}
	return false
}
