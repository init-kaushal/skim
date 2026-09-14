// Package rg locates and drives ripgrep on skim's behalf.
//
// It exists as its own package because two callers need the same resolution
// logic — the Grep hook, which counts and samples matches, and `skim doctor`,
// which has to report honestly whether Grep interception can work at all.
package rg

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kaushal/skim/internal/hookio"
)

// sampleCap bounds the context dump fed to the worker as the Grep sample.
const sampleCap = 32768

// Path resolves the ripgrep binary, preferring a standalone install and
// falling back to Claude Code's own executable.
//
// This fallback is not a nicety. `exec.Command("rg", …)` performs a PATH lookup
// for an *executable*, and on a normal Claude Code install there is no `rg`
// executable to find: Claude Code ships ripgrep inside its own binary and, on
// at least some setups, exposes it as a shell *function* that re-execs
// `claude` with argv[0] set to "rg". A shell function is invisible to
// exec.Command, so Count failed with "executable file not found", the Grep
// hook degraded open, and Grep interception silently never ran at all —
// verified on a machine where `which rg` printed a zsh function and no `rg`
// binary existed anywhere on disk.
//
// Invoked with argv[0] == "rg", the Claude Code binary reports itself as
// ripgrep 14.1.1 and accepts the full ripgrep CLI, so it is a complete
// substitute rather than a partial shim. skim already requires `claude` to be
// present for the worker, which makes this a strictly weaker dependency than
// standalone ripgrep.
//
// The second return value is the argv[0] to run it under: callers must set
// cmd.Args[0] to it, mirroring `exec -a rg`.
func Path() (path, argv0 string, err error) {
	if p, lerr := exec.LookPath("rg"); lerr == nil {
		return p, p, nil
	}
	p, lerr := exec.LookPath("claude")
	if lerr != nil {
		return "", "", fmt.Errorf("neither rg nor claude found on PATH: %w", lerr)
	}
	return p, "rg", nil
}

// Command builds an exec.Cmd that runs ripgrep with args, whichever way it
// had to be resolved.
func Command(args ...string) (*exec.Cmd, error) {
	path, argv0, err := Path()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	cmd.Args[0] = argv0
	return cmd, nil
}

// matchArgs returns the args that decide WHICH LINES MATCH: the scope the
// model restricted the search to, plus the flags that change matching itself.
// Both the count and the sample must use all of them, or skim decides whether
// to intercept — and then describes the results — using a different query than
// the model actually asked for.
//
// `multiline` is passed without `--multiline-dotall`. Whether Claude Code adds
// dotall is not observable from the hook payload, and the two differ for
// patterns containing `.`: dotall matches more. Under that uncertainty the
// under-counting choice is the safe one, because under-counting means skim
// passes the call through untouched, while over-counting means intercepting a
// call that did not warrant it.
func matchArgs(gi hookio.GrepInput) []string {
	var a []string
	if gi.Glob != "" {
		a = append(a, "--glob", gi.Glob)
	}
	if gi.Type != "" {
		a = append(a, "--type", gi.Type)
	}
	if gi.CaseInsensitive {
		a = append(a, "-i")
	}
	if gi.Multiline {
		a = append(a, "--multiline")
	}
	return a
}

// outputArgs returns the args that change only how much text the call would
// have RETURNED, not which lines matched: context lines and line numbering.
// The sample pass needs these so its byte count is a fair proxy for what the
// Grep would have put in the session's context; the counting pass must not use
// them, since context lines would inflate a match count.
func outputArgs(gi hookio.GrepInput) []string {
	var a []string
	if gi.Context > 0 {
		a = append(a, "-C", strconv.Itoa(gi.Context))
	} else {
		if gi.After > 0 {
			a = append(a, "-A", strconv.Itoa(gi.After))
		}
		if gi.Before > 0 {
			a = append(a, "-B", strconv.Itoa(gi.Before))
		}
	}
	if gi.LineNumbers {
		a = append(a, "-n")
	}
	return a
}

// Args builds a full rg command line: the given mode flags, the shared
// matching args, then the pattern and optional path after `--`.
func Args(gi hookio.GrepInput, mode ...string) []string {
	args := append([]string{}, mode...)
	args = append(args, matchArgs(gi)...)
	args = append(args, "--", gi.Pattern)
	if gi.Path != "" {
		args = append(args, gi.Path)
	}
	return args
}

// Count backs handler.Deps.CountMatches. It runs
//
//	rg --count --no-heading --no-messages --color never [match args] -- <pattern> [path]
//
// and sums the per-file `path:N` counts. rg exit code 1 (no matches) is not an
// error; any other failure (rg missing, bad regex) is returned so the hook
// degrades open.
//
// `--count` counts MATCHING LINES; `--count-matches`, used here originally,
// counts occurrences. Grep in content mode returns one line per match, and
// grep_max_matches is compared against that, so occurrences systematically
// over-counted: `foo foo foo\nfoo\nbar` reports 4 with --count-matches and 2
// with --count. Every line with more than one hit pushed a call toward
// interception it had not earned.
func Count(gi hookio.GrepInput) (int, error) {
	cmd, cerr := Command(Args(gi,
		"--count", "--no-heading", "--no-messages", "--color", "never")...)
	if cerr != nil {
		return 0, cerr
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return 0, nil // rg: no matches
		}
		return 0, err
	}

	total := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if i := strings.LastIndex(line, ":"); i >= 0 {
			n := 0
			if _, serr := fmt.Sscanf(line[i+1:], "%d", &n); serr == nil {
				total += n
			}
		}
	}
	return total, nil
}

// Sample backs handler.Deps.Sample. It runs rg with the model's own matching
// AND output flags — its context lines and line numbering, not a fixed `-n -C1`
// — so the result approximates what the Grep call would actually have returned.
// It is called only for a Grep that already cleared the threshold. It returns
// the output capped at 32 KB for the worker prompt, plus the UNCAPPED byte
// length — the latter is what the Grep would have put in the session's context,
// and therefore the only honest baseline for the saving. Errors are returned
// (not swallowed) so a failed sample degrades open rather than feeding the
// worker an empty prompt.
func Sample(gi hookio.GrepInput) (string, int, error) {
	mode := append([]string{"--no-heading", "--no-messages", "--color", "never"},
		outputArgs(gi)...)
	cmd, cerr := Command(Args(gi, mode...)...)
	if cerr != nil {
		return "", 0, cerr
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "", 0, nil // rg: no matches
		}
		return "", 0, err
	}
	full := len(out)
	if len(out) > sampleCap {
		out = out[:sampleCap]
	}
	return string(out), full, nil
}

// Verify confirms the resolved binary really is ripgrep, by asking it for its
// version. It is for `skim doctor`, not the hook path: the check costs a
// process spawn, which is wasteful per interception but free in a diagnostic.
//
// It matters because the fallback trusts that a binary named `claude` is Claude
// Code, and therefore ripgrep. If something else answers to that name, Count
// parses its output as ripgrep counts and quietly returns a wrong number —
// observed while testing against a stub `claude`, where a 240-match search
// counted low enough to pass through untouched. That failure is safe (skim does
// nothing) but invisible, so doctor should be able to name it.
func Verify() (version string, err error) {
	cmd, err := Command("--version")
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not run the resolved ripgrep: %w", err)
	}
	first := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	if !strings.Contains(strings.ToLower(first), "ripgrep") {
		return first, fmt.Errorf("resolved binary does not identify as ripgrep (said %q)", first)
	}
	return first, nil
}
