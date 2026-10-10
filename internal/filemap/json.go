package filemap

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kaushal/skim/internal/digest"
)

// lockFileNames is the set of JSON filenames whose exact content must reach
// the model unchanged. A structural digest of a lock file is actively
// unhelpful — the model needs package versions, not a summary.
var lockFileNames = map[string]bool{
	"package-lock.json": true,
	"npm-shrinkwrap.json": true,
	"composer.lock": true,
}

// parseJSONFile builds a FileMap from a JSON source file.
//
// Objects produce one map entry per top-level key with its value's type and
// shape (scalar, object with N keys, array of N items). Arrays produce a
// single entry describing the element count and homogeneity.
//
// Line ranges use json.Decoder.InputOffset() for byte-accurate positions.
// The decoder stops at the end of each value, so ranges are tight.
func parseJSONFile(src, filePath string) (digest.FileMap, error) {
	if src == "" {
		return digest.FileMap{}, fmt.Errorf("filemap: empty file")
	}

	dec := json.NewDecoder(strings.NewReader(src))

	// Peek at the outermost token to distinguish object from array.
	first, err := dec.Token()
	if err != nil {
		return digest.FileMap{}, fmt.Errorf("filemap: invalid JSON: %w", err)
	}
	delim, ok := first.(json.Delim)
	if !ok {
		return digest.FileMap{}, fmt.Errorf("filemap: JSON root is not an object or array")
	}

	// Line-offset index: maps byte offsets → line numbers (1-based).
	// Built once and reused for every entry.
	lineOf := buildLineIndex(src)

	switch delim {
	case '{':
		return parseJSONObject(src, dec, lineOf, filePath)
	case '[':
		return parseJSONArray(src, dec, lineOf, filePath)
	default:
		return digest.FileMap{}, fmt.Errorf("filemap: unexpected JSON delimiter %q", delim)
	}
}

// parseJSONObject handles a JSON object whose opening `{` has already been
// consumed by the caller's dec.Token() call.
func parseJSONObject(src string, dec *json.Decoder, lineOf func(int64) int, filePath string) (digest.FileMap, error) {
	type keyEntry struct {
		key       string
		startLine int
		endLine   int
		shape     string
	}

	var entries []keyEntry

	for dec.More() {
		startOff := dec.InputOffset()

		keyTok, err := dec.Token()
		if err != nil {
			return digest.FileMap{}, fmt.Errorf("filemap: reading JSON key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return digest.FileMap{}, fmt.Errorf("filemap: expected string key, got %T", keyTok)
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return digest.FileMap{}, fmt.Errorf("filemap: decoding value for %q: %w", key, err)
		}
		endOff := dec.InputOffset()

		startLine := lineOf(startOff)
		endLine := lineOf(endOff)
		// The decoder offset sits just after the value; back off one line when
		// it lands on an opening brace/bracket of the next entry.
		if endLine > startLine+1 {
			endLine--
		}

		entries = append(entries, keyEntry{
			key:       key,
			startLine: startLine,
			endLine:   endLine,
			shape:     describeJSONValue(raw),
		})
	}

	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return digest.FileMap{}, fmt.Errorf("filemap: incomplete JSON object: %w", err)
	}

	// Reject trailing non-whitespace after the document.
	if rest := strings.TrimSpace(src[int(dec.InputOffset()):]); rest != "" {
		return digest.FileMap{}, fmt.Errorf("filemap: trailing content after JSON document")
	}

	if len(entries) == 0 {
		return digest.FileMap{}, fmt.Errorf("filemap: empty JSON object")
	}

	// Symbols: top-level key names.
	symbols := make([]string, 0, len(entries))
	for _, e := range entries {
		if len(symbols) < 40 {
			symbols = append(symbols, e.key)
		}
	}

	// Map entries.
	mapEntries := make([]digest.MapEntry, 0, len(entries))
	for _, e := range entries {
		mapEntries = append(mapEntries, digest.MapEntry{
			Lines: lineRange(e.startLine, e.endLine),
			Kind:  fmt.Sprintf("%s: %s", e.key, e.shape),
		})
	}

	// Summary: describe the file by its well-known shape or by key count.
	summary := describeJSONObjectSummary(symbols, filePath)

	return digest.FileMap{
		Summary: summary,
		Map:     mapEntries,
		Symbols: symbols,
	}, nil
}

