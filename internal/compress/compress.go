// Package compress provides deterministic output compressors for common
// tool output types. Compressors run before any LLM call and operate purely
// on the structure of the text — no network, no allocation beyond the output.
//
// A compressor that recognises its input returns Complete=true when the
// compressed result is sufficient on its own and no LLM is needed, or
// Complete=false when the result is a reduced input that still benefits from
// an LLM pass.
package compress

import "strings"

// Result is what a compressor returns.
type Result struct {
	Output   string
	Kind     string  // "go_test", "git_status", "git_diff", "git_log", "build", "generic_test", "" = no match
	Reduced  bool    // false when the input passed through unchanged
	Complete bool    // true when no LLM is needed; the output is already actionable
	Ratio    float64 // len(Output)/len(Input); 1.0 when unchanged
}

// compressor tries to compact input. It returns ok=true if this compressor
// recognised and handled the input.
type compressor func(input string) (output string, complete bool, ok bool)

// registry is ordered: earlier compressors take priority on ambiguous input.
var registry = []struct {
	kind string
	fn   compressor
}{
	{"go_test", compressGoTest},
	{"git_status", compressGitStatus},
	{"git_diff", compressGitDiff},
	{"git_log", compressGitLog},
	{"build", compressBuildOutput},
	{"generic_test", compressGenericTest},
}

// Compress runs the first matching compressor against input. When no
// compressor recognises the input, it returns the input unchanged with
// Kind="" and Reduced=false.
func Compress(input string) Result {
	for _, c := range registry {
		out, complete, ok := c.fn(input)
		if !ok {
			continue
		}
		ratio := 1.0
		if len(input) > 0 {
			ratio = float64(len(out)) / float64(len(input))
		}
		return Result{
			Output:   out,
			Kind:     c.kind,
			Reduced:  len(out) < len(input),
			Complete: complete,
			Ratio:    ratio,
		}
	}
	return Result{Output: input, Ratio: 1.0}
}

// hasAnyPrefix returns true if s starts with any of the prefixes.
func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
