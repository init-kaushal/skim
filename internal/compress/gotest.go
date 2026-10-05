package compress

import (
	"fmt"
	"strings"
)

// compressGoTest compacts `go test` output.
// It preserves: FAIL lines, test failure bodies, panic output, coverage lines.
// It drops: PASS lines (except the summary), build progress, passing test names.
//
// NOTE: In go test output, failure detail lines (indented with tab) appear
// BEFORE their owning "--- FAIL" marker. So we keep any indented line that
// contains a file:line reference or error text, regardless of context.
func compressGoTest(input string) (string, bool, bool) {
	if !looksLikeGoTest(input) {
		return "", false, false
	}

	lines := strings.Split(input, "\n")
	var out []string
	var pass int

	for _, line := range lines {
		stripped := strings.TrimSpace(line)

		// Drop RUN / PAUSE / CONT markers — pure noise
		if hasAnyPrefix(stripped, "=== RUN", "=== PAUSE", "=== CONT") {
			continue
		}

		// Individual PASS lines: count and drop
		if strings.HasPrefix(stripped, "--- PASS") {
			pass++
			continue
		}

		// FAIL marker: always keep
		if strings.HasPrefix(stripped, "--- FAIL") {
			out = append(out, line)
			continue
		}

		// Package-level summary lines (ok  \t or FAIL\t)
		if (hasAnyPrefix(stripped, "ok  ", "FAIL", "?   ") && strings.Contains(stripped, "\t")) ||
			strings.HasPrefix(stripped, "coverage:") {
			out = append(out, line)
			continue
		}

		// panic output — always keep
		if hasAnyPrefix(stripped, "panic:", "goroutine ", "runtime/debug.Stack") ||
			containsAny(stripped, "goroutine 1 [running]") {
			out = append(out, line)
			continue
		}

		// Indented test body lines (tab prefix = failure detail in go test output)
		// Keep: file:line references, error text, or any content from a failing test
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			// Keep lines that are clearly failure details
			if containsAny(stripped, ".go:", "Error:", "error:", "Fatal:", "fatal:", "FAIL:") ||
				looksLikeFileLine(stripped) {
				out = append(out, line)
				continue
			}
			// Also keep other indented lines (could be t.Log output relevant to a failure)
			out = append(out, line)
			continue
		}

		// Stack trace lines
		if strings.HasPrefix(stripped, "testing.") {
			out = append(out, line)
			continue
		}

		// Bare "PASS" or "FAIL" summary
		if stripped == "PASS" || stripped == "FAIL" {
			out = append(out, line)
			continue
		}
	}

	if pass > 0 {
		out = append([]string{fmt.Sprintf("[%d tests passed — output omitted]", pass)}, out...)
	}

	result := strings.Join(out, "\n")
	if len(result) >= len(input) {
		return input, true, true
	}
	return result, true, true
}

// looksLikeFileLine returns true for "file.go:42:" style references.
func looksLikeFileLine(s string) bool {
	for i, r := range s {
		if r == ':' && i > 0 {
			after := s[i+1:]
			if len(after) > 0 && after[0] >= '0' && after[0] <= '9' {
				return true
			}
		}
	}
	return false
}

func looksLikeGoTest(s string) bool {
	return containsAny(s, "--- FAIL", "--- PASS", "=== RUN", "PASS\n", "FAIL\n") &&
		(strings.Contains(s, "\t") || strings.Contains(s, "go test"))
}
