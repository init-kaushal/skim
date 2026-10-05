package compress

import (
	"strings"
)

// compressBuildOutput compacts compiler output (go build, rustc, tsc, etc.).
// Preserves: error lines with file:line references, summary lines.
// Drops: info/progress lines, verbose dependency output.
func compressBuildOutput(input string) (string, bool, bool) {
	if !looksLikeBuildOutput(input) {
		return "", false, false
	}

	lines := strings.Split(input, "\n")
	var out []string
	var errorCount int

	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}

		// Error/warning lines with file:line:col or file:line references
		if isErrorLine(t) {
			out = append(out, line)
			errorCount++
			continue
		}

		// Build summary lines
		if hasAnyPrefix(t, "FAILED", "error[", "warning[", "note:", "help:") ||
			strings.HasPrefix(t, "aborting") ||
			strings.Contains(t, "error(s)") ||
			strings.Contains(t, "warning(s)") {
			out = append(out, line)
			continue
		}

		// go build/vet specific
		if strings.HasPrefix(t, "#") && strings.Contains(t, "/") {
			// Package annotation: # path/to/pkg
			out = append(out, line)
			continue
		}

		// TypeScript errors
		if strings.Contains(t, "TS") && strings.Contains(t, "error") {
			out = append(out, line)
			continue
		}

		// Drop: download/fetching progress, module resolution
		if hasAnyPrefix(t, "go:", "downloading", "fetching", "verifying",
			"Downloading", "Compiling", "Finished", "Blocking") {
			continue
		}
	}

	if len(out) == 0 {
		return "", false, false
	}

	result := strings.Join(out, "\n")
	if len(result) >= len(input) {
		return input, true, true
	}
	return result, true, true
}

// compressGenericTest compacts generic test output (pytest, jest, cargo test, npm test).
// Falls back for output that looks test-ish but isn't go test.
func compressGenericTest(input string) (string, bool, bool) {
	if !looksLikeGenericTest(input) {
		return "", false, false
	}

	lines := strings.Split(input, "\n")
	var out []string
	inFailure := false

	for _, line := range lines {
		t := strings.TrimSpace(line)

		// pytest: PASSED/FAILED/ERROR summary
		if strings.HasSuffix(t, " PASSED") || strings.HasSuffix(t, " PASSED)") {
			continue
		}
		if strings.HasSuffix(t, " FAILED") || strings.HasSuffix(t, " FAILED)") ||
			strings.HasSuffix(t, " ERROR") {
			out = append(out, line)
			inFailure = true
			continue
		}

		// Summary lines
		if containsAny(t, "passed", "failed", "error", "warning") &&
			containsAny(t, "===", "---", "FAILED", "passed in") {
			out = append(out, line)
			inFailure = false
			continue
		}

		// DOTS (pytest progress)
		if isDotLine(t) {
			continue
		}

		if inFailure {
			out = append(out, line)
			if t == "" {
				inFailure = false
			}
		}

		// Always keep error lines
		if isErrorLine(t) {
			out = append(out, line)
		}
	}

	if len(out) == 0 {
		return "", false, false
	}
	result := strings.Join(out, "\n")
	if len(result) >= len(input) {
		return input, true, true
	}
	return result, true, true
}

// isErrorLine returns true for lines that look like compiler/test error references.
func isErrorLine(s string) bool {
	// file.go:42: or file.go:42:10: pattern
	for i, r := range s {
		if r == ':' && i > 0 {
			// Check what follows
			after := s[i+1:]
			if len(after) > 0 {
				c := after[0]
				if c >= '0' && c <= '9' {
					return true
				}
			}
			// Check what precedes (should look like a filename)
			before := s[:i]
			if strings.ContainsAny(before, "./\\") {
				return true
			}
		}
	}
	return false
}

func looksLikeBuildOutput(s string) bool {
	return containsAny(s, "./...", "build failed", "FAILED", "compile error",
		": undefined:", ": cannot use", ": no field", ": declared and not used",
		"TS2", "error TS") ||
		(strings.Contains(s, "#") && strings.Contains(s, "go build"))
}

func looksLikeGenericTest(s string) bool {
	return containsAny(s, "PASSED", "FAILED", "pytest", "jest", "mocha",
		"cargo test", "npm test", "✓", "✗", "✘")
}

func isDotLine(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != '.' && c != 'F' && c != 'E' && c != 's' && c != 'x' && c != ' ' {
			return false
		}
	}
	return len(s) > 3
}