// parseJSONArray handles a JSON array whose opening `[` has already been
// consumed. Arrays are common as data files (e.g. tsconfig paths, fixture
// sets) and produce a single entry describing element count and homogeneity.
func parseJSONArray(src string, dec *json.Decoder, lineOf func(int64) int, filePath string) (digest.FileMap, error) {
	var elements []json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return digest.FileMap{}, fmt.Errorf("filemap: decoding JSON array element: %w", err)
		}
		elements = append(elements, raw)
	}

	// Consume the closing ']'.
	if _, err := dec.Token(); err != nil {
		return digest.FileMap{}, fmt.Errorf("filemap: incomplete JSON array: %w", err)
	}

	// Reject trailing non-whitespace after the document.
	if rest := strings.TrimSpace(src[int(dec.InputOffset()):]); rest != "" {
		return digest.FileMap{}, fmt.Errorf("filemap: trailing content after JSON document")
	}

	if len(elements) == 0 {
		return digest.FileMap{}, fmt.Errorf("filemap: empty JSON array")
	}

	// Describe element type homogeneity.
	firstType := jsonTypeName(elements[0])
	homogeneous := true
	for _, el := range elements[1:] {
		if jsonTypeName(el) != firstType {
			homogeneous = false
			break
		}
	}
	typeDesc := firstType
	if !homogeneous {
		typeDesc = "mixed"
	}

	totalLines := strings.Count(src, "\n") + 1
	summary := fmt.Sprintf("JSON array — %d %s element(s)", len(elements), typeDesc)

	return digest.FileMap{
		Summary: summary,
		Map: []digest.MapEntry{{
			Lines: lineRange(1, totalLines),
			Kind:  fmt.Sprintf("array of %d %s items", len(elements), typeDesc),
		}},
	}, nil
}

// describeJSONValue returns a compact human-readable description of a JSON
// value: the scalar value for short strings/numbers, or a shape description
// for objects and arrays.
func describeJSONValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	switch raw[0] {
	case '{':
		// Count keys.
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			if len(m) == 0 {
				return "object (empty)"
			}
			// Collect and sort keys for deterministic output. Map iteration
			// order is random in Go, so the same nested object would produce
			// different digest strings across runs — hurting cache hit rates.
			all := make([]string, 0, len(m))
			for k := range m {
				all = append(all, k)
			}
			sort.Strings(all)
			if len(m) <= 4 {
				return fmt.Sprintf("object {%s}", strings.Join(all, ", "))
			}
			return fmt.Sprintf("object {%s, ...} (%d keys)", strings.Join(all[:4], ", "), len(m))
		}
		return "object"
	case '[':
		var a []json.RawMessage
		if json.Unmarshal(raw, &a) == nil {
			return fmt.Sprintf("array[%d]", len(a))
		}
		return "array"
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if len(s) <= 40 {
				return fmt.Sprintf("%q", s)
			}
			return fmt.Sprintf("string (%d chars)", len(s))
		}
		return "string"
	case 't', 'f':
		return string(raw)
	case 'n':
		return "null"
	default:
		// Number.
		if len(raw) <= 12 {
			return string(raw)
		}
		return "number"
	}
}

// jsonTypeName returns the JSON type name for the first byte of a raw value.
func jsonTypeName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	switch raw[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// describeJSONObjectSummary builds the Summary field from the file's well-known
// key shapes (package.json, tsconfig.json, etc.) or falls back to a key count.
func describeJSONObjectSummary(keys []string, filePath string) string {
	base := strings.ToLower(filePath)
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}

	switch base {
	case "package.json":
		return "npm package manifest"
	case "tsconfig.json", "jsconfig.json":
		return "TypeScript/JavaScript compiler config"
	case ".eslintrc.json", "eslint.config.json":
		return "ESLint config"
	case ".babelrc", ".babelrc.json":
		return "Babel config"
	case "jest.config.json":
		return "Jest test config"
	case "composer.json":
		return "PHP Composer manifest"
	}

	return fmt.Sprintf("JSON object — %d top-level key(s): %s",
		len(keys), strings.Join(keys[:min(len(keys), 6)], ", "))
}

// buildLineIndex returns a function that maps a byte offset (from
// json.Decoder.InputOffset) to a 1-based line number. It scans src once on
// construction; lookups are O(log n) via binary search.
func buildLineIndex(src string) func(int64) int {
	// newlineOffsets[i] is the byte offset of the i-th '\n'.
	var offsets []int64
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			offsets = append(offsets, int64(i))
		}
	}
	return func(byteOff int64) int {
		// Binary search: count how many newlines fall before byteOff.
		lo, hi := 0, len(offsets)
		for lo < hi {
			mid := (lo + hi) / 2
			if offsets[mid] < byteOff {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		return lo + 1 // 1-based
	}
}
