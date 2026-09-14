package handler

import (
	"os"
	"path/filepath"
	"strings"
)

// execPath returns the absolute path of the running skim binary. Every escape
// hatch skim suggests to the model (`skim run -- …`, `skim cat …`) must be a
// command the model can actually execute, and nothing puts skim on the user's
// PATH — the plugin invokes it as ${CLAUDE_PLUGIN_ROOT}/bin/skim. Falling back
// to the bare name keeps the message sensible if os.Executable ever fails.
func execPath() string {
	p, err := os.Executable()
	if err != nil || p == "" {
		return "skim"
	}
	return p
}

// isSkimCommand reports whether cmd already invokes skim's own `run` or `cat`
// subcommand. Without this the Bash deny path is self-defeating: the suggested
// replacement embeds the original command verbatim, so it re-matches the same
// noisy pattern and gets denied again, forever.
//
// A prefix check on the first two tokens is deliberate — this only has to stop
// skim from fighting its own suggestions, not resist adversarial input.
func isSkimCommand(cmd, self string) bool {
	fields := strings.Fields(cmd)
	if len(fields) < 2 {
		return false
	}
	bin := strings.Trim(fields[0], `"'`)
	if bin != self && filepath.Base(bin) != "skim" {
		return false
	}
	return fields[1] == "run" || fields[1] == "cat"
}

// shellMetaChars are the characters whose meaning would be lost if a command
// were handed to `skim run --`, which execs argv directly with no shell.
const shellMetaChars = "|&;<>$`()"

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
