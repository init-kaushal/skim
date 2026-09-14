package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaushal/skim/internal/detect"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
)

// BashHook is the PreToolUse interception for the Bash tool. Unlike the Read and
// Grep hooks it makes no worker call: it is a pure pattern match. It always
// returns nil after writing exactly one decision to d.Stdout — an allow
// (nothing) when the command looks quiet, or a deny that tells the model to
// re-run the command through `skim run` so the full output is captured to a log
// and reduced to a digest. Every error or negative check degrades open.
func BashHook(_ context.Context, in hookio.Input, d Deps) error {
	if d.Cfg.NotActiveReason(d.Env) != "" {
		return hookio.Allow(d.Stdout)
	}

	bi, err := in.Bash()
	if err != nil {
		d.Logf("bash-hook: bad tool_input: %v", err)
		return hookio.Allow(d.Stdout)
	}

	self := execPath()

	// skim's own suggestion embeds the original command, so it matches the very
	// pattern that produced it. Let skim's commands through before matching, or
	// the model can never escape the deny loop.
	if isSkimCommand(bi.Command, self) {
		return hookio.Allow(d.Stdout)
	}

	matched, pat := detect.MatchNoisy(bi.Command, d.Cfg.BashNoisyPatterns)
	if !matched {
		return hookio.Allow(d.Stdout)
	}

	// `skim run --` execs argv directly with no shell, so a command carrying
	// pipes/redirects/substitutions must be wrapped explicitly — otherwise the
	// shell that runs the suggestion applies them to skim, silently changing
	// what the command does.
	qself := quoteSelfIfNeeded(self)
	suggestion := fmt.Sprintf("%s run -- %s", qself, bi.Command)
	if hasShellMeta(bi.Command) {
		suggestion = fmt.Sprintf("%s run -- sh -c %s", qself, shellSingleQuote(bi.Command))
	}

	reason := fmt.Sprintf(
		"skim: this command tends to produce large output (matched /%s/).\n"+
			"Re-run it through skim so the full output is captured to a log and you get a digest:\n\n"+
			"  %s\n", pat, suggestion)

	d.Record(metrics.Entry{
		TS:   d.Now().UTC().Format(time.RFC3339),
		Tool: "Bash",
	})
	return hookio.Deny(d.Stdout, reason)
}
