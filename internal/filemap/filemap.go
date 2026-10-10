// Package filemap generates compact structural digests of source files without
// an LLM worker call. It is the deterministic tier of the routing hierarchy:
//
//	cache hit → DETERMINISTIC (this package) → cheap worker → direct pass
//
// Supported languages and formats:
//   - .go       — via go/ast; full symbol table with line ranges
//   - .ts/.tsx/.js/.jsx/.mjs/.cjs — regex scan; imports, exports, classes, functions
//   - .py       — regex scan; classes with method lists, functions, imports
//   - .rs       — regex scan; pub/private filtering, traits, enums, impl blocks
//   - .json     — encoding/json; top-level keys with value shapes; lock files excluded
//   - .yml/.yaml — indent scan; Docker Compose, Kubernetes, GitHub Actions recognized
//   - .toml     — line scan; table headers, Cargo.toml and pyproject.toml recognized
//   - Dockerfile / *.dockerfile — FROM stages with instruction verb summary
//   - Makefile / GNUmakefile / *.mk — explicit targets with first recipe line
//   - .sh/.bash/.zsh/.fish and common rc/profile names — function extraction
//   - .tf/.hcl — resource, data, variable, output, module, provider blocks; lock file excluded
//   - .proto   — message, service (with RPC list), enum, extend blocks
//   - .sql     — CREATE TABLE/INDEX/VIEW/FUNCTION/PROCEDURE, ALTER TABLE DDL
//
// All other extensions fall through to the worker path.
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
// .py (regex scan), .rs (regex scan), .json (encoding/json),
// .yml/.yaml (indent scan). Lock files are excluded even when they use a
// supported extension.
func Generate(src, filePath string) (digest.FileMap, bool) {
	// Lock files contain version-pinned dependency graphs that the model must
	// read verbatim. A structural digest is actively misleading.
	base := strings.ToLower(filepath.Base(filePath))
	if lockFileNames[base] {
		return digest.FileMap{}, false
	}

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

	case ".rs":
		fm, err := parseRustFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".json":
		fm, err := parseJSONFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".yml", ".yaml":
		fm, err := parseYAMLFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".toml":
		if tomlLockFileNames[base] {
			return digest.FileMap{}, false
		}
		fm, err := parseTOMLFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true
	}

	// Filename-based dispatch for files without a conventional extension.
	switch base {
	case "dockerfile", "containerfile":
		fm, err := parseDockerfile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case "makefile", "gnumakefile", "makefile.inc":
		fm, err := parseMakefile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true
	}

	// Extension-based dispatch for less common names.
	switch ext {
	case ".dockerfile":
		fm, err := parseDockerfile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".mk", ".make":
		fm, err := parseMakefile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".sh", ".bash", ".zsh", ".fish", ".ksh", ".dash":
		fm, err := parseShellFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".tf", ".hcl":
		if hclLockFileNames[base] {
			return digest.FileMap{}, false
		}
		fm, err := parseHCLFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".proto":
		fm, err := parseProtoFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true

	case ".sql":
		fm, err := parseSQLFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true
	}

	// Shell rc/profile files have no extension; match by basename.
	switch base {
	case ".bashrc", ".bash_profile", ".bash_logout", ".bash_aliases",
		".zshrc", ".zshenv", ".zprofile", ".zlogin", ".zlogout",
		".profile", ".shrc",
		"install.sh", "setup.sh", "run.sh", "entrypoint.sh":
		fm, err := parseShellFile(src, filePath)
		if err != nil {
			return digest.FileMap{}, false
		}
		return fm, true
	}

	return digest.FileMap{}, false
}
