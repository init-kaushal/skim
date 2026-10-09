// Package filemap generates compact structural digests of source files without
// an LLM worker call. It is the deterministic tier of the routing hierarchy:
//
//	cache hit → DETERMINISTIC (this package) → cheap worker → direct pass
//
// Currently supports Go source files via go/ast. Other languages fall through
// to the worker path.
package filemap

import (
	"path/filepath"
	"strings"

	"github.com/kaushal/skim/internal/digest"
)

// Generate attempts to build a FileMap for src without a worker call.
// filePath is used only to determine the file type. Returns (fm, true) when
// a deterministic map is available, (zero, false) when the file type is
// unsupported or the source is too broken to produce a useful map.
//
// Supported: .go (via go/ast), .ts/.tsx/.js/.jsx/.mjs/.cjs (regex scan),
// .py (regex scan). All other extensions fall through to the worker.
func Generate(src, filePath string) (digest.FileMap, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		// Exclude test files from the deterministic path: test helpers and
		// fixtures are rarely what the model needs a structural digest of, and
		// their unusual shapes (init-heavy, unexported helpers) produce noisier
		// maps than regular source.
		if strings.HasSuffix(filePath, "_test.go") {
			return digest.FileMap{}, false
		}
		fm, err := parseGoFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		fm, err := parseTSFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".py":
		fm, err := parsePyFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true
	}
	return digest.FileMap{}, false
}
