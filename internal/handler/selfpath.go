package handler

import (
	"os"
	"path/filepath"
	"strings"
)

// executablePath is a seam over os.Executable so tests can simulate a self
// path (e.g. one containing whitespace) without needing a real binary at that
// location on disk.
var executablePath = os.Executable

// execPath returns the absolute path of the running skim binary. Every escape
// hatch skim suggests to the model (`skim run -- …`, `skim cat …`) must be a
// command the model can actually execute, and nothing puts skim on the user's
// PATH — the plugin invokes it as ${CLAUDE_PLUGIN_ROOT}/bin/skim. Falling back
// to the bare name keeps the message sensible if os.Executable ever fails.
func execPath() string {
	p, err := executablePath()
	if err != nil || p == "" {
		return "skim"
	}
	return p
}

// quoteSelfIfNeeded returns self as-is, unless it contains whitespace, in
// which case it is shell-single-quoted so it survives as one argv word when
// interpolated into a suggested `skim run --`/`skim cat` command. Without this
// a self path like "/Users/x/dir with space/bin/skim" would both be unrunnable
// AND, fed back through isSkimCommand's tokenizer, no longer be recognized as
// a self-invoking command — reopening the C1 infinite-escalation bug.
func quoteSelfIfNeeded(self string) string {
	if strings.ContainsAny(self, " \t\n\r") {
		return shellSingleQuote(self)
	}
	return self
}

// isSkimCommand reports whether cmd already invokes skim's own `run` or `cat`
// subcommand. Without this the Bash deny path is self-defeating: the suggested
// replacement embeds the original command verbatim, so it re-matches the same
// noisy pattern and gets denied again, forever.
//
// A prefix check on the first two tokens is deliberate — this only has to stop
// skim from fighting its own suggestions, not resist adversarial input. The
// strings.Fields tokenization alone breaks when self contains whitespace and
// was therefore shell-quoted in the suggestion (quoteSelfIfNeeded): Fields
// would split the quoted path apart at the embedded space. So this also checks
// the quoted-self prefix directly, without tokenizing self itself.
func isSkimCommand(cmd, self string) bool {
	fields := strings.Fields(cmd)
	if len(fields) >= 2 {
		bin := strings.Trim(fields[0], `"'`)
		if (bin == self || filepath.Base(bin) == "skim") && (fields[1] == "run" || fields[1] == "cat") {
			return true
		}
	}

	if !strings.ContainsAny(self, " \t\n\r") {
		return false
	}
	for _, quoted := range []string{shellSingleQuote(self), `"` + self + `"`} {
		rest := strings.TrimPrefix(cmd, quoted)
		if rest == cmd {
			continue
		}
		if verb := strings.Fields(rest); len(verb) > 0 && (verb[0] == "run" || verb[0] == "cat") {
			return true
		}
	}
	return false
}

// shellMetaChars are the characters whose meaning would be lost if a command
// were handed to `skim run --`, which execs argv directly with no shell. A
// newline or carriage return is included: a multi-line command handed to
// `skim run --` unwrapped loses every line but the first when the outer shell
// re-parses the suggestion, so it must trigger the sh -c wrapped form too.
const shellMetaChars = "|&;<>$`()\n\r"

// hasShellMeta reports whether cmd relies on shell syntax. Such a command must
// be wrapped in `sh -c '…'` when suggested through `skim run --`, otherwise the
// OUTER shell applies the pipe/redirect to skim itself rather than to cmd.
func hasShellMeta(cmd string) bool {
	return strings.ContainsAny(cmd, shellMetaChars)
}

// shellSingleQuote wraps s in single quotes, escaping any embedded single quote
// the POSIX way ('\”), so the result is one shell word.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
